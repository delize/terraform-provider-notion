package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// importIDWithWorkspace builds '<workspace_id>/<id>' from state for import.
func importIDWithWorkspace(addr, ws string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return "", fmt.Errorf("%s not in state", addr)
		}
		return ws + "/" + rs.Primary.ID, nil
	}
}

// Admin API acceptance tests run against a real Enterprise organization.
// They skip unless NOTION_ADMIN_TOKEN (permission-group:read and :write,
// workspace:read, user:read) and NOTION_TEST_ADMIN_WORKSPACE_ID are set, so
// CI skips them. The member test also needs NOTION_TEST_ADMIN_USER_ID, a
// full workspace member (not a guest) it can add and remove.

func adminAccPreCheck(t *testing.T) string {
	t.Helper()
	if os.Getenv("NOTION_ADMIN_TOKEN") == "" {
		t.Skip("NOTION_ADMIN_TOKEN not set")
	}
	ws := os.Getenv("NOTION_TEST_ADMIN_WORKSPACE_ID")
	if ws == "" {
		t.Skip("NOTION_TEST_ADMIN_WORKSPACE_ID not set")
	}
	return ws
}

func TestAccPermissionGroupResource(t *testing.T) {
	ws := adminAccPreCheck(t)
	name := fmt.Sprintf("tf-acc-test-group-%s", testRunSuffix())

	cfg := func(n string) string {
		return fmt.Sprintf(`
resource "notion_permission_group" "test" {
  workspace_id = %q
  name         = %q
}
`, ws, n)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("notion_permission_group.test", "id"),
					resource.TestCheckResourceAttr("notion_permission_group.test", "name", name),
				),
			},
			{
				Config: cfg(name + "-renamed"),
				Check:  resource.TestCheckResourceAttr("notion_permission_group.test", "name", name+"-renamed"),
			},
			{
				ResourceName:      "notion_permission_group.test",
				ImportState:       true,
				ImportStateIdFunc: importIDWithWorkspace("notion_permission_group.test", ws),
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccPermissionGroupMemberResource(t *testing.T) {
	ws := adminAccPreCheck(t)
	userID := os.Getenv("NOTION_TEST_ADMIN_USER_ID")
	if userID == "" {
		t.Skip("NOTION_TEST_ADMIN_USER_ID not set")
	}
	name := fmt.Sprintf("tf-acc-test-members-%s", testRunSuffix())

	cfg := func(role string) string {
		return fmt.Sprintf(`
resource "notion_permission_group" "test" {
  workspace_id = %[1]q
  name         = %[2]q
}

resource "notion_permission_group_member" "test" {
  workspace_id = %[1]q
  group_id     = notion_permission_group.test.id
  user_id      = %[3]q
  role         = %[4]q
}
`, ws, name, userID, role)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("member"),
				Check:  resource.TestCheckResourceAttr("notion_permission_group_member.test", "role", "member"),
			},
			{
				Config: cfg("owner"),
				Check:  resource.TestCheckResourceAttr("notion_permission_group_member.test", "role", "owner"),
			},
			{
				ResourceName:      "notion_permission_group_member.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccAdminDataSources(t *testing.T) {
	ws := adminAccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`
data "notion_workspaces" "all" {}

data "notion_permission_groups" "ws" {
  workspace_id = %[1]q
}

data "notion_admin_users" "ws" {
  workspace_id = %[1]q
}
`, ws),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("data.notion_workspaces.all", "workspaces.0.id"),
				resource.TestCheckResourceAttrSet("data.notion_permission_groups.ws", "groups.#"),
				resource.TestCheckResourceAttrSet("data.notion_admin_users.ws", "users.0.id"),
			),
		}},
	})
}
