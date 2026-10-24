package provider

import (
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccSearchDataSource_InTrash checks that Notion accepts filter.in_trash
// (2026-07-15) on the 2022-06-28 search path. The trash contents of a shared
// workspace aren't predictable and trashed pages take time to reach the
// index, so only a successful read is asserted.
func TestAccSearchDataSource_InTrash(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "notion_search" "trash" {
  filter_object = "page"
  in_trash      = true
}
`,
				Check: resource.TestMatchResourceAttr("data.notion_search.trash", "results.#", regexp.MustCompile(`^[0-9]+$`)),
			},
		},
	})
}

// TestAccUsersDataSource_EmailVerified reads users over the direct-HTTP path.
// email_verified is null for bots and when the token lacks email
// capabilities, so the test asserts the list still loads.
func TestAccUsersDataSource_EmailVerified(t *testing.T) {
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
					logAttr(t, "data.notion_users.all", "users.0.email_verified"),
				),
			},
		},
	})
}
