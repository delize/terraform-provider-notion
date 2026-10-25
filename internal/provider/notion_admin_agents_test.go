package provider

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type fakeAgent struct {
	ID, Name, Status, Type string
	CreditLimit            *float64
	CreditLimitCalls       []interface{} // body values sent to PUT credit_limit
	Perms                  map[string]map[string]interface{}
	Deleted                bool
}

type fakeAgentWorkspace struct {
	Policy      string
	PolicyCalls []map[string]interface{}
	LimitCalls  []interface{}
}

func (f *fakeAdminAPI) agents() map[string]*fakeAgent {
	m, ok := f.areaState["agents"].(map[string]*fakeAgent)
	if !ok {
		m = map[string]*fakeAgent{}
		f.areaState["agents"] = m
	}
	return m
}

func (f *fakeAdminAPI) agentWorkspace() *fakeAgentWorkspace {
	w, ok := f.areaState["agent_ws"].(*fakeAgentWorkspace)
	if !ok {
		w = &fakeAgentWorkspace{}
		f.areaState["agent_ws"] = w
	}
	return w
}

func permKey(p map[string]interface{}) string {
	return fmt.Sprint(p["type"], ":", p["user_id"], p["group_id"])
}

func (a *fakeAgent) permList() []interface{} {
	keys := make([]string, 0, len(a.Perms))
	for k := range a.Perms {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := []interface{}{}
	for _, k := range keys {
		out = append(out, a.Perms[k])
	}
	return out
}

func (a *fakeAgent) usage(periods []interface{}) map[string]interface{} {
	out := map[string]interface{}{
		"id": a.ID, "name": a.Name, "status": a.Status, "type": a.Type,
		"runs_completed": 12.0, "total_credits_used": 340.0, "has_more": false,
		"credit_per_billing_period_list": periods,
	}
	if a.CreditLimit != nil {
		out["credit_limit"] = *a.CreditLimit
	}
	return out
}

// cursorPage pages with the agent endpoints' `cursor` parameter.
func cursorPage(r *http.Request, size int, all []interface{}) map[string]interface{} {
	if size <= 0 {
		size = len(all)
	}
	start := 0
	if c := r.URL.Query().Get("cursor"); c != "" {
		fmt.Sscanf(c, "%d", &start)
	}
	end := start + size
	if end > len(all) {
		end = len(all)
	}
	out := map[string]interface{}{"results": append([]interface{}{}, all[start:end]...), "has_more": end < len(all)}
	if end < len(all) {
		out["next_cursor"] = fmt.Sprint(end)
	}
	return out
}

func (f *fakeAdminAPI) serveAgents(w http.ResponseWriter, r *http.Request, parts []string, body map[string]interface{}) bool {
	if len(parts) < 4 || parts[1] != "spaces" {
		return false
	}
	ws := f.agentWorkspace()
	if len(parts) == 4 && parts[3] == "credit_limit" && r.Method == http.MethodPatch {
		ws.LimitCalls = append(ws.LimitCalls, body["default_agent_credit_limit"])
		f.writeJSON(w, map[string]interface{}{"space_id": parts[2], "default_agent_credit_limit": body["default_agent_credit_limit"]})
		return true
	}
	if parts[3] != "agents" {
		return false
	}
	agents := f.agents()
	ids := make([]string, 0, len(agents))
	for id := range agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	switch {
	case len(parts) == 4 && r.Method == http.MethodGet:
		statuses := r.URL.Query()["status"]
		var all []interface{}
		for _, id := range ids {
			a := agents[id]
			if len(statuses) > 0 && !containsStr(statuses, a.Status) {
				continue
			}
			if a.Deleted && !containsStr(statuses, "deleted") {
				continue
			}
			if n := r.URL.Query().Get("name"); n != "" && !strings.Contains(strings.ToLower(a.Name), strings.ToLower(n)) {
				continue
			}
			all = append(all, map[string]interface{}{
				"id": a.ID, "name": a.Name, "status": a.Status, "type": a.Type, "alive": !a.Deleted,
				"space_id": parts[2], "created_time": 1700000000000.0, "permissions": a.permList(),
			})
		}
		f.writeJSON(w, cursorPage(r, f.pageSize, all))
	case len(parts) == 5 && parts[4] == "credit_usage" && r.Method == http.MethodGet:
		var all []interface{}
		for _, id := range ids {
			all = append(all, agents[id].usage([]interface{}{map[string]interface{}{"credits_used": 40.0, "metrics_period_start_ms": 3.0, "metrics_period_end_ms": 4.0}}))
		}
		f.writeJSON(w, cursorPage(r, f.pageSize, all))
	case len(parts) == 5 && parts[4] == "creation_policy" && r.Method == http.MethodPatch:
		ws.PolicyCalls = append(ws.PolicyCalls, body)
		ws.Policy, _ = body["policy"].(string)
		f.writeJSON(w, map[string]interface{}{
			"agent_counts": map[string]float64{"active": 2, "deleted": 0, "disabled": 0}, "current_policy": ws.Policy,
			"current_policy_description": "Policy is " + ws.Policy, "space_id": parts[2], "space_name": "Acme",
		})
	default:
		a, ok := agents[parts[4]]
		if !ok {
			f.writeErr(w, 404, "not_found", "Not found.")
			return true
		}
		switch {
		case len(parts) == 5 && r.Method == http.MethodDelete:
			a.Deleted, a.Status = true, "deleted"
			f.writeJSON(w, map[string]interface{}{})
		case len(parts) == 6 && parts[5] == "credit_usage":
			// Two pages of periods, paged with start_cursor.
			if r.URL.Query().Get("start_cursor") == "" {
				u := a.usage([]interface{}{map[string]interface{}{"credits_used": 40.0, "metrics_period_start_ms": 3.0, "metrics_period_end_ms": 4.0}})
				u["has_more"], u["next_cursor"] = true, "p2"
				f.writeJSON(w, u)
			} else {
				f.writeJSON(w, a.usage([]interface{}{map[string]interface{}{"credits_used": 300.0, "metrics_period_start_ms": 1.0, "metrics_period_end_ms": 2.0}}))
			}
		case len(parts) == 6 && parts[5] == "credit_limit" && r.Method == http.MethodPut:
			a.CreditLimitCalls = append(a.CreditLimitCalls, body["credit_limit"])
			if v, ok := body["credit_limit"].(float64); ok {
				a.CreditLimit = &v
			} else {
				a.CreditLimit = nil
			}
			f.writeJSON(w, map[string]interface{}{"credit_limit_by_workspace_admin": body["credit_limit"], "enforced_credit_limit": body["credit_limit"],
				"current_credit_usage": 40.0, "did_clear_paused_state": false, "is_active": true})
		case len(parts) == 6 && parts[5] == "status" && r.Method == http.MethodPatch:
			if body["admin_status"] == "disabled" {
				a.Status = "disabled_from_workspace_settings"
			} else {
				a.Status = "active"
			}
			f.writeJSON(w, map[string]interface{}{"admin_status": body["admin_status"], "run_status": "active"})
		case len(parts) == 6 && parts[5] == "permissions" && r.Method == http.MethodGet:
			page := cursorPage(r, f.pageSize, a.permList())
			page["permissions"] = page["results"]
			delete(page, "results")
			delete(page, "has_more")
			f.writeJSON(w, page)
		case len(parts) == 6 && parts[5] == "permissions" && r.Method == http.MethodPatch:
			if set, ok := body["set"].([]interface{}); ok {
				for _, s := range set {
					m := s.(map[string]interface{})
					pr := m["principal"].(map[string]interface{})
					a.Perms[permKey(pr)] = map[string]interface{}{"principal": pr, "role": m["role"], "resolved_role": m["role"]}
				}
			}
			if rem, ok := body["remove"].([]interface{}); ok {
				for _, x := range rem {
					delete(a.Perms, permKey(x.(map[string]interface{})))
				}
			}
			f.writeJSON(w, map[string]interface{}{"permissions": a.permList()})
		default:
			f.writeErr(w, 404, "not_found", "no agent route")
		}
	}
	return true
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func perm(typ, id, role string) map[string]interface{} {
	pr := map[string]interface{}{"type": typ}
	switch typ {
	case "user":
		pr["user_id"] = id
	case "group":
		pr["group_id"] = id
	}
	return map[string]interface{}{"principal": pr, "role": role, "resolved_role": role}
}

func newFakeAdminAPIWithAgents(t *testing.T) *fakeAdminAPI {
	f, _ := newFakeAdminAPI(t)
	f.routes = append(f.routes, f.serveAgents)
	u9 := perm("user", "u9", "comment_only")
	f.agents()["agent-1"] = &fakeAgent{
		ID: "agent-1", Name: "Triage bot", Status: "active", Type: "custom",
		Perms: map[string]map[string]interface{}{permKey(u9["principal"].(map[string]interface{})): u9},
	}
	return f
}

func TestUnitAdminAgentSettings_Lifecycle(t *testing.T) {
	f := newFakeAdminAPIWithAgents(t)
	f.pageSize = 1
	prov := adminOnlyProvider(t, f.token)
	agent := f.agents()["agent-1"]

	step1 := prov + `
resource "notion_admin_agent_settings" "triage" {
  workspace_id = "ws-1"
  agent_id     = "agent-1"
  admin_status = "disabled"
  credit_limit = 500

  permissions = [
    { principal_type = "workspace", role = "view_and_interact" },
    { principal_type = "group", principal_id = "g1", role = "edit" },
  ]
}
`
	step2 := prov + `
resource "notion_admin_agent_settings" "triage" {
  workspace_id = "ws-1"
  agent_id     = "agent-1"
  admin_status = "active"

  permissions = [
    { principal_type = "group", principal_id = "g1", role = "full_access" },
  ]
}
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			if agent.Deleted || agent.Status != "active" || len(agent.Perms) != 1 {
				return fmt.Errorf("destroy must leave the agent untouched, got %+v", agent)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: step1,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_admin_agent_settings.triage", "id", "ws-1/agent-1"),
					resource.TestCheckResourceAttr("notion_admin_agent_settings.triage", "name", "Triage bot"),
					resource.TestCheckResourceAttr("notion_admin_agent_settings.triage", "status", "disabled_from_workspace_settings"),
					resource.TestCheckResourceAttr("notion_admin_agent_settings.triage", "enforced_credit_limit", "500"),
					resource.TestCheckResourceAttr("notion_admin_agent_settings.triage", "permissions.#", "2"),
					func(*terraform.State) error {
						// The comment-only grant made outside Terraform is gone.
						if len(agent.Perms) != 2 {
							return fmt.Errorf("permissions should be authoritative, got %v", agent.Perms)
						}
						return nil
					},
				),
			},
			{
				// A grant added in Notion shows up as drift and is removed.
				PreConfig: func() {
					f.mu.Lock()
					p := perm("user", "u7", "edit")
					agent.Perms[permKey(p["principal"].(map[string]interface{}))] = p
					f.mu.Unlock()
				},
				Config: step1,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("notion_admin_agent_settings.triage", plancheck.ResourceActionUpdate)},
				},
				Check: func(*terraform.State) error {
					if len(agent.Perms) != 2 {
						return fmt.Errorf("drifted grant not removed: %v", agent.Perms)
					}
					return nil
				},
			},
			{
				// Re-enable, clear the credit limit, narrow sharing, all in place.
				Config: step2,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_admin_agent_settings.triage", "status", "active"),
					resource.TestCheckNoResourceAttr("notion_admin_agent_settings.triage", "credit_limit"),
					func(*terraform.State) error {
						last := agent.CreditLimitCalls[len(agent.CreditLimitCalls)-1]
						if last != nil {
							return fmt.Errorf("removing credit_limit should send null, sent %v", last)
						}
						return nil
					},
				),
			},
			{
				ResourceName:            "notion_admin_agent_settings.triage",
				ImportState:             true,
				ImportStateId:           "ws-1/agent-1",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"admin_status", "permissions"},
			},
		},
	})
}

func TestUnitAdminWorkspaceAgentSettings(t *testing.T) {
	f := newFakeAdminAPIWithAgents(t)
	prov := adminOnlyProvider(t, f.token)
	ws := f.agentWorkspace()
	cfg := func(body string) string {
		return prov + "resource \"notion_admin_workspace_agent_settings\" \"acme\" {\n  workspace_id = \"ws-1\"\n" + body + "}\n"
	}
	var callsBeforeDestroy int
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			if len(ws.PolicyCalls)+len(ws.LimitCalls) != callsBeforeDestroy {
				return fmt.Errorf("destroy must not change workspace settings")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: cfg("  creation_policy = \"workspace_owners_only\"\n  default_agent_credit_limit = 100\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_admin_workspace_agent_settings.acme", "current_policy_description", "Policy is workspace_owners_only"),
					func(*terraform.State) error {
						if ws.Policy != "workspace_owners_only" || len(ws.LimitCalls) != 1 || ws.LimitCalls[0] != 100.0 {
							return fmt.Errorf("server state: %+v", ws)
						}
						return nil
					},
				),
			},
			{
				Config: cfg("  creation_policy = \"disabled\"\n  disable_existing_agents = true\n"),
				Check: func(*terraform.State) error {
					last := ws.PolicyCalls[len(ws.PolicyCalls)-1]
					if last["policy"] != "disabled" || last["disable_existing_agents"] != true {
						return fmt.Errorf("policy call: %v", last)
					}
					if ws.LimitCalls[len(ws.LimitCalls)-1] != nil {
						return fmt.Errorf("removing the limit should send null, got %v", ws.LimitCalls)
					}
					callsBeforeDestroy = len(ws.PolicyCalls) + len(ws.LimitCalls)
					return nil
				},
			},
		},
	})
}

func TestUnitAdminAgentDataSourcesAndDeleteAction(t *testing.T) {
	f := newFakeAdminAPIWithAgents(t)
	f.pageSize = 1
	f.agents()["agent-2"] = &fakeAgent{ID: "agent-2", Name: "Digest", Status: "credit_limit", Type: "custom", Perms: map[string]map[string]interface{}{}}
	prov := adminOnlyProvider(t, f.token)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: prov + `
data "notion_admin_agents" "all" {
  workspace_id = "ws-1"
}

data "notion_admin_agents" "active" {
  workspace_id = "ws-1"
  statuses     = ["active"]
}

data "notion_admin_agent_credit_usage" "triage" {
  workspace_id = "ws-1"
  agent_id     = "agent-1"
}

data "notion_admin_agents_credit_usage" "all" {
  workspace_id = "ws-1"
}

action "notion_admin_agent_delete" "digest" {
  config {
    workspace_id = "ws-1"
    agent_id     = "agent-2"
  }
}

resource "terraform_data" "cleanup" {
  input = "digest"
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.notion_admin_agent_delete.digest]
    }
  }
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.notion_admin_agents.all", "agents.#", "2"),
				resource.TestCheckResourceAttr("data.notion_admin_agents.all", "agents.0.permissions.0.principal_type", "user"),
				resource.TestCheckResourceAttr("data.notion_admin_agents.all", "agents.0.permissions.0.role", "comment_only"),
				resource.TestCheckResourceAttr("data.notion_admin_agents.active", "agents.#", "1"),
				resource.TestCheckResourceAttr("data.notion_admin_agent_credit_usage.triage", "periods.#", "2"),
				resource.TestCheckResourceAttr("data.notion_admin_agent_credit_usage.triage", "periods.1.credits_used", "300"),
				resource.TestCheckResourceAttr("data.notion_admin_agent_credit_usage.triage", "total_credits_used", "340"),
				resource.TestCheckResourceAttr("data.notion_admin_agents_credit_usage.all", "agents.#", "2"),
				func(*terraform.State) error {
					if !f.agents()["agent-2"].Deleted {
						return fmt.Errorf("delete action did not run")
					}
					return nil
				},
			),
		}},
	})
}

func TestUnitAdminAgent_Validation(t *testing.T) {
	prov := adminOnlyProvider(t, "adm_test")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: prov + `
resource "notion_admin_agent_settings" "x" {
  workspace_id = "w"
  agent_id     = "a"
  permissions = [
    { principal_type = "workspace", principal_id = "w", role = "comment_only" },
  ]
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)A workspace grant has no principal_id|"comment_only" is not a role`),
			},
			{
				Config: prov + `
resource "notion_admin_workspace_agent_settings" "x" {
  workspace_id            = "w"
  creation_policy         = "workspace_owners_only"
  disable_existing_agents = true
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`only takes effect with creation_policy = "disabled"`),
			},
		},
	})
}
