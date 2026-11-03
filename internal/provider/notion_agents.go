package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// Notion Agent APIs (public beta 2026-08-20, generally available 2026-11-02).
// The jomei/notionapi SDK has no agent support, so these endpoints go over
// doNotionRequestWithVersion.
//
// Every endpoint here documents Notion-Version 2026-03-11 and no Notion-Beta
// header. Both personal access tokens and connection tokens work. A
// connection token needs the "View sessions and interact with agents"
// capability to read agents and "Edit selected agents" to change one. Access
// is also checked per agent, edit access to change name, description, model,
// or status and full access to read or change the credit limit. A connection
// can't change a credit limit at all, so that needs a personal access token.
// Fields the caller can't read come back as the string "hidden" instead of
// being omitted.

const agentAPIVersion = "2026-03-11"

// agentsBaseURL is a variable so unit tests can point it at httptest.
var agentsBaseURL = notionAPIBaseURL + "/agents"

// agentObject is the subset of the agent object the provider exposes.
// credit_limit and last_run_at can be a value, null, or "hidden", so they
// stay raw and go through the helpers below.
type agentObject struct {
	ID                 string          `json:"id"`
	AgentType          string          `json:"agent_type"`
	Name               string          `json:"name"`
	Description        *string         `json:"description"`
	InstructionsPageID *string         `json:"instructions_page_id"`
	Status             string          `json:"status"`
	PauseReason        *string         `json:"pause_reason"`
	CreditLimit        json.RawMessage `json:"credit_limit"`
	LastRunAt          json.RawMessage `json:"last_run_at"`
	Model              *agentModel     `json:"model"`
	CreatedTime        string          `json:"created_time"`
	LastEditedTime     string          `json:"last_edited_time"`
}

// agentModel is the agent's model selection. Mode is "auto" or "pinned".
// A pinned model reads back with a null ID when it is an early-access model,
// and the request schema rejects an id next to "auto", so ID is omitted when
// nil.
type agentModel struct {
	Mode string  `json:"mode"`
	ID   *string `json:"id,omitempty"`
}

type agentInsights struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	AgentType        string          `json:"agent_type"`
	Status           string          `json:"status"`
	PauseReason      *string         `json:"pause_reason"`
	TotalCreditsUsed float64         `json:"total_credits_used"`
	CreditLimit      json.RawMessage `json:"credit_limit"`
	RunsCompleted    float64         `json:"runs_completed"`
}

// agentNotFoundError marks a 404 so resources can drop the agent from state.
type agentNotFoundError struct{ msg string }

func (e *agentNotFoundError) Error() string { return e.msg }

// manualPauseReasons are the pauses that come from someone turning the agent
// off, as opposed to Notion pausing it for a limit or an error. Only these
// count as "disabled" for the resource's enabled attribute.
var manualPauseReasons = map[string]bool{
	"disabled_from_api":            true,
	"disabled_from_agent_settings": true,
}

// parseHiddenInt reads a field that is an integer, null, or "hidden".
func parseHiddenInt(raw json.RawMessage) (value *int64, hidden bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return nil, s == "hidden"
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		v := int64(f)
		return &v, false
	}
	return nil, false
}

// parseHiddenString reads a field that is a string, null, or "hidden".
func parseHiddenString(raw json.RawMessage) (value *string, hidden bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return nil, false
	}
	if s == "hidden" {
		return nil, true
	}
	return &s, false
}

func doAgentRequest(ctx context.Context, method, endpoint, token string, body interface{}) ([]byte, error) {
	var reqBody []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = b
	}
	resp, err := doNotionRequestWithVersion(ctx, method, endpoint, token, agentAPIVersion, reqBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, &agentNotFoundError{msg: fmt.Sprintf("notion API 404 on %s %s: %s", method, endpoint, string(respBody))}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("notion API %d on %s %s: %s", resp.StatusCode, method, endpoint, string(respBody))
	}
	return respBody, nil
}

func agentURL(agentID string, suffix string) string {
	return agentsBaseURL + "/" + url.PathEscape(agentID) + suffix
}

// getAgent calls GET /v1/agents/{id} and returns the parsed object plus the
// raw JSON.
func getAgent(ctx context.Context, token, agentID string) (*agentObject, []byte, error) {
	body, err := doAgentRequest(ctx, http.MethodGet, agentURL(agentID, ""), token, nil)
	if err != nil {
		return nil, nil, err
	}
	var a agentObject
	if err := json.Unmarshal(body, &a); err != nil {
		return nil, nil, fmt.Errorf("failed to parse agent response: %w", err)
	}
	return &a, body, nil
}

// queryAgents pages through POST /v1/agents/query. limit <= 0 means all.
func queryAgents(ctx context.Context, token string, req map[string]interface{}, limit int) ([]agentObject, error) {
	var out []agentObject
	cursor := ""
	for {
		body := make(map[string]interface{}, len(req)+2)
		for k, v := range req {
			body[k] = v
		}
		body["page_size"] = 100
		if cursor != "" {
			body["start_cursor"] = cursor
		}
		respBody, err := doAgentRequest(ctx, http.MethodPost, agentsBaseURL+"/query", token, body)
		if err != nil {
			return nil, err
		}
		var page struct {
			Results    []agentObject `json:"results"`
			HasMore    bool          `json:"has_more"`
			NextCursor *string       `json:"next_cursor"`
		}
		if err := json.Unmarshal(respBody, &page); err != nil {
			return nil, fmt.Errorf("failed to parse query agents response: %w", err)
		}
		out = append(out, page.Results...)
		if limit > 0 && len(out) >= limit {
			return out[:limit], nil
		}
		// Cursors are opaque (2026-04-22): pass them back verbatim.
		if !page.HasMore || page.NextCursor == nil || *page.NextCursor == "" {
			return out, nil
		}
		cursor = *page.NextCursor
	}
}

// updateAgent sends PATCH /v1/agents/{id} with the given fields and returns
// the updated agent. Since the Agent APIs became generally available
// (2026-11-02) this one endpoint takes name, description, model, and status,
// and Notion saves all of them together or none. Callers send only the fields
// that change. Sending "disabled" for an agent that is already disabled fails
// the whole request, so status needs the statusChangeNeeded guard.
func updateAgent(ctx context.Context, token, agentID string, fields map[string]interface{}) (*agentObject, error) {
	respBody, err := doAgentRequest(ctx, http.MethodPatch, agentURL(agentID, ""), token, fields)
	if err != nil {
		return nil, err
	}
	var a agentObject
	if err := json.Unmarshal(respBody, &a); err != nil {
		return nil, fmt.Errorf("failed to parse update agent response: %w", err)
	}
	return &a, nil
}

// updateAgentCreditLimit calls PATCH /v1/agents/{id}/credit_limit. A nil
// limit clears it. Returns the agent's own limit from the response, which is
// the only place the API reports it (GET returns the effective limit).
func updateAgentCreditLimit(ctx context.Context, token, agentID string, limit *int64) (*int64, error) {
	payload := map[string]interface{}{"credit_limit": nil}
	if limit != nil {
		payload["credit_limit"] = *limit
	}
	respBody, err := doAgentRequest(ctx, http.MethodPatch, agentURL(agentID, "/credit_limit"), token, payload)
	if err != nil {
		return nil, err
	}
	var r struct {
		CreditLimit json.RawMessage `json:"credit_limit"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return nil, fmt.Errorf("failed to parse credit limit response: %w", err)
	}
	v, _ := parseHiddenInt(r.CreditLimit)
	return v, nil
}

// getAgentInsights calls GET /v1/agents/{id}/insights. start and end are
// epoch seconds and must be given together, otherwise the window is the
// current billing period.
func getAgentInsights(ctx context.Context, token, agentID string, start, end *int64) (*agentInsights, error) {
	endpoint := agentURL(agentID, "/insights")
	if start != nil && end != nil {
		q := url.Values{}
		q.Set("start_time", strconv.FormatInt(*start, 10))
		q.Set("end_time", strconv.FormatInt(*end, 10))
		endpoint += "?" + q.Encode()
	}
	respBody, err := doAgentRequest(ctx, http.MethodGet, endpoint, token, nil)
	if err != nil {
		return nil, err
	}
	var in agentInsights
	if err := json.Unmarshal(respBody, &in); err != nil {
		return nil, fmt.Errorf("failed to parse agent insights response: %w", err)
	}
	return &in, nil
}

// compactJSON normalizes a JSON string so a jsonencode() value and the same
// object written by hand compare equal.
func compactJSON(s string) (json.RawMessage, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		return nil, err
	}
	return json.RawMessage(buf.Bytes()), nil
}
