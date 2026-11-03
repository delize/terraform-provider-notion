package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jomei/notionapi"
)

func TestAccDatabaseEntryResource(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}
	// Work inside a throwaway child page so anything a failed run leaves
	// behind is trashed with it.
	parentPageID = makeIsolatedParentPage(t, notionTestClient(t), "databaseentryresource")

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

func TestAccDatabaseEntryResourceWithMarkdown(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}
	// Work inside a throwaway child page so anything a failed run leaves
	// behind is trashed with it.
	parentPageID = makeIsolatedParentPage(t, notionTestClient(t), "databaseentryresourcewit")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDatabaseEntryWithMarkdownConfig(parentPageID, "Entry With Content", "## Details\n\nSome content here."),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_database_entry.test_md", "id"),
					resource.TestCheckResourceAttr("notion_database_entry.test_md", "title", "Entry With Content"),
					resource.TestCheckResourceAttrSet("notion_database_entry.test_md", "markdown"),
					resource.TestCheckResourceAttr("notion_database_entry.test_md", "select_properties.Stage", "Draft"),
					resource.TestCheckResourceAttr("notion_database_entry.test_md", "number_properties.Points", "5"),
				),
			},
			{
				// Read the properties back from Notion. A property the create
				// dropped would be missing from the refreshed maps.
				RefreshState: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_database_entry.test_md", "select_properties.Stage", "Draft"),
					resource.TestCheckResourceAttr("notion_database_entry.test_md", "number_properties.Points", "5"),
				),
			},
			{
				Config: testAccDatabaseEntryWithMarkdownConfig(parentPageID, "Entry With Content", "## Updated\n\n- [ ] Task 1\n- [x] Task 2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_database_entry.test_md", "markdown"),
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

func testAccDatabaseEntryWithMarkdownConfig(parentPageID, title, markdown string) string {
	return fmt.Sprintf(`
resource "notion_database" "test_entry_md_parent" {
  parent             = %q
  title              = "Markdown Entry Test DB"
  title_column_title = "Name"
}

resource "notion_database_property_select" "test_md_stage" {
  database = notion_database.test_entry_md_parent.id
  name     = "Stage"
  options = {
    "Draft" = "gray"
    "Live"  = "green"
  }
}

resource "notion_database_property_number" "test_md_points" {
  database = notion_database.test_entry_md_parent.id
  name     = "Points"
  format   = "number"
}

# Typed properties next to markdown. The create with markdown used to send
# only the title, which left these unset in Notion and showed a diff on
# the next plan.
resource "notion_database_entry" "test_md" {
  database          = notion_database.test_entry_md_parent.id
  title             = %q
  markdown          = %q
  select_properties = { "Stage" = "Draft" }
  number_properties = { "Points" = 5 }

  depends_on = [
    notion_database_property_select.test_md_stage,
    notion_database_property_number.test_md_points,
  ]
}
`, parentPageID, title, markdown)
}

// TestAccDatabaseEntryResource_ListAndPhoneProperties sets multi_select,
// relation, and phone values on entries created both ways (plain and with
// markdown), changes them, clears them, and imports.
func TestAccDatabaseEntryResource_ListAndPhoneProperties(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}
	client := notionTestClient(t)
	parentPageID = makeIsolatedParentPage(t, client, "databaseentrylists")

	const plain = "notion_database_entry.lists"
	const md = "notion_database_entry.lists_md"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDatabaseEntryListsConfig(parentPageID, `
  multi_select_properties = { "Tags" = ["A", "B"] }
  relation_properties     = { "Links" = [notion_database_entry.target_a.id, notion_database_entry.target_b.id] }
  phone_number_properties = { "Phone" = "+1 415 555 0100" }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(plain, "multi_select_properties.Tags.#", "2"),
					resource.TestCheckResourceAttr(plain, "multi_select_properties.Tags.0", "A"),
					resource.TestCheckResourceAttr(plain, "multi_select_properties.Tags.1", "B"),
					resource.TestCheckResourceAttr(plain, "relation_properties.Links.#", "2"),
					resource.TestCheckResourceAttrPair(plain, "relation_properties.Links.0", "notion_database_entry.target_a", "id"),
					resource.TestCheckResourceAttr(plain, "phone_number_properties.Phone", "+1 415 555 0100"),
					resource.TestCheckResourceAttr(md, "multi_select_properties.Tags.0", "C"),
					resource.TestCheckResourceAttrPair(md, "relation_properties.Links.0", "notion_database_entry.target_c", "id"),
					resource.TestCheckResourceAttr(md, "phone_number_properties.Phone", "+44 20 7946 0000"),
				),
			},
			{
				// Read everything back from Notion. A value either create
				// path dropped would be missing here.
				RefreshState: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(plain, "multi_select_properties.Tags.#", "2"),
					resource.TestCheckResourceAttr(plain, "relation_properties.Links.#", "2"),
					resource.TestCheckResourceAttr(plain, "phone_number_properties.Phone", "+1 415 555 0100"),
					resource.TestCheckResourceAttr(md, "multi_select_properties.Tags.#", "1"),
					resource.TestCheckResourceAttr(md, "relation_properties.Links.#", "1"),
					resource.TestCheckResourceAttr(md, "phone_number_properties.Phone", "+44 20 7946 0000"),
				),
			},
			{
				// Change every value. The relation ID is written with
				// hyphens and must stay in that form, since it's the same
				// page Notion returns without them.
				Config: testAccDatabaseEntryListsConfig(parentPageID, `
  multi_select_properties = { "Tags" = ["C", "A"] }
  relation_properties     = { "Links" = [local.target_c_hyphenated] }
  phone_number_properties = { "Phone" = "+1 415 555 0199" }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(plain, "multi_select_properties.Tags.0", "C"),
					resource.TestCheckResourceAttr(plain, "multi_select_properties.Tags.1", "A"),
					resource.TestCheckResourceAttr(plain, "relation_properties.Links.#", "1"),
					resource.TestMatchResourceAttr(plain, "relation_properties.Links.0", regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-`)),
					resource.TestCheckResourceAttr(plain, "phone_number_properties.Phone", "+1 415 555 0199"),
				),
			},
			{
				// Clear the relation with an empty list, and the tags and
				// phone by dropping their maps.
				Config: testAccDatabaseEntryListsConfig(parentPageID, `
  relation_properties = { "Links" = [] }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(plain, "relation_properties.Links.#", "0"),
					resource.TestCheckNoResourceAttr(plain, "multi_select_properties.%"),
					testAccCheckEntryListsCleared(t, client, plain),
				),
			},
			{
				// Set values again so the import has something to read.
				Config: testAccDatabaseEntryListsConfig(parentPageID, `
  multi_select_properties = { "Tags" = ["B"] }
  relation_properties     = { "Links" = [notion_database_entry.target_b.id] }
  phone_number_properties = { "Phone" = "+1 415 555 0100" }`),
			},
			{
				// Import reads no typed maps, since it can't know which
				// properties the config manages. The next apply writes
				// them from config.
				ResourceName:      plain,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"multi_select_properties",
					"relation_properties",
					"phone_number_properties",
				},
			},
		},
	})
}

// testAccCheckEntryListsCleared reads the entry straight from Notion and
// checks the cleared values are empty there, not only in state.
func testAccCheckEntryListsCleared(t *testing.T, client *notionapi.Client, addr string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return fmt.Errorf("%s not in state", addr)
		}
		page, err := client.Page.Get(context.Background(), notionapi.PageID(rs.Primary.ID))
		if err != nil {
			return err
		}
		if p, ok := page.Properties["Tags"].(*notionapi.MultiSelectProperty); !ok || len(p.MultiSelect) != 0 {
			return fmt.Errorf("tags not cleared in Notion: %#v", page.Properties["Tags"])
		}
		if p, ok := page.Properties["Links"].(*notionapi.RelationProperty); !ok || len(p.Relation) != 0 {
			return fmt.Errorf("links not cleared in Notion: %#v", page.Properties["Links"])
		}
		if p, ok := page.Properties["Phone"].(*notionapi.PhoneNumberProperty); !ok || p.PhoneNumber != "" {
			return fmt.Errorf("phone not cleared in Notion: %#v", page.Properties["Phone"])
		}
		t.Logf("Tags, Links, and Phone are empty in Notion")
		return nil
	}
}

// testAccDatabaseEntryListsConfig builds a target database with three
// entries and a main database with multi_select, relation, and phone
// properties. plainBody goes into the plain entry. The markdown entry keeps
// fixed values throughout.
func testAccDatabaseEntryListsConfig(parentPageID, plainBody string) string {
	return fmt.Sprintf(`
resource "notion_database" "targets" {
  parent             = %[1]q
  title              = "Relation Targets"
  title_column_title = "Name"
}

resource "notion_database_entry" "target_a" {
  database = notion_database.targets.id
  title    = "Target A"
}

resource "notion_database_entry" "target_b" {
  database = notion_database.targets.id
  title    = "Target B"
}

resource "notion_database_entry" "target_c" {
  database = notion_database.targets.id
  title    = "Target C"
}

locals {
  c                   = notion_database_entry.target_c.id
  target_c_hyphenated = join("-", [substr(local.c, 0, 8), substr(local.c, 8, 4), substr(local.c, 12, 4), substr(local.c, 16, 4), substr(local.c, 20, 12)])
}

resource "notion_database" "lists" {
  parent             = %[1]q
  title              = "List Properties Test DB"
  title_column_title = "Name"
}

resource "notion_database_property_multi_select" "tags" {
  database = notion_database.lists.id
  name     = "Tags"
  options = {
    "A" = "blue"
    "B" = "green"
    "C" = "red"
  }
}

resource "notion_database_property_relation" "links" {
  database         = notion_database.lists.id
  name             = "Links"
  related_database = notion_database.targets.id
}

resource "notion_database_property_phone_number" "phone" {
  database = notion_database.lists.id
  name     = "Phone"
}

resource "notion_database_entry" "lists" {
  database = notion_database.lists.id
  title    = "Plain entry"
%[2]s

  depends_on = [
    notion_database_property_multi_select.tags,
    notion_database_property_relation.links,
    notion_database_property_phone_number.phone,
  ]
}

resource "notion_database_entry" "lists_md" {
  database         = notion_database.lists.id
  title            = "Markdown entry"
  markdown_version = "v2"
  markdown         = "## Notes"

  multi_select_properties = { "Tags" = ["C"] }
  relation_properties     = { "Links" = [notion_database_entry.target_c.id] }
  phone_number_properties = { "Phone" = "+44 20 7946 0000" }

  depends_on = [
    notion_database_property_multi_select.tags,
    notion_database_property_relation.links,
    notion_database_property_phone_number.phone,
  ]
}
`, parentPageID, plainBody)
}

// TestAccDatabaseEntryResource_People assigns the integration's own bot
// user to a people property. A bot avoids notifying a real person, and
// Notion documents that bot user IDs are accepted.
func TestAccDatabaseEntryResource_People(t *testing.T) {
	parentPageID := os.Getenv("NOTION_TEST_PARENT_PAGE_ID")
	if parentPageID == "" {
		t.Skip("NOTION_TEST_PARENT_PAGE_ID not set")
	}
	client := notionTestClient(t)
	me, err := client.User.Me(context.Background())
	if err != nil {
		t.Fatalf("reading the integration's bot user: %v", err)
	}
	botID := string(me.ID)
	parentPageID = makeIsolatedParentPage(t, client, "databaseentrypeople")

	const addr = "notion_database_entry.owned"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDatabaseEntryPeopleConfig(parentPageID, fmt.Sprintf("[%q]", botID)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "people_properties.Owner.#", "1"),
					resource.TestCheckResourceAttr(addr, "people_properties.Owner.0", botID),
				),
			},
			{
				RefreshState: true,
				Check:        resource.TestCheckResourceAttr(addr, "people_properties.Owner.0", botID),
			},
			{
				Config: testAccDatabaseEntryPeopleConfig(parentPageID, "[]"),
				Check:  resource.TestCheckResourceAttr(addr, "people_properties.Owner.#", "0"),
			},
		},
	})
}

func testAccDatabaseEntryPeopleConfig(parentPageID, owners string) string {
	return fmt.Sprintf(`
resource "notion_database" "people" {
  parent             = %q
  title              = "People Test DB"
  title_column_title = "Name"
}

resource "notion_database_property_people" "owner" {
  database = notion_database.people.id
  name     = "Owner"
}

resource "notion_database_entry" "owned" {
  database          = notion_database.people.id
  title             = "Owned entry"
  people_properties = { "Owner" = %s }

  depends_on = [notion_database_property_people.owner]
}
`, parentPageID, owners)
}
