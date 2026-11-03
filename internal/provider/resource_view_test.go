package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// fetchDefaultDataSourceID returns the first data_source_id reported by
// GET /v1/databases/{id} under Notion-Version 2026-03-11. Used by view
// acceptance tests since the SDK is pinned to a pre-v2025-09-03 version
// and doesn't expose data sources.
//
// Returns "" if the response doesn't include a data_sources array — that
// signals the workspace is on a pre-v2025-09-03 API and the test should
// skip.
func fetchDefaultDataSourceID(t *testing.T, databaseID string) string {
	t.Helper()

	token := os.Getenv("NOTION_TOKEN")
	if token == "" {
		t.Fatal("NOTION_TOKEN must be set")
	}

	url := fmt.Sprintf("%s/databases/%s", notionAPIBaseURL, databaseID)
	resp, err := doNotionRequest(context.Background(), http.MethodGet, url, token, nil)
	if err != nil {
		t.Fatalf("fetching database %s: %v", databaseID, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading database response: %v", err)
	}
	if resp.StatusCode >= 400 {
		t.Fatalf("notion API %d fetching database %s: %s", resp.StatusCode, databaseID, string(body))
	}

	var parsed struct {
		DataSources []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data_sources"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parsing database response: %v", err)
	}
	if len(parsed.DataSources) == 0 {
		return ""
	}
	return normalizeID(parsed.DataSources[0].ID)
}

// TestAccViewResource drives notion_view through Create + Update against a
// pre-existing test database. The database is provided via
// NOTION_TEST_VIEW_DATABASE_ID; its data_source_id is discovered at
// test-time via a direct API call (the SDK doesn't expose it).
//
// Skips when the database GET doesn't include a data_sources array — that
// means the workspace is on a pre-v2025-09-03 API and the Views API can't
// be exercised end-to-end yet.
func TestAccViewResource(t *testing.T) {
	databaseID := os.Getenv("NOTION_TEST_VIEW_DATABASE_ID")
	if databaseID == "" {
		t.Skip("NOTION_TEST_VIEW_DATABASE_ID not set — set to a Notion database ID accessible to the integration. Any test database works; it won't be modified beyond having a view added.")
	}

	dataSourceID := fetchDefaultDataSourceID(t, normalizeID(databaseID))
	if dataSourceID == "" {
		t.Skip("database GET did not return a data_sources array; workspace likely on pre-v2025-09-03 API. The Views API requires a data_source_id which can't be discovered.")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccViewConfig(databaseID, dataSourceID, "TF Acc View Initial", "table"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_view.test", "id"),
					resource.TestCheckResourceAttr("notion_view.test", "name", "TF Acc View Initial"),
					resource.TestCheckResourceAttr("notion_view.test", "type", "table"),
					resource.TestCheckResourceAttrSet("notion_view.test", "url"),
				),
			},
			{
				Config: testAccViewConfig(databaseID, dataSourceID, "TF Acc View Renamed", "table"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_view.test", "name", "TF Acc View Renamed"),
				),
			},
			{
				ResourceName:      "notion_view.test",
				ImportState:       true,
				ImportStateVerify: true,
				// filter/sorts/quick_filters/configuration may differ on import
				// (Notion may inject defaults). Ignoring keeps the assertion
				// focused on the round-tripping fields we care about.
				ImportStateVerifyIgnore: []string{"filter", "sorts", "quick_filters", "configuration"},
			},
		},
	})
}

// TestAccViewQueryDataSource exercises the POST /v1/views/{id}/query path
// against a pre-existing view. Gated on NOTION_TEST_VIEW_ID — typically you'd
// run TestAccViewResource first, copy the view ID it created, and feed it in.
func TestAccViewQueryDataSource(t *testing.T) {
	viewID := os.Getenv("NOTION_TEST_VIEW_ID")
	if viewID == "" {
		t.Skip("NOTION_TEST_VIEW_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "notion_view_query" "test" {
  view_id   = %q
  page_size = 5
}
`, viewID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.notion_view_query.test", "raw_json"),
					resource.TestCheckResourceAttrSet("data.notion_view_query.test", "has_more"),
				),
			},
		},
	})
}

// TestAccViewJSONAttributes creates a table view with a filter, sorts, a
// quick filter, and a configuration with property visibility on a new
// database in an isolated page, then changes each of them. Every step's
// post-apply plan must be empty, which fails if Notion's copy of any JSON
// attribute differs from the config in a way the provider doesn't treat as
// the same value. The filter and sorts name properties ("Priority",
// "Name"), which Notion answers with property IDs, and Notion lists every
// property in the table configuration. The provider matches both through
// the data source schema.
func TestAccViewJSONAttributes(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}
	parentID := makeIsolatedParentPage(t, notionTestClient(t), "view-json")

	const addr = "notion_view.v"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourcesTrashed(t),
		Steps: []resource.TestStep{
			{
				Config: testAccViewJSONConfig(parentID, "High", "ascending", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "filter", `{"property":"Priority","select":{"equals":"High"}}`),
					resource.TestCheckResourceAttr(addr, "sorts", `[{"direction":"ascending","property":"Name"},{"direction":"descending","property":"Priority"}]`),
					resource.TestCheckResourceAttr(addr, "quick_filters", `{"title":{}}`),
					resource.TestCheckResourceAttr(addr, "configuration",
						`{"properties":[{"property_id":"title","visible":true,"width":320}],"type":"table","wrap_cells":true}`),
				),
			},
			{
				Config: testAccViewJSONConfig(parentID, "Low", "descending", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "filter", `{"property":"Priority","select":{"equals":"Low"}}`),
					resource.TestCheckResourceAttr(addr, "configuration",
						`{"properties":[{"property_id":"title","visible":true,"width":320}],"type":"table","wrap_cells":false}`),
				),
			},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateVerify: true,
				// Notion returns configuration fields the config doesn't set,
				// and property IDs where the config names properties. The
				// provider matches those only against a config.
				ImportStateVerifyIgnore: []string{"configuration", "filter", "sorts"},
			},
		},
	})
}

func testAccViewJSONConfig(parentID, priority, nameDirection string, wrapCells bool) string {
	return fmt.Sprintf(`
resource "notion_database" "db" {
  parent             = %q
  title              = "View JSON attributes"
  title_column_title = "Name"
}

resource "notion_database_property_select" "priority" {
  database = notion_database.db.id
  name     = "Priority"
  options = {
    "High" = "red"
    "Low"  = "gray"
  }
}

resource "notion_view" "v" {
  database_id    = notion_database.db.id
  data_source_id = notion_database.db.data_source_ids[0]
  name           = "Open work"
  type           = "table"

  filter = jsonencode({
    property = "Priority"
    select   = { equals = %q }
  })

  sorts = jsonencode([
    { property = "Name", direction = %q },
    { property = "Priority", direction = "descending" },
  ])

  quick_filters = jsonencode({ title = {} })

  configuration = jsonencode({
    type       = "table"
    properties = [{ property_id = "title", visible = true, width = 320 }]
    wrap_cells = %t
  })

  depends_on = [notion_database_property_select.priority]
}
`, parentID, priority, nameDirection, wrapCells)
}

func testAccViewConfig(databaseID, dataSourceID, name, viewType string) string {
	return fmt.Sprintf(`
resource "notion_view" "test" {
  database_id    = %q
  data_source_id = %q
  name           = %q
  type           = %q
}
`, databaseID, dataSourceID, name, viewType)
}
