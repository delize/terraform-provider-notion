package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func templatesTestServer(t *testing.T, dataSources string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get("Notion-Version"); v != notionTrashAPIVersion {
			t.Errorf("Notion-Version: %q", v)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/databases/db1":
			_, _ = w.Write([]byte(`{"object":"database","id":"db1","data_sources":` + dataSources + `}`))
		case r.URL.Path == "/data_sources/ds1/templates" && r.URL.Query().Get("start_cursor") == "":
			if r.URL.Query().Get("name") != "weekly" {
				t.Errorf("name filter not sent: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"templates":[{"id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","name":"Weekly sync","is_default":false}],"has_more":true,"next_cursor":"next"}`))
		case r.URL.Path == "/data_sources/ds1/templates":
			_, _ = w.Write([]byte(`{"templates":[{"id":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb","name":"Weekly review","is_default":true}],"has_more":false,"next_cursor":null}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"object":"error","code":"object_not_found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDiscoverDataSourceID(t *testing.T) {
	ctx := context.Background()

	srv := templatesTestServer(t, `[{"id":"ds1","name":"Tasks"}]`)
	id, err := discoverDataSourceID(ctx, srv.URL, "tok", "db1")
	if err != nil || id != "ds1" {
		t.Errorf("single: got %q, %v", id, err)
	}

	srv = templatesTestServer(t, `[]`)
	if _, err := discoverDataSourceID(ctx, srv.URL, "tok", "db1"); err == nil {
		t.Error("none: expected error")
	}

	srv = templatesTestServer(t, `[{"id":"ds1","name":"Tasks"},{"id":"ds2","name":"Archive"}]`)
	_, err = discoverDataSourceID(ctx, srv.URL, "tok", "db1")
	if err == nil || !strings.Contains(err.Error(), "Archive (ds2)") {
		t.Errorf("several: expected error naming each data source, got %v", err)
	}
}

func TestListDataSourceTemplates_Paginates(t *testing.T) {
	srv := templatesTestServer(t, `[]`)
	got, err := listDataSourceTemplates(context.Background(), srv.URL, "tok", "ds1", "weekly")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[1].IsDefault || got[0].Name != "Weekly sync" {
		t.Errorf("templates: %+v", got)
	}
}

func TestUnitDatabaseTemplates_RequiresExactlyOneID(t *testing.T) {
	for _, body := range []string{
		``,
		`database_id = "db"
  data_source_id = "ds"`,
	} {
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{{
				Config: fmt.Sprintf(`
provider "notion" {
  token = "unit-test-token"
}

data "notion_database_templates" "t" {
  %s
}`, body),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Set exactly one of database_id or data_source_id`),
			}},
		})
	}
}

// TestAccDatabaseTemplatesDataSource discovers the data source of a fresh
// database and lists its templates. A new database may have none, so the
// count isn't asserted. Discovery working is the point.
func TestAccDatabaseTemplatesDataSource(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "notion_database" "templates_test" {
  parent             = %q
  title              = "Templates Test DB"
  title_column_title = "Name"
}

data "notion_database_templates" "t" {
  database_id = notion_database.templates_test.id
}
`, parentPageID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("data.notion_database_templates.t", "data_source_id", regexp.MustCompile(`^[0-9a-f]{32}$`)),
					resource.TestMatchResourceAttr("data.notion_database_templates.t", "templates.#", regexp.MustCompile(`^[0-9]+$`)),
				),
			},
		},
	})
}
