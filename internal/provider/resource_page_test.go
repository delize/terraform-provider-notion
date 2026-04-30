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
