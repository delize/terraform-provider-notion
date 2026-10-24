package provider

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func (f *fakeAdminAPI) serveManagedUsers(w http.ResponseWriter, r *http.Request, parts []string, body map[string]interface{}) bool {
	if len(parts) != 3 || parts[1] != "managed_users" || parts[2] != "revoke_session" || r.Method != http.MethodPost {
		return false
	}
	user, _ := body["user"].(map[string]interface{})
	revoked, _ := f.areaState["revoked_sessions"].([]map[string]interface{})
	f.areaState["revoked_sessions"] = append(revoked, user)
	f.writeJSON(w, map[string]string{"status": "revoked"})
	return true
}

func TestUnitRevokeUserSessionsAction(t *testing.T) {
	f, _ := newFakeAdminAPI(t)
	f.routes = append(f.routes, f.serveManagedUsers)
	prov := adminOnlyProvider(t, f.token)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: prov + `
action "notion_revoke_user_sessions" "ada" {
  config {
    email = "ada@example.com"
  }
}

resource "terraform_data" "offboard" {
  input = "ada"
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.notion_revoke_user_sessions.ada]
    }
  }
}
`,
				Check: func(*terraform.State) error {
					got, _ := f.areaState["revoked_sessions"].([]map[string]interface{})
					if len(got) != 1 || got[0]["type"] != "email" || got[0]["email"] != "ada@example.com" {
						return fmt.Errorf("revoke requests: %v", got)
					}
					return nil
				},
			},
		},
	})
}

func TestUnitRevokeUserSessionsAction_NeedsOneIdentifier(t *testing.T) {
	prov := adminOnlyProvider(t, "adm_test")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: prov + `
action "notion_revoke_user_sessions" "x" {
  config {
    user_id = "u1"
    email   = "ada@example.com"
  }
}

resource "terraform_data" "t" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.notion_revoke_user_sessions.x]
    }
  }
}`,
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`Set exactly one of user_id or email`),
		}},
	})
}
