package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDatabasePropertyRichTextResource(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDatabasePropertyBasicConfig(parentPageID, "rich_text", "Description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_database_property_rich_text.test", "id"),
					resource.TestCheckResourceAttr("notion_database_property_rich_text.test", "name", "Description"),
				),
			},
		},
	})
}

func TestAccDatabasePropertySelectResource(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDatabasePropertySelectConfig(parentPageID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_database_property_select.test", "id"),
					resource.TestCheckResourceAttr("notion_database_property_select.test", "name", "Status"),
				),
			},
		},
	})
}

func TestAccDatabasePropertyNumberResource(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDatabasePropertyNumberConfig(parentPageID, "dollar"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_database_property_number.test", "id"),
					resource.TestCheckResourceAttr("notion_database_property_number.test", "format", "dollar"),
				),
			},
			{
				Config: testAccDatabasePropertyNumberConfig(parentPageID, "percent"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_database_property_number.test", "format", "percent"),
				),
			},
		},
	})
}

func testAccDatabasePropertyBasicConfig(parentPageID, propType, propName string) string {
	return fmt.Sprintf(`
resource "notion_database" "prop_test" {
  parent             = %q
  title              = "Property Test DB"
  title_column_title = "Name"
}

resource "notion_database_property_%s" "test" {
  database = notion_database.prop_test.id
  name     = %q
}
`, parentPageID, propType, propName)
}

func testAccDatabasePropertySelectConfig(parentPageID string) string {
	return fmt.Sprintf(`
resource "notion_database" "select_test" {
  parent             = %q
  title              = "Select Test DB"
  title_column_title = "Name"
}

resource "notion_database_property_select" "test" {
  database = notion_database.select_test.id
  name     = "Status"
  options = {
    "Active"   = "green"
    "Inactive" = "red"
  }
}
`, parentPageID)
}

func testAccDatabasePropertyNumberConfig(parentPageID, format string) string {
	return fmt.Sprintf(`
resource "notion_database" "number_test" {
  parent             = %q
  title              = "Number Test DB"
  title_column_title = "Name"
}

resource "notion_database_property_number" "test" {
  database = notion_database.number_test.id
  name     = "Price"
  format   = %q
}
`, parentPageID, format)
}

// TestAccDatabasePropertyStatusResource exercises the 2026-03-19 writable
// status property. Two steps verify create + option update; Notion groups
// (To-do / In progress / Complete) are assigned server-side so we don't
// assert on them.
func TestAccDatabasePropertyStatusResource(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDatabasePropertyStatusConfig(parentPageID, `{
    "Not started" = "default"
    "In progress" = "blue"
    "Done"        = "green"
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_database_property_status.test", "id"),
					resource.TestCheckResourceAttr("notion_database_property_status.test", "name", "Workflow"),
					resource.TestCheckResourceAttr("notion_database_property_status.test", "options.Not started", "default"),
					resource.TestCheckResourceAttr("notion_database_property_status.test", "options.In progress", "blue"),
					resource.TestCheckResourceAttr("notion_database_property_status.test", "options.Done", "green"),
				),
			},
			{
				Config: testAccDatabasePropertyStatusConfig(parentPageID, `{
    "Not started" = "default"
    "In progress" = "blue"
    "Blocked"     = "red"
    "Done"        = "green"
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_database_property_status.test", "options.Blocked", "red"),
				),
			},
		},
	})
}

func testAccDatabasePropertyStatusConfig(parentPageID, optionsBody string) string {
	return fmt.Sprintf(`
resource "notion_database" "status_test" {
  parent             = %q
  title              = "Status Test DB"
  title_column_title = "Name"
}

resource "notion_database_property_status" "test" {
  database = notion_database.status_test.id
  name     = "Workflow"
  options = %s
}
`, parentPageID, optionsBody)
}
