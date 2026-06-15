package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccMeetingNotesDataSource is a smoke test against the 2026-05-11
// POST /v1/blocks/meeting_notes/query endpoint. The workspace may have zero
// meeting notes available to the integration — that's fine, we only assert
// that the call succeeds and raw_json is populated (it'll be the literal
// response, which still contains at least `{"results":[]}`).
func TestAccMeetingNotesDataSource(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "notion_meeting_notes" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.notion_meeting_notes.all", "raw_json"),
					// results may be empty; the attribute itself must be set.
					resource.TestCheckResourceAttrSet("data.notion_meeting_notes.all", "results.#"),
				),
			},
		},
	})
}

// TestAccMeetingNotesDataSource_WithLimit exercises the limit parameter to
// make sure the JSON request body is encoded correctly. Still tolerates an
// empty workspace.
func TestAccMeetingNotesDataSource_WithLimit(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "notion_meeting_notes" "limited" {
  limit = 5
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.notion_meeting_notes.limited", "raw_json"),
				),
			},
		},
	})
}
