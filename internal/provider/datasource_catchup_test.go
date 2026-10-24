package provider

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccSearchDataSource_InTrash checks that Notion accepts filter.in_trash
// (2026-07-15) on the 2022-06-28 search path. The trash contents of a shared
// workspace aren't predictable and trashed pages take time to reach the
// index, so only a successful read is asserted.
func TestAccSearchDataSource_InTrash(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "notion_search" "trash" {
  filter_object = "page"
  in_trash      = true
}
`,
				Check: resource.TestMatchResourceAttr("data.notion_search.trash", "results.#", regexp.MustCompile(`^[0-9]+$`)),
			},
		},
	})
}

// TestAccPageMarkdownDataSource_IncludeTranscript proves Notion accepts
// include_transcript (2026-02-26). The page has no meeting notes, so the
// markdown should match the plain read.
func TestAccPageMarkdownDataSource_IncludeTranscript(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "notion_page" "source" {
  parent_page_id = %q
  title          = "Transcript Flag Test Page"
  markdown       = "## Notes\n\nNo meeting here."
}

data "notion_page_markdown" "plain" {
  page_id = notion_page.source.id
}

data "notion_page_markdown" "with_transcript" {
  page_id            = notion_page.source.id
  include_transcript = true
}
`, parentPageID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.notion_page_markdown.with_transcript", "markdown"),
					resource.TestCheckResourceAttrPair(
						"data.notion_page_markdown.with_transcript", "markdown",
						"data.notion_page_markdown.plain", "markdown",
					),
				),
			},
		},
	})
}

// TestAccDatabaseEntriesDataSource_FilterAndSorts builds a small database
// and queries it with a multi-value select filter (2026-04-17) and a title
// sort. Database queries are read-consistent, unlike search, so exact counts
// and order are asserted.
func TestAccDatabaseEntriesDataSource_FilterAndSorts(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "notion_database" "filter_test" {
  parent             = %q
  title              = "Entries Filter Test DB"
  title_column_title = "Name"
}

resource "notion_database_property_select" "stage" {
  database = notion_database.filter_test.id
  name     = "Stage"
  options = {
    "Draft"  = "gray"
    "Review" = "yellow"
    "Live"   = "green"
  }
}

resource "notion_database_entry" "rows" {
  for_each = { "Alpha" = "Draft", "Bravo" = "Review", "Charlie" = "Live" }

  database          = notion_database.filter_test.id
  title             = each.key
  select_properties = { "Stage" = each.value }

  depends_on = [notion_database_property_select.stage]
}

data "notion_database_entries" "not_live" {
  database = notion_database.filter_test.id
  filter = jsonencode({
    property = "Stage"
    select   = { equals = ["Draft", "Review"] }
  })
  sorts = jsonencode([{ property = "Name", direction = "descending" }])

  depends_on = [notion_database_entry.rows]
}
`, parentPageID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.notion_database_entries.not_live", "entries.#", "2"),
					resource.TestCheckResourceAttr("data.notion_database_entries.not_live", "entries.0.title", "Bravo"),
					resource.TestCheckResourceAttr("data.notion_database_entries.not_live", "entries.1.title", "Alpha"),
				),
			},
		},
	})
}

// TestAccUsersDataSource_EmailVerified reads users over the direct-HTTP path.
// email_verified is null for bots and when the token lacks email
// capabilities, so the test asserts the list still loads.
func TestAccUsersDataSource_EmailVerified(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "notion_users" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.notion_users.all", "users.0.id"),
					logAttr(t, "data.notion_users.all", "users.0.email_verified"),
				),
			},
		},
	})
}
