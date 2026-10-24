package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Plan-only checks through real Terraform for the tab block config rules.
// No Notion workspace needed.

const blockUnitProvider = `
provider "notion" {
  token = "unit-test-token"
}
`

func blockUnitStep(config, errPattern string) resource.TestStep {
	step := resource.TestStep{Config: blockUnitProvider + config, PlanOnly: true}
	if errPattern == "" {
		step.ExpectNonEmptyPlan = true
	} else {
		step.ExpectError = regexp.MustCompile(errPattern)
	}
	return step
}

func TestUnitBlockTab_RequiresItems(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{blockUnitStep(`
resource "notion_block" "t" {
  parent_id = "00000000000000000000000000000000"
  type      = "tab"
}`, `A tab block needs at least one entry in tab_items`)},
	})
}

func TestUnitBlockTab_EmptyItems(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{blockUnitStep(`
resource "notion_block" "t" {
  parent_id = "00000000000000000000000000000000"
  type      = "tab"
  tab_items = []
}`, `at least one tab item`)},
	})
}

func TestUnitBlockTab_RejectsRichText(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{blockUnitStep(`
resource "notion_block" "t" {
  parent_id = "00000000000000000000000000000000"
  type      = "tab"
  rich_text = "nope"
  tab_items = [{ label = "One" }]
}`, `rich_text has no effect on a tab block`)},
	})
}

func TestUnitBlockTabItems_OnlyOnTab(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{blockUnitStep(`
resource "notion_block" "p" {
  parent_id = "00000000000000000000000000000000"
  type      = "paragraph"
  rich_text = "hi"
  tab_items = [{ label = "One" }]
}`, `tab_items only applies to blocks of type tab`)},
	})
}

func TestUnitBlockTabsTypeRemoved(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{blockUnitStep(`
resource "notion_block" "t" {
  parent_id = "00000000000000000000000000000000"
  type      = "tabs"
}`, `Invalid Block Type`)},
	})
}

func TestUnitBlockTabAndHeading4_ValidPlan(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{blockUnitStep(`
resource "notion_block" "h4" {
  parent_id     = "00000000000000000000000000000000"
  type          = "heading_4"
  rich_text     = "Details"
  is_toggleable = true
}

resource "notion_block" "t" {
  parent_id = "00000000000000000000000000000000"
  type      = "tab"
  tab_items = [
    { label = "Overview", icon = "📋" },
    { label = "Details" },
  ]
}`, ``)},
	})
}
