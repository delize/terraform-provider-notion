package provider

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeLegalHold mirrors the documented legal hold object. Legal hold lists
// use the older envelope: the array is named after the resource, there is no
// has_more, and next_cursor is omitted on the last page.
type fakeLegalHold struct {
	ID, Name, Description, Icon, Status string
	StartDate                           float64
	EndDate                             *float64
	Types                               []string
	Users                               map[string]bool
	Exports                             []map[string]interface{}
}

func (h *fakeLegalHold) json() map[string]interface{} {
	users := make([]string, 0, len(h.Users))
	for u := range h.Users {
		users = append(users, u)
	}
	sort.Strings(users)
	out := map[string]interface{}{
		"id": h.ID, "name": h.Name, "created_by": "creator-1", "created_time": 1700000000000.0,
		"start_date": h.StartDate, "status": h.Status, "user_interaction_type": h.Types,
		"users":      map[string]interface{}{"ids": users, "total": len(users)},
		"workspaces": map[string]interface{}{"ids": []string{"ws-1"}, "total": 1},
	}
	if h.Description != "" {
		out["description"] = h.Description
	}
	if h.Icon != "" {
		out["icon"] = h.Icon
	}
	if h.EndDate != nil {
		out["end_date"] = *h.EndDate
	}
	return out
}

// legalPage pages an ID list with the legal hold envelope.
func (f *fakeAdminAPI) legalPage(r *http.Request, key string, all []string) map[string]interface{} {
	size := len(all)
	if f.pageSize > 0 {
		size = f.pageSize
	}
	start := 0
	if c := r.URL.Query().Get("start_cursor"); c != "" {
		fmt.Sscanf(c, "%d", &start)
	}
	end := start + size
	if end > len(all) {
		end = len(all)
	}
	out := map[string]interface{}{key: append([]string{}, all[start:end]...)}
	if end < len(all) {
		out["next_cursor"] = fmt.Sprint(end)
	}
	return out
}

func strList(v interface{}) []string {
	var out []string
	if arr, ok := v.([]interface{}); ok {
		for _, x := range arr {
			out = append(out, fmt.Sprint(x))
		}
	}
	return out
}

func (f *fakeAdminAPI) serveLegalHolds(w http.ResponseWriter, r *http.Request, parts []string, body map[string]interface{}) bool {
	if len(parts) < 2 || parts[1] != "legal_holds" {
		return false
	}
	notFound := func() { f.writeErr(w, 404, "not_found", "Not found.") }

	if len(parts) == 2 {
		switch r.Method {
		case http.MethodGet:
			ids := make([]string, 0, len(f.legalHolds))
			for id := range f.legalHolds {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			var holds []interface{}
			for _, id := range ids {
				h := f.legalHolds[id]
				if u := r.URL.Query().Get("user_id"); u != "" && !h.Users[u] {
					continue
				}
				holds = append(holds, h.json())
			}
			if holds == nil {
				holds = []interface{}{}
			}
			f.writeJSON(w, map[string]interface{}{"legal_holds": holds})
		case http.MethodPost:
			f.nextID++
			h := &fakeLegalHold{ID: fmt.Sprintf("hold-%d", f.nextID), Status: "active", Users: map[string]bool{}}
			h.Name, _ = body["name"].(string)
			h.Description, _ = body["description"].(string)
			h.Icon, _ = body["icon"].(string)
			h.StartDate, _ = body["start_date"].(float64)
			if e, ok := body["end_date"].(float64); ok {
				h.EndDate = &e
			}
			h.Types = strList(body["user_interaction_type"])
			for _, u := range strList(body["user_ids"]) {
				h.Users[u] = true
			}
			// Tests can make the fake store values differently from what
			// was sent, to mimic undocumented server normalization.
			if hook, ok := f.areaState["legalHoldOnCreate"].(func(*fakeLegalHold)); ok {
				hook(h)
			}
			f.legalHolds[h.ID] = h
			f.writeJSON(w, h.json())
		}
		return true
	}

	h, ok := f.legalHolds[parts[2]]
	if !ok {
		notFound()
		return true
	}
	switch {
	case len(parts) == 3 && r.Method == http.MethodGet:
		f.writeJSON(w, h.json())
	case len(parts) == 3 && r.Method == http.MethodPatch:
		if v, ok := body["name"].(string); ok {
			h.Name = v
		}
		if v, ok := body["description"].(string); ok {
			h.Description = v
		}
		if v, ok := body["icon"].(string); ok {
			h.Icon = v
		}
		f.writeJSON(w, h.json())
	case len(parts) == 4 && parts[3] == "users" && r.Method == http.MethodGet:
		users := make([]string, 0, len(h.Users))
		for u := range h.Users {
			users = append(users, u)
		}
		sort.Strings(users)
		f.writeJSON(w, f.legalPage(r, "user_ids", users))
	case len(parts) == 4 && parts[3] == "users" && r.Method == http.MethodPost:
		for _, u := range strList(body["user_ids"]) {
			h.Users[u] = true
		}
		f.writeJSON(w, h.json())
	case len(parts) == 5 && parts[3] == "users" && r.Method == http.MethodDelete:
		if !h.Users[parts[4]] {
			notFound()
			return true
		}
		delete(h.Users, parts[4])
		f.writeJSON(w, h.json())
	case len(parts) == 4 && parts[3] == "workspaces":
		f.writeJSON(w, f.legalPage(r, "workspace_ids", []string{"ws-1"}))
	case len(parts) == 6 && parts[3] == "spaces" && parts[5] == "pages":
		f.writeJSON(w, f.legalPage(r, "page_ids", []string{"p-1", "p-2", "p-3"}))
	case len(parts) == 4 && parts[3] == "release" && r.Method == http.MethodPost:
		h.Status = "released"
		f.writeJSON(w, h.json())
	case len(parts) == 4 && parts[3] == "export" && r.Method == http.MethodPost:
		h.Exports = append(h.Exports, body)
		f.writeJSON(w, map[string]string{"legal_hold_export_id": fmt.Sprintf("export-%d", len(h.Exports))})
	default:
		notFound()
	}
	return true
}

func newFakeAdminAPIWithLegalHolds(t *testing.T) *fakeAdminAPI {
	f, _ := newFakeAdminAPI(t)
	f.routes = append(f.routes, f.serveLegalHolds)
	return f
}

func legalHoldConfig(prov, name, desc string, users []string, release bool) string {
	descLine := ""
	if desc != "" {
		descLine = fmt.Sprintf("  description = %q\n", desc)
	}
	quoted := ""
	for i, u := range users {
		if i > 0 {
			quoted += ", "
		}
		quoted += fmt.Sprintf("%q", u)
	}
	return prov + fmt.Sprintf(`
resource "notion_legal_hold" "case" {
  name                   = %q
%s  start_date             = 1760000000000
  user_interaction_types = ["page.created", "page.edited"]
  user_ids               = [%s]
  release_on_destroy     = %t
}
`, name, descLine, quoted, release)
}

func TestUnitLegalHold_LifecycleKeepsHoldOnDestroy(t *testing.T) {
	f := newFakeAdminAPIWithLegalHolds(t)
	f.pageSize = 1
	prov := adminOnlyProvider(t, f.token)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			h := f.legalHolds["hold-1"]
			if h == nil || h.Status != "active" {
				return fmt.Errorf("hold should still be active after destroy, got %+v", h)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: legalHoldConfig(prov, "Case 42", "", []string{"u1", "u2"}, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_legal_hold.case", "id", "hold-1"),
					resource.TestCheckResourceAttr("notion_legal_hold.case", "status", "active"),
					resource.TestCheckResourceAttr("notion_legal_hold.case", "user_ids.#", "2"),
					resource.TestCheckResourceAttr("notion_legal_hold.case", "workspace_ids.#", "1"),
					resource.TestCheckNoResourceAttr("notion_legal_hold.case", "description"),
					resource.TestCheckResourceAttr("notion_legal_hold.case", "created_by", "creator-1"),
				),
			},
			{
				// Rename, add a description, swap u2 for u3, all in place.
				Config: legalHoldConfig(prov, "Case 42 (litigation)", "Smith v. Acme", []string{"u1", "u3"}, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("notion_legal_hold.case", plancheck.ResourceActionUpdate)},
				},
				Check: func(*terraform.State) error {
					h := f.legalHolds["hold-1"]
					if h.Name != "Case 42 (litigation)" || h.Description != "Smith v. Acme" || h.Users["u2"] || !h.Users["u3"] {
						return fmt.Errorf("server state: %+v", h)
					}
					return nil
				},
			},
			{
				ResourceName:            "notion_legal_hold.case",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"release_on_destroy"},
			},
		},
	})
}

func TestUnitLegalHold_ReleaseOnDestroy(t *testing.T) {
	f := newFakeAdminAPIWithLegalHolds(t)
	prov := adminOnlyProvider(t, f.token)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			if h := f.legalHolds["hold-1"]; h == nil || h.Status != "released" {
				return fmt.Errorf("hold should be released, got %+v", h)
			}
			return nil
		},
		Steps: []resource.TestStep{{Config: legalHoldConfig(prov, "Case 7", "", []string{"u1"}, true)}},
	})
}

func TestUnitLegalHold_DataSourcesAndExportAction(t *testing.T) {
	f := newFakeAdminAPIWithLegalHolds(t)
	f.pageSize = 2
	f.legalHolds["hold-a"] = &fakeLegalHold{ID: "hold-a", Name: "A", Status: "active", StartDate: 1, Types: []string{"page.viewed"}, Users: map[string]bool{"u1": true}}
	f.legalHolds["hold-b"] = &fakeLegalHold{ID: "hold-b", Name: "B", Status: "released", StartDate: 2, Types: []string{"page.edited"}, Users: map[string]bool{"u2": true}}
	prov := adminOnlyProvider(t, f.token)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: prov + `
data "notion_legal_holds" "all" {}

data "notion_legal_holds" "u1" {
  user_id = "u1"
}

data "notion_legal_hold_pages" "a" {
  legal_hold_id = "hold-a"
  workspace_id  = "ws-1"
}

action "notion_legal_hold_export" "a" {
  config {
    legal_hold_id      = "hold-a"
    requesting_user_id = "u9"
    workspace_id       = "ws-1"
  }
}

resource "terraform_data" "export" {
  input = "run-1"
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.notion_legal_hold_export.a]
    }
  }
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.notion_legal_holds.all", "legal_holds.#", "2"),
				resource.TestCheckResourceAttr("data.notion_legal_holds.all", "legal_holds.1.status", "released"),
				resource.TestCheckResourceAttr("data.notion_legal_holds.u1", "legal_holds.#", "1"),
				resource.TestCheckResourceAttr("data.notion_legal_holds.u1", "legal_holds.0.user_count", "1"),
				resource.TestCheckResourceAttr("data.notion_legal_hold_pages.a", "page_ids.#", "3"),
				func(*terraform.State) error {
					ex := f.legalHolds["hold-a"].Exports
					if len(ex) != 1 || ex[0]["space_id"] != "ws-1" || ex[0]["requesting_user_id"] != "u9" {
						return fmt.Errorf("export requests: %v", ex)
					}
					return nil
				},
			),
		}},
	})
}

func TestUnitLegalHold_Validation(t *testing.T) {
	prov := adminOnlyProvider(t, "adm_test")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: prov + `
resource "notion_legal_hold" "x" {
  name                   = "x"
  start_date             = 10
  end_date               = 5
  user_interaction_types = ["page.deleted"]
  user_ids               = ["u1"]
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)"page.deleted" is not a legal hold interaction type.*end_date is before start_date|(?s)end_date is before start_date.*"page.deleted"`),
			},
			{
				Config: prov + `
action "notion_legal_hold_export" "x" {
  config {
    legal_hold_id      = "h"
    requesting_user_id = "u"
  }
}

resource "terraform_data" "t" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.notion_legal_hold_export.x]
    }
  }
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Set exactly one of workspace_id`),
			},
		},
	})
}

// Notion doesn't document the date unit or whether it normalizes activity
// types. If it stores something other than what was sent, apply must not
// fail and the next plan must not replace the hold, since a replacement
// creates a second active hold.
func TestUnitLegalHold_ServerNormalizedValuesKeepApplied(t *testing.T) {
	f := newFakeAdminAPIWithLegalHolds(t)
	f.areaState["legalHoldOnCreate"] = func(h *fakeLegalHold) {
		h.StartDate /= 1000
		h.Types = []string{"page.created", "page.edited", "page.viewed"}
	}
	prov := adminOnlyProvider(t, f.token)
	cfg := legalHoldConfig(prov, "Case 9", "", []string{"u1"}, false)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_legal_hold.case", "start_date", "1760000000000"),
					resource.TestCheckResourceAttr("notion_legal_hold.case", "user_interaction_types.#", "2"),
				),
			},
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
	if len(f.legalHolds) != 1 {
		t.Fatalf("expected exactly one hold, got %d", len(f.legalHolds))
	}
}

func TestUnitLegalHold_DateGuards(t *testing.T) {
	prov := adminOnlyProvider(t, "adm_test")
	hold := func(start string) string {
		return prov + fmt.Sprintf(`
resource "notion_legal_hold" "x" {
  name                   = "x"
  start_date             = %s
  user_interaction_types = ["page.created"]
  user_ids               = ["u1"]
}`, start)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Epoch seconds by mistake.
				Config:      hold("1760000000"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Date looks like seconds`),
			},
			{
				// Before May 2022 is a warning, not an error.
				Config:             hold("1600000000000"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
