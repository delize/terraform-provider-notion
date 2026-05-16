package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccCommentResource_RichText creates a comment on a fresh page using
// rich_text body, then updates the body. Verifies the comment exists at
// each step and that update flows through to plain_text changing.
func TestAccCommentResource_RichText(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}
	client := notionTestClient(t)
	parentPageID := makeIsolatedParentPage(t, client, "comment-rt")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCommentRichTextConfig(parentPageID, "First comment"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_comment.test", "id"),
					resource.TestCheckResourceAttr("notion_comment.test", "plain_text", "First comment"),
					resource.TestCheckResourceAttrSet("notion_comment.test", "discussion_id"),
					resource.TestCheckResourceAttrSet("notion_comment.test", "created_time"),
				),
			},
			{
				Config: testAccCommentRichTextConfig(parentPageID, "First comment (edited)"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_comment.test", "plain_text", "First comment (edited)"),
				),
			},
		},
	})
}

// TestAccCommentResource_Markdown creates a comment on a fresh page using
// the markdown body parameter (Notion's April 2026 addition).
func TestAccCommentResource_Markdown(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}
	client := notionTestClient(t)
	parentPageID := makeIsolatedParentPage(t, client, "comment-md")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCommentMarkdownConfig(parentPageID, "**Bold** comment with [link](https://example.com)."),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_comment.test_md", "id"),
					resource.TestCheckResourceAttrSet("notion_comment.test_md", "plain_text"),
				),
			},
		},
	})
}

func testAccCommentRichTextConfig(parentPageID, body string) string {
	return fmt.Sprintf(`
resource "notion_comment" "test" {
  parent_page_id = %q
  rich_text      = %q
}
`, parentPageID, body)
}

func testAccCommentMarkdownConfig(parentPageID, markdown string) string {
	return fmt.Sprintf(`
resource "notion_comment" "test_md" {
  parent_page_id = %q
  markdown       = %q
}
`, parentPageID, markdown)
}
