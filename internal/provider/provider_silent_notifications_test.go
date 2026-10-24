package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccSilentNotifications runs page create, update, and destroy with
// silent_notifications on. It covers both request paths: the SDK, which
// sends Notion-Version 2022-06-28, and the direct-HTTP markdown and trash
// calls on 2026-03-11. Notion rejecting notifications.mode on either
// version would fail this test. Notifications themselves can't be observed
// from the API, so only acceptance of the field is asserted.
func TestAccSilentNotifications(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSilentNotificationsConfig(parentPageID, "Silent Page"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_page.silent", "title", "Silent Page"),
					resource.TestCheckResourceAttrSet("notion_page.silent_md", "id"),
				),
			},
			{
				Config: testAccSilentNotificationsConfig(parentPageID, "Silent Page Updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_page.silent", "title", "Silent Page Updated"),
				),
			},
		},
	})
}

func testAccSilentNotificationsConfig(parentPageID, title string) string {
	return fmt.Sprintf(`
provider "notion" {
  silent_notifications = true
}

resource "notion_page" "silent" {
  parent_page_id = %[1]q
  title          = %[2]q
}

resource "notion_page" "silent_md" {
  parent_page_id = %[1]q
  title          = "Silent Markdown Page"
  markdown       = "Created without notifications."
}
`, parentPageID, title)
}
