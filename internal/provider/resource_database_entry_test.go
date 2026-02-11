package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDatabaseEntryResource(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDatabaseEntryResourceConfig(parentPageID, "Test Entry"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_database_entry.test", "id"),
					resource.TestCheckResourceAttr("notion_database_entry.test", "title", "Test Entry"),
					resource.TestCheckResourceAttrSet("notion_database_entry.test", "url"),
				),
			},
			{
				Config: testAccDatabaseEntryResourceConfig(parentPageID, "Test Entry Updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_database_entry.test", "title", "Test Entry Updated"),
				),
			},
		},
	})
}

func testAccDatabaseEntryResourceConfig(parentPageID, title string) string {
	return fmt.Sprintf(`
resource "notion_database" "test_entry_parent" {
  parent             = %q
  title              = "Entry Test DB"
  title_column_title = "Name"
}

resource "notion_database_entry" "test" {
  database = notion_database.test_entry_parent.id
  title    = %q
}
`, parentPageID, title)
}
