package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDatabaseResource(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

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

func testAccDatabaseResourceConfig(parentPageID, title, titleColumnTitle string) string {
	return fmt.Sprintf(`
resource "notion_database" "test" {
  parent             = %q
  title              = %q
  title_column_title = %q
}
`, parentPageID, title, titleColumnTitle)
}
