package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPageResource(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPageResourceConfig(parentPageID, "Test Page"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_page.test", "id"),
					resource.TestCheckResourceAttr("notion_page.test", "title", "Test Page"),
					resource.TestCheckResourceAttrSet("notion_page.test", "url"),
				),
			},
			{
				ResourceName:      "notion_page.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccPageResourceConfig(parentPageID, "Test Page Updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_page.test", "title", "Test Page Updated"),
				),
			},
		},
	})
}

func TestAccPageResourceWithMarkdown(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPageResourceWithMarkdownConfig(parentPageID, "Markdown Page", "## Hello\n\nWorld"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_page.test_md", "id"),
					resource.TestCheckResourceAttr("notion_page.test_md", "title", "Markdown Page"),
					resource.TestCheckResourceAttrSet("notion_page.test_md", "url"),
					resource.TestCheckResourceAttrSet("notion_page.test_md", "markdown"),
				),
			},
			{
				Config: testAccPageResourceWithMarkdownConfig(parentPageID, "Markdown Page Updated", "## Updated\n\n- [ ] Task 1\n- [x] Task 2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_page.test_md", "title", "Markdown Page Updated"),
					resource.TestCheckResourceAttrSet("notion_page.test_md", "markdown"),
				),
			},
		},
	})
}

func TestAccPageMarkdownDataSource(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPageMarkdownDataSourceConfig(parentPageID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.notion_page_markdown.test", "markdown"),
				),
			},
		},
	})
}

func testAccPageResourceConfig(parentPageID, title string) string {
	return fmt.Sprintf(`
resource "notion_page" "test" {
  parent_page_id = %q
  title          = %q
}
`, parentPageID, title)
}

func testAccPageResourceWithMarkdownConfig(parentPageID, title, markdown string) string {
	return fmt.Sprintf(`
resource "notion_page" "test_md" {
  parent_page_id = %q
  title          = %q
  markdown       = %q
}
`, parentPageID, title, markdown)
}

func testAccPageMarkdownDataSourceConfig(parentPageID string) string {
	return fmt.Sprintf(`
resource "notion_page" "source" {
  parent_page_id = %q
  title          = "Data Source Test Page"
  markdown       = "## Test Content\n\nSome paragraph text."
}

data "notion_page_markdown" "test" {
  page_id = notion_page.source.id
}
`, parentPageID)
}

// TestAccPageMarkdownInsert exercises the 2026-05-15 insert_content.position
// path through the notion_page.markdown_insert nested attribute. Two steps:
//   1. Create the page with initial markdown + an insert at "end".
//   2. Change the insert content + flip to "start"; verify it re-applies (each
//      change is a trigger, not declarative — both inserts will be on the page).
//
// We can't easily verify the actual page body without round-tripping through
// the markdown data source, but Notion normalizes markdown so a strict equality
// check would be brittle. Smoke-test is: the apply succeeds and state holds
// the values we set.
func TestAccPageMarkdownInsert(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPageMarkdownInsertConfig(parentPageID, "## Initial", "Appended at end.", "end"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_page.insert_test", "id"),
					resource.TestCheckResourceAttr("notion_page.insert_test", "markdown_insert.content", "Appended at end."),
					resource.TestCheckResourceAttr("notion_page.insert_test", "markdown_insert.position", "end"),
				),
			},
			{
				Config: testAccPageMarkdownInsertConfig(parentPageID, "## Initial", "Prepended now.", "start"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_page.insert_test", "markdown_insert.content", "Prepended now."),
					resource.TestCheckResourceAttr("notion_page.insert_test", "markdown_insert.position", "start"),
				),
			},
		},
	})
}

func testAccPageMarkdownInsertConfig(parentPageID, body, insertContent, position string) string {
	return fmt.Sprintf(`
resource "notion_page" "insert_test" {
  parent_page_id = %q
  title          = "Markdown Insert Test"
  markdown       = %q

  markdown_insert = {
    content  = %q
    position = %q
  }
}
`, parentPageID, body, insertContent, position)
}
