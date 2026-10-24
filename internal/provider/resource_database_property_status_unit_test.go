package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// These run through real Terraform but stop at plan, so they need no Notion
// workspace. They cover the option_groups config validation.

const statusUnitProvider = `
provider "notion" {
  token = "unit-test-token"
}
`

func TestUnitStatusOptionGroups_InvalidGroup(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: statusUnitProvider + `
resource "notion_database_property_status" "test" {
  database = "00000000000000000000000000000000"
  name     = "Workflow"
  options  = { "Done" = "green" }
  option_groups = { "Done" = "Finished" }
}`,
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`"Finished" is not a status group`),
		}},
	})
}

func TestUnitStatusOptionGroups_UnknownOption(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: statusUnitProvider + `
resource "notion_database_property_status" "test" {
  database = "00000000000000000000000000000000"
  name     = "Workflow"
  options  = { "Done" = "green" }
  option_groups = { "Blocked" = "In progress" }
}`,
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`"Blocked" is in option_groups but not in options`),
		}},
	})
}

// A valid config plans cleanly with default_option unknown until apply.
func TestUnitStatusOptionGroups_ValidPlans(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: statusUnitProvider + `
resource "notion_database_property_status" "test" {
  database = "00000000000000000000000000000000"
  name     = "Workflow"
  options  = { "Done" = "green", "Blocked" = "red" }
  option_groups = { "Done" = "Complete" }
}`,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		}},
	})
}
