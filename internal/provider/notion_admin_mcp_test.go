package provider

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func (f *fakeAdminAPI) serveMCP(w http.ResponseWriter, r *http.Request, parts []string, body map[string]interface{}) bool {
	if len(parts) < 2 || parts[1] != "mcp_client_connections" {
		return false
	}
	switch {
	case len(parts) == 2 && r.Method == http.MethodGet:
		f.areaState["mcp_list_query"] = r.URL.RawQuery
		all := []interface{}{
			map[string]interface{}{
				"client": map[string]string{"key": "claude-code", "name": "Claude Code", "type": "claude"}, "user": map[string]string{"id": "u1"},
				"workspaces": []map[string]string{{"id": "ws-1"}}, "is_enterprise_managed": false, "created_at": 1700000000000.0,
			},
			map[string]interface{}{
				"client": map[string]string{"key": "cursor", "name": "Cursor", "type": "cursor"}, "user": map[string]string{"id": "u2"},
				"workspaces": []map[string]string{{"id": "ws-1"}}, "is_enterprise_managed": true, "idp_issuer": "https://idp.example.com",
				"created_at": 1700000000000.0, "last_active_at": 1710000000000.0,
			},
		}
		f.writeJSON(w, f.page(r, "", all))
	case len(parts) == 3 && parts[2] == "revoke" && r.Method == http.MethodPost:
		f.areaState["mcp_revoke"] = body
		f.writeJSON(w, map[string]float64{"revoked_token_count": 2})
	case len(parts) == 3 && parts[2] == "enterprise_managed_access" && r.Method == http.MethodPut:
		calls, _ := f.areaState["mcp_access"].([]map[string]interface{})
		f.areaState["mcp_access"] = append(calls, body)
		f.writeJSON(w, map[string]interface{}{"access": body["access"], "revoked_token_count": 0})
	default:
		f.writeErr(w, 404, "object_not_found", "no MCP route")
	}
	return true
}

func mcpAccessCalls(f *fakeAdminAPI) []map[string]interface{} {
	calls, _ := f.areaState["mcp_access"].([]map[string]interface{})
	return calls
}

func TestUnitMCPEnterpriseAccess_DestroyKeepsDenial(t *testing.T) {
	f, _ := newFakeAdminAPI(t)
	f.routes = append(f.routes, f.serveMCP)
	prov := adminOnlyProvider(t, f.token)
	cfg := func(access string) string {
		return prov + fmt.Sprintf(`
resource "notion_mcp_enterprise_access" "ada" {
  workspace_id = "ws-1"
  user_id      = "u1"
  access       = %q
}
`, access)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			calls := mcpAccessCalls(f)
			if len(calls) != 3 || calls[2]["access"] != "denied" {
				return fmt.Errorf("destroy must leave the denial in place by default: %v", calls)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: cfg("denied"),
				Check: func(*terraform.State) error {
					calls := mcpAccessCalls(f)
					if len(calls) != 1 || calls[0]["access"] != "denied" || calls[0]["user_id"] != "u1" || calls[0]["workspace_id"] != "ws-1" {
						return fmt.Errorf("access calls: %v", calls)
					}
					return nil
				},
			},
			{Config: cfg("allowed")},
			{Config: cfg("denied")},
		},
	})
}

func TestUnitMCPEnterpriseAccess_RestoreOnDestroy(t *testing.T) {
	f, _ := newFakeAdminAPI(t)
	f.routes = append(f.routes, f.serveMCP)
	prov := adminOnlyProvider(t, f.token)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			calls := mcpAccessCalls(f)
			if len(calls) != 2 || calls[1]["access"] != "allowed" {
				return fmt.Errorf("restore_on_destroy should restore access: %v", calls)
			}
			return nil
		},
		Steps: []resource.TestStep{{Config: prov + `
resource "notion_mcp_enterprise_access" "ada" {
  workspace_id       = "ws-1"
  user_id            = "u1"
  access             = "denied"
  restore_on_destroy = true
}
`}},
	})
}

func TestUnitMCPConnectionsAndRevokeAction(t *testing.T) {
	f, _ := newFakeAdminAPI(t)
	f.routes = append(f.routes, f.serveMCP)
	f.pageSize = 1
	prov := adminOnlyProvider(t, f.token)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: prov + `
data "notion_mcp_client_connections" "ws" {
  workspace_ids = ["ws-1"]
}

action "notion_mcp_client_connection_revoke" "ada_claude" {
  config {
    client_key   = data.notion_mcp_client_connections.ws.connections[0].client_key
    user_id      = "u1"
    workspace_id = "ws-1"
  }
}

resource "terraform_data" "revoke" {
  input = "1"
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.notion_mcp_client_connection_revoke.ada_claude]
    }
  }
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.notion_mcp_client_connections.ws", "connections.#", "2"),
				resource.TestCheckResourceAttr("data.notion_mcp_client_connections.ws", "connections.0.client_key", "claude-code"),
				resource.TestCheckNoResourceAttr("data.notion_mcp_client_connections.ws", "connections.0.idp_issuer"),
				resource.TestCheckResourceAttr("data.notion_mcp_client_connections.ws", "connections.1.idp_issuer", "https://idp.example.com"),
				func(*terraform.State) error {
					if q, _ := f.areaState["mcp_list_query"].(string); !regexp.MustCompile(`workspace_ids%5B%5D=ws-1`).MatchString(q) {
						return fmt.Errorf("workspace filter not bracket-encoded: %q", q)
					}
					rev, _ := f.areaState["mcp_revoke"].(map[string]interface{})
					if rev["client_key"] != "claude-code" || rev["user_id"] != "u1" {
						return fmt.Errorf("revoke body: %v", rev)
					}
					return nil
				},
			),
		}},
	})
}
