package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jomei/notionapi"
)

// TestAccUsersDataSource verifies the users list data source returns at
// least one user (the integration's own bot).
func TestAccUsersDataSource(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "notion_users" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.notion_users.all", "users.0.id"),
					resource.TestCheckResourceAttrSet("data.notion_users.all", "users.0.type"),
				),
			},
		},
	})
}

// TestAccSearchDataSource creates an isolated parent page via the API, waits
// for Notion's eventually-consistent search index to pick it up, then runs
// the search data source and asserts the page appears in results. The wait
// step is required because /v1/search has multi-second indexing lag for
// just-created pages.
func TestAccSearchDataSource(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}
	client := notionTestClient(t)
	parentPageID := makeIsolatedParentPage(t, client, "search")
	expectedTitle := fmt.Sprintf("tf-acc-test-search-%s", testRunSuffix())

	waitForSearchIndex(t, client, expectedTitle, parentPageID, 60*time.Second)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "notion_search" "by_title" {
  query         = %q
  filter_object = "page"
}
`, expectedTitle),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("data.notion_search.by_title", "results.#", regexp.MustCompile(`^[1-9][0-9]*$`)),
					checkSearchContainsID("data.notion_search.by_title", parentPageID),
				),
			},
		},
	})
}

// waitForSearchIndex polls /v1/search until the given page ID appears in the
// results for the given query, or fails the test on timeout. Notion's search
// index is eventually-consistent — newly-created pages typically appear
// within 5–15 seconds.
func waitForSearchIndex(t *testing.T, client *notionapi.Client, query, expectedID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	ctx := context.Background()
	for {
		resp, err := client.Search.Do(ctx, &notionapi.SearchRequest{
			Query: query,
			Filter: notionapi.SearchFilter{
				Property: "object",
				Value:    "page",
			},
			PageSize: 100,
		})
		if err != nil {
			t.Fatalf("waitForSearchIndex: search failed: %v", err)
		}
		for _, obj := range resp.Results {
			if p, ok := obj.(*notionapi.Page); ok && normalizeID(string(p.ID)) == expectedID {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("waitForSearchIndex: page %s not indexed within %s for query %q", expectedID, timeout, query)
		}
		time.Sleep(3 * time.Second)
	}
}

// TestAccBlocksDataSource creates a page with two blocks and verifies the
// data source lists them in document order with their plain text.
func TestAccBlocksDataSource(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}
	client := notionTestClient(t)
	parentPageID := makeIsolatedParentPage(t, client, "blocks")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourcesTrashed(t),
		Steps: []resource.TestStep{
			{
				Config: testAccBlocksDataSourceConfig(parentPageID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.notion_blocks.children", "blocks.#", "2"),
					resource.TestCheckResourceAttr("data.notion_blocks.children", "blocks.0.plain_text", "Welcome"),
					resource.TestCheckResourceAttr("data.notion_blocks.children", "blocks.1.plain_text", "Hello, world!"),
				),
			},
		},
	})
}

func testAccBlocksDataSourceConfig(parentPageID string) string {
	return fmt.Sprintf(`
resource "notion_page" "test" {
  parent_page_id = %q
  title          = "Blocks DS Test"
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

data "notion_blocks" "children" {
  parent_id  = notion_page.test.id
  depends_on = [notion_block.heading, notion_block.intro]
}
`, parentPageID)
}

// checkSearchContainsID asserts that the named search data source's results
// contain a result with the given ID. Walks the flat-key state because we
// can't index into list-of-objects via TestCheckResourceAttr directly.
func checkSearchContainsID(resourceName, id string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceName)
		}
		for k, v := range rs.Primary.Attributes {
			if strings.HasSuffix(k, ".id") && v == id {
				return nil
			}
		}
		return fmt.Errorf("search results did not contain id %s", id)
	}
}
