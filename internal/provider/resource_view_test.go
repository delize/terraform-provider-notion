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
