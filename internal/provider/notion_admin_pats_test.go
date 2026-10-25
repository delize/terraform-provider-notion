package provider

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func (f *fakeAdminAPI) servePATs(w http.ResponseWriter, r *http.Request, parts []string, _ map[string]interface{}) bool {
	if len(parts) < 4 || parts[1] != "spaces" || parts[3] != "personal_access_tokens" {
		return false
	}
	switch {
	case len(parts) == 4 && r.Method == http.MethodGet:
		f.areaState["pat_query"] = r.URL.RawQuery
		all := []interface{}{
			map[string]interface{}{"id": "pat-1", "name": "ci", "status": "active", "creator": map[string]string{"id": "u1", "email": "ada@example.com"},
				"created_at": 1700000000000.0, "expires_at": 1800000000000.0},
			map[string]interface{}{"id": "pat-2", "status": "revoked", "creator": map[string]string{"id": "u2"}, "created_at": 1700000000000.0,
				"revoked_at": 1750000000000.0, "revoked_by": map[string]string{"type": "organization_bot", "id": "org-bot"}},
		}
		f.writeJSON(w, f.page(r, "", all))
	case len(parts) == 5 && r.Method == http.MethodDelete:
		f.areaState["pat_revoked"] = parts[4]
		f.writeJSON(w, map[string]interface{}{})
	default:
		f.writeErr(w, 404, "object_not_found", "no PAT route")
	}
	return true
}

func TestUnitPATsDataSourceAndRevokeAction(t *testing.T) {
	f, _ := newFakeAdminAPI(t)
	f.routes = append(f.routes, f.servePATs)
	f.pageSize = 1
	prov := adminOnlyProvider(t, f.token)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: prov + `
data "notion_personal_access_tokens" "ws" {
  workspace_id = "ws-1"
  statuses     = ["active", "revoked"]
  search       = "ci"
}

action "notion_personal_access_token_revoke" "ci" {
  config {
    workspace_id = "ws-1"
    bot_id       = data.notion_personal_access_tokens.ws.tokens[0].id
  }
}

resource "terraform_data" "revoke" {
  input = "1"
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.notion_personal_access_token_revoke.ci]
    }
  }
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.notion_personal_access_tokens.ws", "tokens.#", "2"),
				resource.TestCheckResourceAttr("data.notion_personal_access_tokens.ws", "tokens.0.creator_email", "ada@example.com"),
				resource.TestCheckResourceAttr("data.notion_personal_access_tokens.ws", "tokens.0.expires_at", "1800000000000"),
				resource.TestCheckNoResourceAttr("data.notion_personal_access_tokens.ws", "tokens.1.name"),
				resource.TestCheckResourceAttr("data.notion_personal_access_tokens.ws", "tokens.1.revoked_by_id", "org-bot"),
				func(*terraform.State) error {
					q, _ := f.areaState["pat_query"].(string)
					if !regexp.MustCompile(`status%5B%5D=active&status%5B%5D=revoked`).MatchString(q) || !regexp.MustCompile(`search=ci`).MatchString(q) {
						return fmt.Errorf("filters not encoded as documented: %q", q)
					}
					if f.areaState["pat_revoked"] != "pat-1" {
						return fmt.Errorf("revoke action hit %v", f.areaState["pat_revoked"])
					}
					return nil
				},
			),
		}},
	})
}

func TestUnitPATsDataSource_InvalidStatus(t *testing.T) {
	prov := adminOnlyProvider(t, "adm_test")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: prov + `
data "notion_personal_access_tokens" "ws" {
  workspace_id = "ws-1"
  statuses     = ["live"]
}`,
			ExpectError: regexp.MustCompile(`"live" is not valid`),
		}},
	})
}
