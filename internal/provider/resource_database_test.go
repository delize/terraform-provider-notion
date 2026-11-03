package provider

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDatabaseResource(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}
	// Work inside a throwaway child page so anything a failed run leaves
	// behind is trashed with it.
	parentPageID = makeIsolatedParentPage(t, notionTestClient(t), "databaseresource")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDatabaseResourceConfig(parentPageID, "Test DB", "Name"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_database.test", "id"),
					resource.TestCheckResourceAttr("notion_database.test", "title", "Test DB"),
					resource.TestCheckResourceAttr("notion_database.test", "title_column_title", "Name"),
					resource.TestCheckResourceAttrSet("notion_database.test", "url"),
					resource.TestCheckNoResourceAttr("notion_database.test", "database_type"),
					resource.TestCheckResourceAttr("notion_database.test", "data_source_ids.#", "1"),
					resource.TestMatchResourceAttr("notion_database.test", "data_source_ids.0", regexp.MustCompile(`^[0-9a-f]{32}$`)),
				),
			},
			{
				ResourceName:      "notion_database.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccDatabaseResourceConfig(parentPageID, "Test DB Updated", "Name"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_database.test", "title", "Test DB Updated"),
				),
			},
		},
	})
}

// TestAccDatabaseShowDataSourceTitle walks a full-page database with one
// data source through show_data_source_title. Notion never shows the title
// on such a database, so this is where an API that echoed the effective
// value instead of the stored one would plan a diff forever. Every step's
// post-apply plan must be empty. Removing the attribute leaves the value
// alone.
func TestAccDatabaseShowDataSourceTitle(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}
	parentPageID := makeIsolatedParentPage(t, notionTestClient(t), "db-show-ds-title")

	const addr = "notion_database.show"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourcesTrashed(t),
		Steps: []resource.TestStep{
			{
				Config: testAccDatabaseShowDSTitleConfig(parentPageID, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "is_inline", "false"),
					resource.TestCheckResourceAttr(addr, "show_data_source_title", "true"),
				),
			},
			{
				Config: testAccDatabaseShowDSTitleConfig(parentPageID, "false"),
				Check:  resource.TestCheckResourceAttr(addr, "show_data_source_title", "false"),
			},
			{
				Config: testAccDatabaseShowDSTitleConfig(parentPageID, "true"),
				Check:  resource.TestCheckResourceAttr(addr, "show_data_source_title", "true"),
			},
			{
				Config: testAccDatabaseShowDSTitleConfig(parentPageID, ""),
				Check:  resource.TestCheckResourceAttr(addr, "show_data_source_title", "true"),
			},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccDatabaseShowDSTitleConfig(parentPageID, show string) string {
	line := ""
	if show != "" {
		line = "  show_data_source_title = " + show + "\n"
	}
	return fmt.Sprintf(`
resource "notion_database" "show" {
  parent             = %q
  title              = "Show data source title"
  title_column_title = "Name"
%s}
`, parentPageID, line)
}

// TestAccDatabaseDataSourceIDsFeedView creates a view on a new database by
// referencing data_source_ids[0], with no API lookup in the test.
func TestAccDatabaseDataSourceIDsFeedView(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}
	parentPageID := makeIsolatedParentPage(t, notionTestClient(t), "db-ds-view")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourcesTrashed(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "notion_database" "ds" {
  parent             = %q
  title              = "Data source IDs"
  title_column_title = "Name"
}

resource "notion_view" "table" {
  database_id    = notion_database.ds.id
  data_source_id = notion_database.ds.data_source_ids[0]
  name           = "From data_source_ids"
  type           = "table"
}
`, parentPageID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_database.ds", "data_source_ids.#", "1"),
					resource.TestCheckResourceAttrSet("notion_view.table", "id"),
					resource.TestCheckResourceAttrPair("notion_view.table", "data_source_id", "notion_database.ds", "data_source_ids.0"),
				),
			},
		},
	})
}

func testAccDatabaseResourceConfig(parentPageID, title, titleColumnTitle string) string {
	return fmt.Sprintf(`
resource "notion_database" "test" {
  parent             = %q
  title              = %q
  title_column_title = %q
}
`, parentPageID, title, titleColumnTitle)
}
