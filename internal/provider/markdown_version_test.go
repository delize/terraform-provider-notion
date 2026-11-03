package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jomei/notionapi"
)

// TestAccPageMarkdownV2 creates a page with markdown_version = "v2", then
// changes only the title, then changes the markdown.
//
// Between the first two steps it appends a paragraph through the API, the
// way someone editing in the Notion UI would. The title-only update must
// leave that paragraph alone, since the body is resent only when the
// markdown or its version changes. The markdown change then replaces the
// whole body, so the paragraph is gone after step three.
func TestAccPageMarkdownV2(t *testing.T) {
	if os.Getenv("NOTION_TEST_PARENT_PAGE_ID") == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}
	client := notionTestClient(t)
	parentPageID := makeIsolatedParentPage(t, client, "pagemarkdownv2")

	const outside = "Edited outside Terraform"
	var pageID string
	capture := func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["notion_page.v2"]
		if !ok {
			return fmt.Errorf("notion_page.v2 not in state")
		}
		pageID = rs.Primary.ID
		return nil
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPageMarkdownV2Config(parentPageID, "Markdown v2", "## Hello\n\nFirst body."),
				Check: resource.ComposeAggregateTestCheckFunc(
					capture,
					resource.TestCheckResourceAttr("notion_page.v2", "markdown_version", "v2"),
					resource.TestMatchResourceAttr("data.notion_page_markdown.v2", "markdown", regexp.MustCompile(`First body`)),
				),
			},
			{
				PreConfig: func() {
					_, err := client.Block.AppendChildren(context.Background(), notionapi.BlockID(pageID), &notionapi.AppendBlockChildrenRequest{
						Children: []notionapi.Block{
							notionapi.ParagraphBlock{
								BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeParagraph},
								Paragraph:  notionapi.Paragraph{RichText: plainToRichText(outside)},
							},
						},
					})
					if err != nil {
						t.Fatalf("appending a block outside Terraform: %v", err)
					}
				},
				Config: testAccPageMarkdownV2Config(parentPageID, "Markdown v2 renamed", "## Hello\n\nFirst body."),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_page.v2", "title", "Markdown v2 renamed"),
					resource.TestMatchResourceAttr("data.notion_page_markdown.v2", "markdown", regexp.MustCompile(outside)),
					resource.TestMatchResourceAttr("data.notion_page_markdown.v2", "markdown", regexp.MustCompile(`First body`)),
				),
			},
			{
				Config: testAccPageMarkdownV2Config(parentPageID, "Markdown v2 renamed", "## Changed\n\nSecond body."),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("data.notion_page_markdown.v2", "markdown", regexp.MustCompile(`Second body`)),
					func(s *terraform.State) error {
						md := s.RootModule().Resources["data.notion_page_markdown.v2"].Primary.Attributes["markdown"]
						if regexp.MustCompile(outside).MatchString(md) {
							return fmt.Errorf("markdown change should have replaced the whole body, still found %q in %q", outside, md)
						}
						return nil
					},
				),
			},
		},
	})
}

// The data source depends on the page so it reads after the page's update
// in the same apply, not during plan.
func testAccPageMarkdownV2Config(parentPageID, title, markdown string) string {
	return fmt.Sprintf(`
resource "notion_page" "v2" {
  parent_page_id   = %q
  title            = %q
  markdown         = %q
  markdown_version = "v2"
}

data "notion_page_markdown" "v2" {
  page_id          = notion_page.v2.id
  markdown_version = "v2"
  depends_on       = [notion_page.v2]
}
`, parentPageID, title, markdown)
}
