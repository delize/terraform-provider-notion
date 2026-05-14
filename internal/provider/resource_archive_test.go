package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jomei/notionapi"
)

// notionTestClient builds a Notion API client from NOTION_TOKEN. Acceptance
// tests that call this should already be guarded by a token check.
func notionTestClient(t *testing.T) *notionapi.Client {
	t.Helper()
	token := os.Getenv("NOTION_TOKEN")
	if token == "" {
		t.Fatal("NOTION_TOKEN must be set for acceptance tests")
	}
	return notionapi.NewClient(notionapi.Token(token))
}

// findRootPageID returns the ID of any page the integration token has access
// to, suitable for use as a parent for an ephemeral test page. Honors
// NOTION_TEST_PARENT_PAGE_ID for deterministic CI runs; otherwise discovers via
// the Search API.
func findRootPageID(t *testing.T, client *notionapi.Client) string {
	t.Helper()
	if id := os.Getenv("NOTION_TEST_PARENT_PAGE_ID"); id != "" {
		return normalizeID(id)
	}

	resp, err := client.Search.Do(context.Background(), &notionapi.SearchRequest{
		Filter: notionapi.SearchFilter{
			Property: "object",
			Value:    "page",
		},
		PageSize: 100,
	})
	if err != nil {
		t.Fatalf("searching for an accessible parent page: %v", err)
	}

	for _, obj := range resp.Results {
		page, ok := obj.(*notionapi.Page)
		if !ok || page.Archived {
			continue
		}
		switch page.Parent.Type {
		case notionapi.ParentTypeWorkspace, notionapi.ParentTypePageID:
			return normalizeID(string(page.ID))
		}
	}

	t.Skip("no non-archived workspace/page-parented pages are shared with the integration; share at least one page with the integration or set NOTION_TEST_PARENT_PAGE_ID")
	return ""
}

// makeIsolatedParentPage creates a fresh page via the Notion API to act as the
// parent for resources under test, and registers a cleanup that archives it
// when the test ends. Returns the new page's normalized ID.
func makeIsolatedParentPage(t *testing.T, client *notionapi.Client, label string) string {
	t.Helper()
	rootID := findRootPageID(t, client)
	ctx := context.Background()

	titleText := fmt.Sprintf("tf-acc-test-%s-%d", label, os.Getpid())
	page, err := client.Page.Create(ctx, &notionapi.PageCreateRequest{
		Parent: notionapi.Parent{
			Type:   notionapi.ParentTypePageID,
			PageID: notionapi.PageID(rootID),
		},
		Properties: notionapi.Properties{
			"title": notionapi.TitleProperty{
				Type: notionapi.PropertyTypeTitle,
				Title: []notionapi.RichText{
					{Type: notionapi.ObjectTypeText, Text: &notionapi.Text{Content: titleText}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("creating isolated parent page: %v", err)
	}

	parentID := normalizeID(string(page.ID))
	t.Cleanup(func() {
		if _, err := client.Block.Delete(context.Background(), notionapi.BlockID(parentID)); err != nil {
			t.Logf("cleanup: failed to archive parent page %s: %v", parentID, err)
		}
	})
	return parentID
}

// testAccCheckResourcesArchived returns a CheckDestroy that verifies every
// notion_page, notion_database, and notion_database_entry resource in state
// was actually archived in Notion after destroy.
func testAccCheckResourcesArchived(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client := notionTestClient(t)
		ctx := context.Background()

		for name, rs := range s.RootModule().Resources {
			id := rs.Primary.ID
			if id == "" {
				continue
			}
			switch rs.Type {
			case "notion_page", "notion_database_entry":
				page, err := client.Page.Get(ctx, notionapi.PageID(id))
				if err != nil {
					return fmt.Errorf("fetching %s (%s) after destroy: %w", name, id, err)
				}
				if !page.Archived {
					return fmt.Errorf("%s (%s) was not archived after destroy", name, id)
				}
			case "notion_database":
				db, err := client.Database.Get(ctx, notionapi.DatabaseID(id))
				if err != nil {
					return fmt.Errorf("fetching %s (%s) after destroy: %w", name, id, err)
				}
				if !db.Archived {
					return fmt.Errorf("%s (%s) was not archived after destroy", name, id)
				}
			}
		}
		return nil
	}
}

// TestAccPageResource_DeleteWithChildBlocks reproduces the scenario from
// https://github.com/delize/terraform-provider-notion/issues/2: a page with
// child blocks fails to delete because the archive request was sending
// `properties: null` instead of an object.
func TestAccPageResource_DeleteWithChildBlocks(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}
	client := notionTestClient(t)
	parentPageID := makeIsolatedParentPage(t, client, "page-delete")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourcesArchived(t),
		Steps: []resource.TestStep{
			{
				Config: testAccPageWithBlocksConfig(parentPageID, "Archive Test Page"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_page.test", "id"),
					resource.TestCheckResourceAttrSet("notion_block.heading", "id"),
					resource.TestCheckResourceAttrSet("notion_block.intro", "id"),
				),
			},
		},
	})
}

// TestAccDatabaseResource_Delete verifies the database delete path archives
// the database without error.
func TestAccDatabaseResource_Delete(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}
	client := notionTestClient(t)
	parentPageID := makeIsolatedParentPage(t, client, "db-delete")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourcesArchived(t),
		Steps: []resource.TestStep{
			{
				Config: testAccDatabaseResourceConfig(parentPageID, "Archive Test DB", "Name"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_database.test", "id"),
				),
			},
		},
	})
}

// TestAccDatabaseEntryResource_Delete is a regression test for commit 81d553a
// (the same root cause as issue #2, applied previously to database entries).
func TestAccDatabaseEntryResource_Delete(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}
	client := notionTestClient(t)
	parentPageID := makeIsolatedParentPage(t, client, "entry-delete")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourcesArchived(t),
		Steps: []resource.TestStep{
			{
				Config: testAccDatabaseEntryResourceConfig(parentPageID, "Archive Test Entry"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_database_entry.test", "id"),
				),
			},
		},
	})
}

func testAccPageWithBlocksConfig(parentPageID, title string) string {
	return fmt.Sprintf(`
resource "notion_page" "test" {
  parent_page_id = %q
  title          = %q
}

resource "notion_block" "heading" {
  parent_id = notion_page.test.id
  type      = "heading_1"
  rich_text = "Welcome"
  color     = "default"
}

resource "notion_block" "intro" {
  parent_id = notion_page.test.id
  type      = "paragraph"
  rich_text = "Hello, world!"
  after     = notion_block.heading.id
  color     = "default"
}
`, parentPageID, title)
}
