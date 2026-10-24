package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestUpdateAgentStatus(t *testing.T) {
	var got map[string]string
	agentTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/agents/a1/status" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agent_id":"a1","status":"` + got["status"] + `","pause_reason":null,"last_edited_time":"x"}`))
	})
	if err := updateAgentStatus(context.Background(), "tok", "a1", false); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "disabled" {
		t.Errorf("disable sent %v", got)
	}
	if err := updateAgentStatus(context.Background(), "tok", "a1", true); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "active" {
		t.Errorf("enable sent %v", got)
	}
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
