package provider

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Custom agent governance endpoints of the Admin API, all workspace-scoped
// under /v1/spaces/{space_id}. Scopes: workflows:read and workflows:write.
//
// Times are epoch milliseconds and credits are whole numbers, per the
// reference. Agent lists and permission lists page with `cursor`. The agent
// list documents array filters without saying how to encode them, so the
// provider repeats the key (status=a&status=b), the default OpenAPI form
// encoding. The MCP and PAT endpoints, which do say, use brackets instead.

// agentPrincipalTypes, agentGrantRoles, and friends are the documented enums.
var (
	agentPrincipalTypes   = []string{"user", "group", "workspace"}
	agentGrantRoles       = []string{"edit", "full_access", "view_and_interact"}
	agentAdminStatuses    = []string{"active", "disabled"}
	agentCreationPolicies = []string{"all_workspace_members", "disabled", "workspace_owners_only"}
)

type adminAgentPrincipal struct {
	Type    string `json:"type"`
	UserID  string `json:"user_id,omitempty"`
	GroupID string `json:"group_id,omitempty"`
}

// id returns the principal's user or group ID, or "" for the workspace.
func (p adminAgentPrincipal) id() string {
	if p.Type == "group" {
		return p.GroupID
	}
	return p.UserID
}

// key identifies a principal for diffing, ignoring ID hyphens and case.
func (p adminAgentPrincipal) key() string {
	return p.Type + ":" + strings.ToLower(normalizeID(p.id()))
}

func newAgentPrincipal(typ, id string) adminAgentPrincipal {
	switch typ {
	case "group":
		return adminAgentPrincipal{Type: typ, GroupID: id}
	case "user":
		return adminAgentPrincipal{Type: typ, UserID: id}
	}
	return adminAgentPrincipal{Type: typ}
}

type adminAgentPermission struct {
	Principal    adminAgentPrincipal `json:"principal"`
	Role         string              `json:"role"`
	ResolvedRole string              `json:"resolved_role"`
}

type adminAgent struct {
	ID                  string                 `json:"id"`
	SpaceID             string                 `json:"space_id"`
	Name                string                 `json:"name"`
	Description         string                 `json:"description"`
	Icon                string                 `json:"icon"`
	Status              string                 `json:"status"`
	Type                string                 `json:"type"`
	Alive               bool                   `json:"alive"`
	CreatedByID         string                 `json:"created_by_id"`
	CreatedTime         *float64               `json:"created_time"`
	LastEditedTime      *float64               `json:"last_edited_time"`
	LastRunTime         *float64               `json:"last_run_time"`
	LastCreditUsageTime *float64               `json:"last_credit_usage_time"`
	Model               string                 `json:"model"`
	ParentID            string                 `json:"parent_id"`
	ParentTable         string                 `json:"parent_table"`
	Permissions         []adminAgentPermission `json:"permissions"`
}

type adminCreditPeriod struct {
	CreditsUsed float64 `json:"credits_used"`
	StartMS     float64 `json:"metrics_period_start_ms"`
	EndMS       float64 `json:"metrics_period_end_ms"`
}

type adminAgentCreditUsage struct {
	ID                  string              `json:"id"`
	Name                string              `json:"name"`
	Status              string              `json:"status"`
	Type                string              `json:"type"`
	CreatedByID         string              `json:"created_by_id"`
	CreditLimit         *float64            `json:"credit_limit"`
	TotalCreditsUsed    float64             `json:"total_credits_used"`
	RunsCompleted       float64             `json:"runs_completed"`
	LastCreditUsageTime *float64            `json:"last_credit_usage_time"`
	Periods             []adminCreditPeriod `json:"credit_per_billing_period_list"`
	HasMore             bool                `json:"has_more"`
	NextCursor          string              `json:"next_cursor"`
}

type agentListFilter struct {
	Name     string
	Statuses []string
}

func (c *adminClient) ListAgents(ctx context.Context, spaceID string, f agentListFilter) ([]adminAgent, error) {
	q := url.Values{}
	if f.Name != "" {
		q.Set("name", f.Name)
	}
	for _, s := range f.Statuses {
		q.Add("status", s)
	}
	return listInto[adminAgent](ctx, c, adminListSpec{
		Path: adminSpacePath(spaceID, "agents"), Query: q, CursorParam: "cursor", PageSize: 100,
	})
}

// GetAgentCreditUsage returns one agent's usage with every billing period,
// following the period cursor (start_cursor on this endpoint).
func (c *adminClient) GetAgentCreditUsage(ctx context.Context, spaceID, agentID string) (*adminAgentCreditUsage, error) {
	var first *adminAgentCreditUsage
	cursor := ""
	for {
		q := url.Values{}
		if cursor != "" {
			q.Set("start_cursor", cursor)
		}
		var page adminAgentCreditUsage
		if err := c.do(ctx, http.MethodGet, adminSpacePath(spaceID, "agents", agentID, "credit_usage"), q, nil, &page); err != nil {
			return nil, err
		}
		if first == nil {
			first = &page
		} else {
			first.Periods = append(first.Periods, page.Periods...)
		}
		if !page.HasMore || page.NextCursor == "" {
			first.HasMore, first.NextCursor = false, ""
			return first, nil
		}
		cursor = page.NextCursor
	}
}

// ListAgentsCreditUsage returns usage for every agent. Each agent carries
// only its first page of billing periods, as the endpoint returns them.
func (c *adminClient) ListAgentsCreditUsage(ctx context.Context, spaceID string, includeDeleted bool) ([]adminAgentCreditUsage, error) {
	q := url.Values{}
	if includeDeleted {
		q.Set("include_deleted", "true")
	}
	return listInto[adminAgentCreditUsage](ctx, c, adminListSpec{
		Path: adminSpacePath(spaceID, "agents", "credit_usage"), Query: q, CursorParam: "cursor", PageSize: 100,
	})
}

type agentCreditLimitResult struct {
	CreditLimitByWorkspaceAdmin *float64 `json:"credit_limit_by_workspace_admin"`
	EnforcedCreditLimit         *float64 `json:"enforced_credit_limit"`
	CurrentCreditUsage          *float64 `json:"current_credit_usage"`
	DidClearPausedState         bool     `json:"did_clear_paused_state"`
	IsActive                    bool     `json:"is_active"`
}

// UpdateAgentCreditLimit sets the admin credit limit, or clears it when
// limit is nil (the API takes an explicit null).
func (c *adminClient) UpdateAgentCreditLimit(ctx context.Context, spaceID, agentID string, limit *int64) (*agentCreditLimitResult, error) {
	var out agentCreditLimitResult
	body := map[string]interface{}{"credit_limit": limit}
	if err := c.do(ctx, http.MethodPut, adminSpacePath(spaceID, "agents", agentID, "credit_limit"), nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *adminClient) GetAgentPermissions(ctx context.Context, spaceID, agentID string) ([]adminAgentPermission, error) {
	return listInto[adminAgentPermission](ctx, c, adminListSpec{
		Path: adminSpacePath(spaceID, "agents", agentID, "permissions"), CursorParam: "cursor", ResultsKey: "permissions",
	})
}

type agentPermissionSet struct {
	Principal adminAgentPrincipal `json:"principal"`
	Role      string              `json:"role"`
}

func (c *adminClient) UpdateAgentPermissions(ctx context.Context, spaceID, agentID string, set []agentPermissionSet, remove []adminAgentPrincipal) error {
	body := map[string]interface{}{}
	if len(set) > 0 {
		body["set"] = set
	}
	if len(remove) > 0 {
		body["remove"] = remove
	}
	return c.do(ctx, http.MethodPatch, adminSpacePath(spaceID, "agents", agentID, "permissions"), nil, body, nil)
}

type agentStatusResult struct {
	AdminStatus  string `json:"admin_status"`
	RunStatus    string `json:"run_status"`
	PausedReason string `json:"paused_reason"`
}

func (c *adminClient) UpdateAgentStatus(ctx context.Context, spaceID, agentID, adminStatus string) (*agentStatusResult, error) {
	var out agentStatusResult
	err := c.do(ctx, http.MethodPatch, adminSpacePath(spaceID, "agents", agentID, "status"), nil, map[string]string{"admin_status": adminStatus}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteAgent soft-deletes a custom agent.
func (c *adminClient) DeleteAgent(ctx context.Context, spaceID, agentID string) error {
	return c.do(ctx, http.MethodDelete, adminSpacePath(spaceID, "agents", agentID), nil, nil, nil)
}

type agentCreationPolicyResult struct {
	AgentCounts struct {
		Active   float64 `json:"active"`
		Deleted  float64 `json:"deleted"`
		Disabled float64 `json:"disabled"`
	} `json:"agent_counts"`
	CurrentPolicy            *string `json:"current_policy"`
	CurrentPolicyDescription string  `json:"current_policy_description"`
	SpaceName                string  `json:"space_name"`
}

func (c *adminClient) UpdateAgentCreationPolicy(ctx context.Context, spaceID, policy string, disableExisting bool) (*agentCreationPolicyResult, error) {
	body := map[string]interface{}{"policy": policy}
	if disableExisting {
		body["disable_existing_agents"] = true
	}
	var out agentCreationPolicyResult
	if err := c.do(ctx, http.MethodPatch, adminSpacePath(spaceID, "agents", "creation_policy"), nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateWorkspaceCreditLimit sets the workspace default agent credit limit,
// or removes it when limit is nil.
func (c *adminClient) UpdateWorkspaceCreditLimit(ctx context.Context, spaceID string, limit *int64) (*float64, error) {
	var out struct {
		DefaultAgentCreditLimit *float64 `json:"default_agent_credit_limit"`
	}
	body := map[string]interface{}{"default_agent_credit_limit": limit}
	if err := c.do(ctx, http.MethodPatch, adminSpacePath(spaceID, "credit_limit"), nil, body, &out); err != nil {
		return nil, err
	}
	return out.DefaultAgentCreditLimit, nil
}

// int64FromPtr converts an optional API number to a Terraform value.
func int64FromPtr(v *float64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}

// int64PtrFrom converts an optional Terraform number to a request value.
func int64PtrFrom(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	n := v.ValueInt64()
	return &n
}
