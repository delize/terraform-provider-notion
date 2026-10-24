package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccPageIconsLockPosition walks a page through a native icon with
// is_locked and a create position, then an external icon URL unlocked, then
// no icon at all (which must clear it), and finally an import.
func TestAccPageIconsLockPosition(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

	const addr = "notion_page.icons"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPageIconsConfig(parentPageID, `
  icon_name  = "pizza"
  icon_color = "blue"
  is_locked  = true`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "icon_name", "pizza"),
					resource.TestCheckResourceAttr(addr, "icon_color", "blue"),
					resource.TestCheckResourceAttr(addr, "icon", ""),
					resource.TestCheckResourceAttr(addr, "is_locked", "true"),
					resource.TestCheckResourceAttr(addr, "position", "page_start"),
				),
			},
			{
				Config: testAccPageIconsConfig(parentPageID, `
  icon_url  = "https://www.google.com/favicon.ico"
  is_locked = false`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(addr, "icon_name"),
					resource.TestCheckResourceAttr(addr, "icon_url", "https://www.google.com/favicon.ico"),
					resource.TestCheckResourceAttr(addr, "is_locked", "false"),
				),
			},
			{
				Config: testAccPageIconsConfig(parentPageID, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(addr, "icon_url"),
					resource.TestCheckResourceAttr(addr, "icon", ""),
				),
			},
			{
				ResourceName:            addr,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"position"},
			},
		},
	})
}

func testAccPageIconsConfig(parentPageID, extra string) string {
	return fmt.Sprintf(`
resource "notion_page" "icons" {
  parent_page_id = %q
  title          = "tf-acc-test-icons-%s"
  position       = "page_start"
%s
}
`, parentPageID, testRunSuffix(), extra)
}

// TestAccDatabaseIconsLock sets a native icon and lock on a database, then
// switches to an emoji icon and unlocks it.
func TestAccDatabaseIconsLock(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

	const addr = "notion_database.icons"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDatabaseIconsConfig(parentPageID, `
  icon_name = "token"
  is_locked = true`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "icon_name", "token"),
					resource.TestCheckResourceAttr(addr, "is_locked", "true"),
				),
			},
			{
				Config: testAccDatabaseIconsConfig(parentPageID, `
  icon      = "🚀"
  is_locked = false`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "icon", "🚀"),
					resource.TestCheckNoResourceAttr(addr, "icon_name"),
					resource.TestCheckResourceAttr(addr, "is_locked", "false"),
				),
			},
		},
	})
}

func testAccDatabaseIconsConfig(parentPageID, extra string) string {
	return fmt.Sprintf(`
resource "notion_database" "icons" {
  parent             = %q
  title              = "tf-acc-test-db-icons-%s"
  title_column_title = "Name"
%s
}
`, parentPageID, testRunSuffix(), extra)
}
