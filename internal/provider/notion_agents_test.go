package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// agentTestServer points agentsBaseURL at an httptest server for the test.
func agentTestServer(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	prev := agentsBaseURL
	agentsBaseURL = srv.URL + "/v1/agents"
	t.Cleanup(func() { agentsBaseURL = prev })
}

const agentFixture = `{
  "object": "agent",
  "id": "1f0c7c06-781b-4987-9986-5c8dd3028013",
  "agent_type": "custom_agent",
  "name": "Triage",
  "description": null,
  "instructions_page_id": "2a0c7c06-781b-4987-9986-5c8dd3028014",
  "status": "disabled",
  "pause_reason": "disabled_from_api",
  "credit_limit": 500,
  "last_run_at": "hidden",
  "created_time": "2026-08-21T10:00:00.000Z",
  "last_edited_time": "2026-09-01T10:00:00.000Z"
}`

func TestParseHiddenInt(t *testing.T) {
	cases := []struct {
		in         string
		wantVal    *int64
		wantHidden bool
	}{
		{`500`, ptrInt64(500), false},
		{`null`, nil, false},
		{``, nil, false},
		{`"hidden"`, nil, true},
	}
	for _, c := range cases {
		v, h := parseHiddenInt(json.RawMessage(c.in))
		if h != c.wantHidden || (v == nil) != (c.wantVal == nil) || (v != nil && *v != *c.wantVal) {
			t.Errorf("parseHiddenInt(%q) = %v, %v", c.in, v, h)
		}
	}
}

func TestParseHiddenString(t *testing.T) {
	if v, h := parseHiddenString(json.RawMessage(`"2026-09-01T00:00:00Z"`)); h || v == nil || *v != "2026-09-01T00:00:00Z" {
		t.Errorf("timestamp: %v %v", v, h)
	}
	if v, h := parseHiddenString(json.RawMessage(`"hidden"`)); !h || v != nil {
		t.Errorf("hidden: %v %v", v, h)
	}
	if v, h := parseHiddenString(json.RawMessage(`null`)); h || v != nil {
		t.Errorf("null: %v %v", v, h)
	}
}

func TestGetAgent(t *testing.T) {
	agentTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/agents/1f0c7c06-781b-4987-9986-5c8dd3028013" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Notion-Version"); got != agentAPIVersion {
			t.Errorf("Notion-Version = %q", got)
		}
		if r.Header.Get("Notion-Beta") != "" {
			t.Errorf("unexpected Notion-Beta header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(agentFixture))
	})

	a, raw, err := getAgent(context.Background(), "tok", "1f0c7c06-781b-4987-9986-5c8dd3028013")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "Triage" || a.Status != "disabled" || a.PauseReason == nil || *a.PauseReason != "disabled_from_api" {
		t.Errorf("unexpected agent: %+v", a)
	}
	if v, _ := parseHiddenInt(a.CreditLimit); v == nil || *v != 500 {
		t.Errorf("credit limit: %v", v)
	}
	if !strings.Contains(string(raw), `"Triage"`) {
		t.Errorf("raw JSON not returned")
	}
}

func TestGetAgent_NotFound(t *testing.T) {
	agentTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"object":"error","status":404,"code":"object_not_found","message":"nope"}`))
	})
	_, _, err := getAgent(context.Background(), "tok", "missing")
	var nf *agentNotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want agentNotFoundError, got %v", err)
	}
}

func TestUpdateAgent(t *testing.T) {
	var got []string
	agentTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/agents/a1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		got = append(got, string(b))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"agent","id":"a1","agent_type":"custom_agent","name":"New","description":null,"status":"disabled","pause_reason":"disabled_from_api","model":{"mode":"pinned","id":null}}`))
	})
	id := "claude-sonnet-5"
	a, err := updateAgent(context.Background(), "tok", "a1", map[string]interface{}{
		"name":   "New",
		"status": "disabled",
		"model":  agentModel{Mode: "pinned", ID: &id},
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "New" || a.Model == nil || a.Model.Mode != "pinned" || a.Model.ID != nil {
		t.Errorf("response not parsed: %+v", a)
	}
	if _, err := updateAgent(context.Background(), "tok", "a1", map[string]interface{}{"model": agentModel{Mode: "auto"}}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`{"model":{"mode":"pinned","id":"claude-sonnet-5"},"name":"New","status":"disabled"}`,
		`{"model":{"mode":"auto"}}`,
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("bodies:\n got %v\nwant %v", got, want)
	}
}

func TestAgentUpdateFields(t *testing.T) {
	sonnet := "claude-sonnet-5"
	desc := "Sorts tickets"
	current := &agentObject{
		ID: "a1", Name: "Triage", Description: &desc, Status: "disabled",
		PauseReason: agentStrPtr("disabled_from_api"),
		Model:       &agentModel{Mode: "pinned", ID: &sonnet},
	}
	state := AgentSettingsModel{
		Enabled:     types.BoolValue(false),
		Name:        types.StringValue("Triage"),
		Description: types.StringValue(desc),
		ModelMode:   types.StringValue("pinned"),
		ModelID:     types.StringValue(sonnet),
	}

	// Nothing changed and the agent is already disabled, so nothing is sent.
	plan := state
	if fields, _ := agentUpdateFields(&plan, &state, current); len(fields) != 0 {
		t.Errorf("unchanged plan sent %v", fields)
	}

	// Only the description changed. Status stays out because the agent is
	// already disabled, and the name and model stay out because they match.
	plan = state
	plan.Description = types.StringValue("")
	fields, _ := agentUpdateFields(&plan, &state, current)
	if len(fields) != 1 || fields["description"] != "" {
		t.Errorf("description change sent %v", fields)
	}

	// Unmanaged fields are null in config and keep the state value in the
	// plan, so they never go out even when Notion has something else.
	plan = state
	drifted := &agentObject{Name: "Other", Status: "disabled", PauseReason: agentStrPtr("disabled_from_api"), Model: &agentModel{Mode: "auto"}}
	if fields, _ := agentUpdateFields(&plan, &state, drifted); len(fields) != 0 {
		t.Errorf("unmanaged fields sent %v", fields)
	}

	// On create a configured value that already matches the agent is skipped.
	create := AgentSettingsModel{
		Enabled:   types.BoolValue(true),
		Name:      types.StringValue("Triage"),
		ModelMode: types.StringValue("auto"),
		ModelID:   types.StringNull(),
	}
	fields, _ = agentUpdateFields(&create, nil, current)
	if len(fields) != 2 || fields["status"] != "active" || fields["model"] != (agentModel{Mode: "auto"}) {
		t.Errorf("create sent %v", fields)
	}

	// A pinned early-access model reads back without an ID. With the ID
	// unchanged since the last apply, the model isn't resent.
	early := &agentObject{ID: "a1", Name: "Triage", Status: "active", Model: &agentModel{Mode: "pinned"}}
	pinnedState := AgentSettingsModel{ModelMode: types.StringValue("pinned"), ModelID: types.StringValue("notion-early-1")}
	plan = pinnedState
	if fields, _ := agentUpdateFields(&plan, &pinnedState, early); len(fields) != 0 {
		t.Errorf("early-access model resent %v", fields)
	}
	// A different ID can't be compared against the null one, so it goes out.
	plan.ModelID = types.StringValue("claude-sonnet-5")
	if fields, _ := agentUpdateFields(&plan, &pinnedState, early); fields["model"] == nil {
		t.Errorf("changed pinned model not sent: %v", fields)
	}
}

// fakeAgentAPI is a stateful stand-in for GET and PATCH /v1/agents/{id} and
// the credit limit endpoint. Model IDs listed in earlyAccess are reported
// with a null ID, the way Notion reports early-access models. A PATCH that
// sends "disabled" to a disabled agent fails with 403, as Notion documents.
type fakeAgentAPI struct {
	mu          sync.Mutex
	agent       map[string]interface{}
	earlyAccess map[string]bool
	patches     []string
}

func newFakeAgentAPI(t *testing.T) *fakeAgentAPI {
	f := &fakeAgentAPI{
		agent: map[string]interface{}{
			"object": "agent", "id": "a1", "agent_type": "custom_agent",
			"name": "Triage", "description": nil, "status": "active", "pause_reason": nil,
			"model": map[string]interface{}{"mode": "auto"}, "credit_limit": nil,
		},
		earlyAccess: map[string]bool{"notion-early-1": true},
	}
	agentTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agents/a1":
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/agents/a1":
			b, _ := io.ReadAll(r.Body)
			f.patches = append(f.patches, string(b))
			var in map[string]interface{}
			_ = json.Unmarshal(b, &in)
			if in["status"] == "disabled" && f.agent["status"] == "disabled" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"object":"error","status":403,"code":"restricted_resource","message":"already disabled"}`))
				return
			}
			for k, v := range in {
				switch k {
				case "status":
					f.agent["status"] = v
					f.agent["pause_reason"] = nil
					if v == "disabled" {
						f.agent["pause_reason"] = "disabled_from_api"
					}
				case "description":
					if v == "" {
						v = nil
					}
					f.agent["description"] = v
				case "model":
					m := v.(map[string]interface{})
					if id, ok := m["id"].(string); ok && f.earlyAccess[id] {
						m["id"] = nil
					}
					f.agent["model"] = m
				default:
					f.agent[k] = v
				}
			}
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/agents/a1/credit_limit":
			b, _ := io.ReadAll(r.Body)
			var in map[string]interface{}
			_ = json.Unmarshal(b, &in)
			f.agent["credit_limit"] = in["credit_limit"]
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		out, _ := json.Marshal(f.agent)
		_, _ = w.Write(out)
	})
	return f
}

// takePatches returns the PATCH /v1/agents/a1 bodies since the last call.
func (f *fakeAgentAPI) takePatches() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.patches
	f.patches = nil
	return p
}

func (f *fakeAgentAPI) expectPatches(want ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		got := f.takePatches()
		if len(got) != len(want) {
			return fmt.Errorf("want PATCH bodies %v, got %v", want, got)
		}
		for i := range want {
			if got[i] != want[i] {
				return fmt.Errorf("PATCH %d: want %s, got %s", i, want[i], got[i])
			}
		}
		return nil
	}
}

// TestUnitAgentSettings_Lifecycle runs the resource through real Terraform
// against fakeAgentAPI. Every apply sends at most one PATCH with only the
// fields that changed, and status isn't resent to an agent that is already
// disabled.
func TestUnitAgentSettings_Lifecycle(t *testing.T) {
	f := newFakeAgentAPI(t)
	const addr = "notion_agent_settings.test"
	cfg := func(body string) string {
		return agentUnitProvider + "resource \"notion_agent_settings\" \"test\" {\n  agent_id = \"a1\"\n  enabled = false\n" + body + "}\n"
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Everything in one PATCH on create.
				Config: cfg(`
  name        = "Triage bot"
  description = "Sorts tickets"
  model_mode  = "pinned"
  model_id    = "claude-sonnet-5"
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					f.expectPatches(`{"description":"Sorts tickets","model":{"mode":"pinned","id":"claude-sonnet-5"},"name":"Triage bot","status":"disabled"}`),
					resource.TestCheckResourceAttr(addr, "name", "Triage bot"),
					resource.TestCheckResourceAttr(addr, "description", "Sorts tickets"),
					resource.TestCheckResourceAttr(addr, "model_mode", "pinned"),
					resource.TestCheckResourceAttr(addr, "model_id", "claude-sonnet-5"),
					resource.TestCheckResourceAttr(addr, "status", "disabled"),
				),
			},
			{
				// Clearing the description sends only the description. The
				// agent is already disabled, so status stays out (the fake
				// would answer 403 if it were resent).
				Config: cfg(`
  name        = "Triage bot"
  description = ""
  model_mode  = "pinned"
  model_id    = "claude-sonnet-5"
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					f.expectPatches(`{"description":""}`),
					resource.TestCheckResourceAttr(addr, "description", ""),
				),
			},
			{
				// Switching to auto sends only the model, and model_id
				// becomes null without being configured.
				Config: cfg(`
  name        = "Triage bot"
  description = ""
  model_mode  = "auto"
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectKnownValue(addr, tfjsonpath.New("model_id"), knownvalue.Null()),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					f.expectPatches(`{"model":{"mode":"auto"}}`),
					resource.TestCheckResourceAttr(addr, "model_mode", "auto"),
					resource.TestCheckNoResourceAttr(addr, "model_id"),
				),
			},
			{
				ResourceName:            addr,
				ImportState:             true,
				ImportStateId:           "a1",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"enabled"},
			},
			{
				// An early-access model reads back with a null ID. The
				// configured ID is kept, so the follow-up plan is empty.
				PreConfig: func() { f.takePatches() },
				Config: cfg(`
  name        = "Triage bot"
  description = ""
  model_mode  = "pinned"
  model_id    = "notion-early-1"
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					f.expectPatches(`{"model":{"mode":"pinned","id":"notion-early-1"}}`),
					resource.TestCheckResourceAttr(addr, "model_id", "notion-early-1"),
				),
			},
			{
				// Unmanaged fields read back from Notion and nothing is sent.
				PreConfig: func() {
					f.mu.Lock()
					f.agent["name"] = "Renamed in Notion"
					f.agent["description"] = "Edited in Notion"
					f.mu.Unlock()
				},
				Config: cfg(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					f.expectPatches(),
					resource.TestCheckResourceAttr(addr, "name", "Renamed in Notion"),
					resource.TestCheckResourceAttr(addr, "description", "Edited in Notion"),
					resource.TestCheckResourceAttr(addr, "model_id", "notion-early-1"),
				),
			},
			{
				// A managed name that drifted is put back, alone.
				Config: cfg(`
  name = "Triage bot"
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate),
						// The unmanaged early-access ID stays known in the
						// plan instead of turning into "known after apply".
						plancheck.ExpectKnownValue(addr, tfjsonpath.New("model_id"), knownvalue.StringExact("notion-early-1")),
						plancheck.ExpectKnownValue(addr, tfjsonpath.New("model_mode"), knownvalue.StringExact("pinned")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					f.expectPatches(`{"name":"Triage bot"}`),
					resource.TestCheckResourceAttr(addr, "name", "Triage bot"),
					resource.TestCheckResourceAttr(addr, "description", "Edited in Notion"),
					resource.TestCheckResourceAttr(addr, "model_id", "notion-early-1"),
				),
			},
			{
				// Re-enabling sends status alone.
				Config: agentUnitProvider + "resource \"notion_agent_settings\" \"test\" {\n  agent_id = \"a1\"\n  enabled = true\n  name = \"Triage bot\"\n}\n",
				Check: resource.ComposeAggregateTestCheckFunc(
					f.expectPatches(`{"status":"active"}`),
					resource.TestCheckResourceAttr(addr, "status", "active"),
				),
			},
		},
	})
}

func TestUpdateAgentCreditLimit(t *testing.T) {
	var bodies []string
	agentTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/agents/a1/credit_limit" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		var in map[string]json.RawMessage
		_ = json.Unmarshal(b, &in)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agent_id":"a1","credit_limit":` + string(in["credit_limit"]) + `,"last_edited_time":"x"}`))
	})

	v, err := updateAgentCreditLimit(context.Background(), "tok", "a1", ptrInt64(250))
	if err != nil || v == nil || *v != 250 {
		t.Fatalf("set: %v %v", v, err)
	}
	v, err = updateAgentCreditLimit(context.Background(), "tok", "a1", nil)
	if err != nil || v != nil {
		t.Fatalf("clear: %v %v", v, err)
	}
	if bodies[0] != `{"credit_limit":250}` || bodies[1] != `{"credit_limit":null}` {
		t.Errorf("bodies: %v", bodies)
	}
}

func TestQueryAgents_PaginatesAndLimits(t *testing.T) {
	var requests []map[string]interface{}
	agentTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/agents/query" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		var in map[string]interface{}
		_ = json.Unmarshal(b, &in)
		requests = append(requests, in)
		w.Header().Set("Content-Type", "application/json")
		if in["start_cursor"] == nil {
			_, _ = w.Write([]byte(`{"object":"list","results":[{"id":"a1","name":"One","agent_type":"custom_agent","status":"active"}],"has_more":true,"next_cursor":"opaque-cursor=="}`))
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","results":[{"id":"a2","name":"Two","agent_type":"custom_agent","status":"disabled"},{"id":"a3","name":"Three","agent_type":"autofill_custom_agent","status":"active"}],"has_more":false,"next_cursor":null}`))
	})

	got, err := queryAgents(context.Background(), "tok", map[string]interface{}{"query": "o"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[2].ID != "a3" {
		t.Errorf("results: %+v", got)
	}
	if requests[1]["start_cursor"] != "opaque-cursor==" || requests[1]["query"] != "o" {
		t.Errorf("second request: %v", requests[1])
	}

	requests = nil
	got, err = queryAgents(context.Background(), "tok", nil, 1)
	if err != nil || len(got) != 1 || len(requests) != 1 {
		t.Errorf("limit 1: %d results after %d requests, err %v", len(got), len(requests), err)
	}
}

func TestGetAgentInsights_Window(t *testing.T) {
	agentTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agents/a1/insights" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.URL.Query().Get("start_time") != "100" || r.URL.Query().Get("end_time") != "200" {
			t.Errorf("query %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"agent_insights","id":"a1","name":"One","agent_type":"custom_agent","status":"active","pause_reason":null,"created_by":null,"total_credits_used":12.5,"credit_limit":"hidden","runs_completed":3}`))
	})
	in, err := getAgentInsights(context.Background(), "tok", "a1", ptrInt64(100), ptrInt64(200))
	if err != nil {
		t.Fatal(err)
	}
	if in.TotalCreditsUsed != 12.5 || in.RunsCompleted != 3 {
		t.Errorf("insights: %+v", in)
	}
	if _, hidden := parseHiddenInt(in.CreditLimit); !hidden {
		t.Errorf("credit_limit should be hidden")
	}
}

func ptrInt64(v int64) *int64 { return &v }
