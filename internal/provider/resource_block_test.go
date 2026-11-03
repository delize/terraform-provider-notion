package provider

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccBlockHeading4AndTabs covers heading_4 (2026-03-30) and tab blocks
// with tab item icons (2026-03-25, 2026-03-30), which go over direct HTTP.
// Steps: create with content inside a tab, then relabel, change an icon, add
// a tab, and recolor the heading, then check that removing an icon is
// refused (the API can't), then import both, then shrink the tab.
func TestAccBlockHeading4AndTabs(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}
	parentPageID = makeIsolatedParentPage(t, notionTestClient(t), "blocks-h4-tabs")
	title := fmt.Sprintf("tf-acc-test-blocks-h4-tabs-%s", testRunSuffix())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccBlockHeading4TabsConfig(parentPageID, title, "", `[
    { label = "Overview", icon = "📋" },
    { label = "Details", icon = "🔍" },
  ]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_block.h4", "id"),
					resource.TestCheckResourceAttr("notion_block.h4", "type", "heading_4"),
					resource.TestCheckResourceAttr("notion_block.h4", "rich_text", "Section four"),
					resource.TestCheckResourceAttr("notion_block.h4", "is_toggleable", "true"),
					resource.TestCheckResourceAttr("notion_block.h4", "color", ""),
					resource.TestCheckResourceAttr("notion_block.tabs", "type", "tab"),
					resource.TestCheckResourceAttr("notion_block.tabs", "has_children", "true"),
					resource.TestCheckResourceAttr("notion_block.tabs", "tab_items.#", "2"),
					resource.TestCheckResourceAttr("notion_block.tabs", "tab_items.0.label", "Overview"),
					resource.TestCheckResourceAttr("notion_block.tabs", "tab_items.0.icon", "📋"),
					resource.TestCheckResourceAttrSet("notion_block.tabs", "tab_items.0.id"),
					resource.TestCheckResourceAttrSet("notion_block.tabs", "tab_items.1.id"),
					resource.TestCheckResourceAttrPair("notion_block.tab_content", "parent_id", "notion_block.tabs", "tab_items.0.id"),
				),
			},
			{
				Config: testAccBlockHeading4TabsConfig(parentPageID, title, "blue", `[
    { label = "Overview", icon = "📁" },
    { label = "More details", icon = "🔍" },
    { label = "Notes", icon = "📝" },
  ]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_block.h4", "color", "blue"),
					resource.TestCheckResourceAttr("notion_block.tabs", "tab_items.#", "3"),
					resource.TestCheckResourceAttr("notion_block.tabs", "tab_items.0.icon", "📁"),
					resource.TestCheckResourceAttr("notion_block.tabs", "tab_items.1.label", "More details"),
					resource.TestCheckResourceAttr("notion_block.tabs", "tab_items.2.label", "Notes"),
					resource.TestCheckResourceAttrSet("notion_block.tabs", "tab_items.2.id"),
					// The content block stays under the first tab, so no replace.
					resource.TestCheckResourceAttrPair("notion_block.tab_content", "parent_id", "notion_block.tabs", "tab_items.0.id"),
				),
			},
			{
				// Update a block takes only an icon object, so removing an
				// icon is refused before anything is sent.
				Config: testAccBlockHeading4TabsConfig(parentPageID, title, "blue", `[
    { label = "Overview" },
    { label = "More details", icon = "🔍" },
    { label = "Notes", icon = "📝" },
  ]`),
				ExpectError: regexp.MustCompile(`can't remove a tab icon`),
			},
			{
				ResourceName:      "notion_block.h4",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "notion_block.tabs",
				ImportState:       true,
				ImportStateVerify: true,
				// after is a create-time placement hint the API doesn't return.
				ImportStateVerifyIgnore: []string{"after"},
			},
			{
				Config: testAccBlockHeading4TabsConfig(parentPageID, title, "blue", `[
    { label = "Overview", icon = "📁" },
  ]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_block.tabs", "tab_items.#", "1"),
					resource.TestCheckResourceAttrPair("notion_block.tab_content", "parent_id", "notion_block.tabs", "tab_items.0.id"),
				),
			},
		},
	})
}

func testAccBlockHeading4TabsConfig(parentPageID, title, color, tabItems string) string {
	colorLine := ""
	if color != "" {
		colorLine = fmt.Sprintf("  color         = %q\n", color)
	}
	return fmt.Sprintf(`
resource "notion_page" "test" {
  parent_page_id = %q
  title          = %q
}

resource "notion_block" "h4" {
  parent_id     = notion_page.test.id
  type          = "heading_4"
  rich_text     = "Section four"
  is_toggleable = true
%s}

resource "notion_block" "tabs" {
  parent_id = notion_page.test.id
  type      = "tab"
  after     = notion_block.h4.id
  tab_items = %s
}

resource "notion_block" "tab_content" {
  parent_id = notion_block.tabs.tab_items[0].id
  type      = "paragraph"
  rich_text = "Content inside the first tab"
}
`, parentPageID, title, colorLine, tabItems)
}

// TestAccBlockDefaultColor checks that a heading_1 with no color plans empty
// after create even though Notion reports its color as "default", that an
// explicit "default" round-trips, and that removing a color resets it in
// Notion. The import step must read the unset color back as "".
func TestAccBlockDefaultColor(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}
	parentPageID := makeIsolatedParentPage(t, notionTestClient(t), "block-color")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourcesTrashed(t),
		Steps: []resource.TestStep{
			{
				Config: testAccBlockDefaultColorConfig(parentPageID, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_block.heading", "color", ""),
					resource.TestCheckResourceAttr("notion_block.explicit", "color", "default"),
				),
			},
			{
				Config: testAccBlockDefaultColorConfig(parentPageID, "red"),
				Check:  resource.TestCheckResourceAttr("notion_block.heading", "color", "red"),
			},
			{
				// Removing the color must reset it. If Notion kept red, the
				// update would fail with an inconsistent result.
				Config: testAccBlockDefaultColorConfig(parentPageID, ""),
				Check:  resource.TestCheckResourceAttr("notion_block.heading", "color", ""),
			},
			{
				ResourceName:      "notion_block.heading",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccBlockDefaultColorConfig(parentPageID, headingColor string) string {
	colorLine := ""
	if headingColor != "" {
		colorLine = fmt.Sprintf("  color     = %q\n", headingColor)
	}
	return fmt.Sprintf(`
resource "notion_page" "test" {
  parent_page_id = %q
  title          = "Block color test"
}

resource "notion_block" "heading" {
  parent_id = notion_page.test.id
  type      = "heading_1"
  rich_text = "No color"
%s}

resource "notion_block" "explicit" {
  parent_id = notion_page.test.id
  type      = "paragraph"
  rich_text = "Explicit default"
  after     = notion_block.heading.id
  color     = "default"
}
`, parentPageID, colorLine)
}

// TestAccBlockCalloutIcons walks a callout through a native icon
// (2026-03-25), then an emoji with a color, then checks that removing the
// icon is refused (the API can't), then imports it, then clears the color
// while keeping the emoji. The callout sits in an isolated page that
// cleanup trashes.
func TestAccBlockCalloutIcons(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}
	parentID := makeIsolatedParentPage(t, notionTestClient(t), "callout-icons")

	const addr = "notion_block.callout"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccBlockCalloutConfig(parentID, `
  icon_name  = "pizza"
  icon_color = "blue"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "type", "callout"),
					resource.TestCheckResourceAttr(addr, "rich_text", "Callout with an icon"),
					resource.TestCheckResourceAttr(addr, "icon_name", "pizza"),
					resource.TestCheckResourceAttr(addr, "icon_color", "blue"),
					resource.TestCheckResourceAttr(addr, "icon", ""),
					resource.TestCheckResourceAttr(addr, "color", ""),
				),
			},
			{
				Config: testAccBlockCalloutConfig(parentID, `
  icon  = "💡"
  color = "yellow_background"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(addr, "icon_name"),
					resource.TestCheckNoResourceAttr(addr, "icon_color"),
					resource.TestCheckResourceAttr(addr, "icon", "💡"),
					resource.TestCheckResourceAttr(addr, "color", "yellow_background"),
				),
			},
			{
				// Update a block takes only an icon object, so removing the
				// icon is refused before anything is sent.
				Config:      testAccBlockCalloutConfig(parentID, ""),
				ExpectError: regexp.MustCompile(`can't remove a callout icon`),
			},
			{
				Config:            testAccBlockCalloutConfig(parentID, `  icon = "💡"`),
				ResourceName:      addr,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccBlockCalloutConfig(parentID, `  icon = "💡"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "icon", "💡"),
					resource.TestCheckResourceAttr(addr, "color", ""),
				),
			},
		},
	})
}

func testAccBlockCalloutConfig(parentID, extra string) string {
	return fmt.Sprintf(`
resource "notion_block" "callout" {
  parent_id = %q
  type      = "callout"
  rich_text = "Callout with an icon"
%s
}
`, parentID, extra)
}
