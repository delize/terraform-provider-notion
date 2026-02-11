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

func testAccPageResourceConfig(parentPageID, title string) string {
	return fmt.Sprintf(`
resource "notion_page" "test" {
  parent_page_id = %q
  title          = %q
}
`, parentPageID, title)
}
