package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource      = &AdminAgentsDataSource{}
	_ datasource.DataSource      = &AdminAgentCreditUsageDataSource{}
	_ datasource.DataSource      = &AdminAgentsCreditUsageDataSource{}
	_ action.ActionWithConfigure = &AdminAgentDeleteAction{}
)

func int64Attr(desc string) schema.Int64Attribute {
	return schema.Int64Attribute{Description: desc, Computed: true}
}

type CreditPeriodModel struct {
	StartMS     types.Int64 `tfsdk:"start_ms"`
	EndMS       types.Int64 `tfsdk:"end_ms"`
	CreditsUsed types.Int64 `tfsdk:"credits_used"`
}

var creditPeriodSchema = schema.ListNestedAttribute{
	Description: "Credits used per monthly billing period, newest first.",
	Computed:    true,
	NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
		"start_ms":     int64Attr("Period start, epoch milliseconds, inclusive."),
		"end_ms":       int64Attr("Period end, epoch milliseconds, exclusive."),
		"credits_used": int64Attr("Premium AI credits used in the period."),
	}},
}

func creditPeriods(in []adminCreditPeriod) []CreditPeriodModel {
	out := make([]CreditPeriodModel, 0, len(in))
	for _, p := range in {
		out = append(out, CreditPeriodModel{
			StartMS: types.Int64Value(int64(p.StartMS)), EndMS: types.Int64Value(int64(p.EndMS)), CreditsUsed: types.Int64Value(int64(p.CreditsUsed)),
		})
	}
	return out
}

func nullableString(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// ---- notion_admin_agents ----

type AdminAgentsDataSource struct{ adminDataSourceBase }

type AdminAgentsDataSourceModel struct {
	WorkspaceID types.String      `tfsdk:"workspace_id"`
	Name        types.String      `tfsdk:"name"`
	Statuses    []string          `tfsdk:"statuses"`
	Agents      []AdminAgentEntry `tfsdk:"agents"`
}

type AdminAgentEntry struct {
	ID                  types.String           `tfsdk:"id"`
	Name                types.String           `tfsdk:"name"`
	Description         types.String           `tfsdk:"description"`
	Type                types.String           `tfsdk:"type"`
	Status              types.String           `tfsdk:"status"`
	Alive               types.Bool             `tfsdk:"alive"`
	Model               types.String           `tfsdk:"model"`
	CreatedByID         types.String           `tfsdk:"created_by_id"`
	CreatedTime         types.Int64            `tfsdk:"created_time"`
	LastEditedTime      types.Int64            `tfsdk:"last_edited_time"`
	LastRunTime         types.Int64            `tfsdk:"last_run_time"`
	LastCreditUsageTime types.Int64            `tfsdk:"last_credit_usage_time"`
	ParentID            types.String           `tfsdk:"parent_id"`
	ParentTable         types.String           `tfsdk:"parent_table"`
	Permissions         []AgentPermissionEntry `tfsdk:"permissions"`
}

type AgentPermissionEntry struct {
	PrincipalType types.String `tfsdk:"principal_type"`
	PrincipalID   types.String `tfsdk:"principal_id"`
	Role          types.String `tfsdk:"role"`
	ResolvedRole  types.String `tfsdk:"resolved_role"`
}

func NewAdminAgentsDataSource() datasource.DataSource { return &AdminAgentsDataSource{} }

func (d *AdminAgentsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_admin_agents"
}

func (d *AdminAgentsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the custom and database autofill agents in a workspace through the Admin API. Requires admin_token with the workflows:read scope. " +
			"Deleted agents are included only when statuses contains \"deleted\".",
		Attributes: map[string]schema.Attribute{
			"workspace_id": schema.StringAttribute{Description: "The workspace.", Required: true},
			"name":         schema.StringAttribute{Description: "Case-insensitive substring match on agent name or description.", Optional: true},
			"statuses": schema.ListAttribute{
				Description: "Only agents with these statuses, such as active, disabled (any admin-disabled agent), credit_limit, or deleted.",
				Optional:    true, ElementType: types.StringType,
			},
			"agents": schema.ListNestedAttribute{
				Description: "Agents.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":                     computedString("The agent ID."),
					"name":                   computedString("The agent's name."),
					"description":            computedString("The agent's description, or null."),
					"type":                   computedString("custom or database_autofill."),
					"status":                 computedString("active, deleted, or a paused reason."),
					"alive":                  schema.BoolAttribute{Description: "False for deleted agents.", Computed: true},
					"model":                  computedString("Model display name, or null when the agent picks automatically."),
					"created_by_id":          computedString("ID of the creator."),
					"created_time":           int64Attr("Creation time, epoch milliseconds."),
					"last_edited_time":       int64Attr("Last edit, epoch milliseconds."),
					"last_run_time":          int64Attr("Last run, epoch milliseconds, or null if it never ran."),
					"last_credit_usage_time": int64Attr("Last credit use, epoch milliseconds, or null."),
					"parent_id":              computedString("The workspace for custom agents, or the database block for autofill agents."),
					"parent_table":           computedString("space or block."),
					"permissions": schema.ListNestedAttribute{
						Description: "Sharing, workspace grant first, at most 250. Empty for autofill agents.",
						Computed:    true,
						NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
							"principal_type": computedString("user, group, or workspace."),
							"principal_id":   computedString("User or group ID, null for workspace."),
							"role":           computedString("The stored role."),
							"resolved_role":  computedString("The effective role, counting other grants."),
						}},
					},
				}},
			},
		},
	}
}

func agentPermissionEntries(in []adminAgentPermission) []AgentPermissionEntry {
	out := make([]AgentPermissionEntry, 0, len(in))
	for _, p := range in {
		out = append(out, AgentPermissionEntry{
			PrincipalType: types.StringValue(p.Principal.Type),
			PrincipalID:   nullableString(p.Principal.id()),
			Role:          types.StringValue(p.Role),
			ResolvedRole:  types.StringValue(p.ResolvedRole),
		})
	}
	return out
}

func (d *AdminAgentsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state AdminAgentsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}
	agents, err := admin.ListAgents(ctx, state.WorkspaceID.ValueString(), agentListFilter{Name: state.Name.ValueString(), Statuses: state.Statuses})
	if err != nil {
		resp.Diagnostics.AddError("Error listing agents", err.Error())
		return
	}
	state.Agents = make([]AdminAgentEntry, 0, len(agents))
	for _, a := range agents {
		state.Agents = append(state.Agents, AdminAgentEntry{
			ID: types.StringValue(a.ID), Name: types.StringValue(a.Name), Description: nullableString(a.Description),
			Type: types.StringValue(a.Type), Status: types.StringValue(a.Status), Alive: types.BoolValue(a.Alive),
			Model: nullableString(a.Model), CreatedByID: nullableString(a.CreatedByID),
			CreatedTime: int64FromPtr(a.CreatedTime), LastEditedTime: int64FromPtr(a.LastEditedTime),
			LastRunTime: int64FromPtr(a.LastRunTime), LastCreditUsageTime: int64FromPtr(a.LastCreditUsageTime),
			ParentID: nullableString(a.ParentID), ParentTable: nullableString(a.ParentTable),
			Permissions: agentPermissionEntries(a.Permissions),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- notion_admin_agent_credit_usage ----

type AdminAgentCreditUsageDataSource struct{ adminDataSourceBase }

type AdminAgentCreditUsageModel struct {
	WorkspaceID         types.String        `tfsdk:"workspace_id"`
	AgentID             types.String        `tfsdk:"agent_id"`
	Name                types.String        `tfsdk:"name"`
	Type                types.String        `tfsdk:"type"`
	Status              types.String        `tfsdk:"status"`
	CreditLimit         types.Int64         `tfsdk:"credit_limit"`
	TotalCreditsUsed    types.Int64         `tfsdk:"total_credits_used"`
	RunsCompleted       types.Int64         `tfsdk:"runs_completed"`
	LastCreditUsageTime types.Int64         `tfsdk:"last_credit_usage_time"`
	Periods             []CreditPeriodModel `tfsdk:"periods"`
}

func NewAdminAgentCreditUsageDataSource() datasource.DataSource {
	return &AdminAgentCreditUsageDataSource{}
}

func (d *AdminAgentCreditUsageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_admin_agent_credit_usage"
}

func (d *AdminAgentCreditUsageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads one agent's premium AI credit usage, with every billing period, through the Admin API. " +
			"Works for custom agents and database autofill agents, including deleted ones. Requires admin_token with the workflows:read scope.",
		Attributes: map[string]schema.Attribute{
			"workspace_id":           schema.StringAttribute{Description: "The workspace.", Required: true},
			"agent_id":               schema.StringAttribute{Description: "The agent.", Required: true},
			"name":                   computedString("The agent's name."),
			"type":                   computedString("custom or database_autofill."),
			"status":                 computedString("The agent's status."),
			"credit_limit":           int64Attr("The enforced per-agent credit limit, or null."),
			"total_credits_used":     int64Attr("All-time credits used."),
			"runs_completed":         int64Attr("All-time finished runs."),
			"last_credit_usage_time": int64Attr("Last credit use, epoch milliseconds, or null."),
			"periods":                creditPeriodSchema,
		},
	}
}

func (d *AdminAgentCreditUsageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state AdminAgentCreditUsageModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}
	u, err := admin.GetAgentCreditUsage(ctx, state.WorkspaceID.ValueString(), state.AgentID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading agent credit usage", err.Error())
		return
	}
	state.Name, state.Type, state.Status = types.StringValue(u.Name), types.StringValue(u.Type), types.StringValue(u.Status)
	state.CreditLimit = int64FromPtr(u.CreditLimit)
	state.TotalCreditsUsed = types.Int64Value(int64(u.TotalCreditsUsed))
	state.RunsCompleted = types.Int64Value(int64(u.RunsCompleted))
	state.LastCreditUsageTime = int64FromPtr(u.LastCreditUsageTime)
	state.Periods = creditPeriods(u.Periods)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- notion_admin_agents_credit_usage ----

type AdminAgentsCreditUsageDataSource struct{ adminDataSourceBase }

type AdminAgentsCreditUsageModel struct {
	WorkspaceID    types.String            `tfsdk:"workspace_id"`
	IncludeDeleted types.Bool              `tfsdk:"include_deleted"`
	Agents         []AgentCreditUsageEntry `tfsdk:"agents"`
}

type AgentCreditUsageEntry struct {
	ID                  types.String        `tfsdk:"id"`
	Name                types.String        `tfsdk:"name"`
	Type                types.String        `tfsdk:"type"`
	Status              types.String        `tfsdk:"status"`
	CreditLimit         types.Int64         `tfsdk:"credit_limit"`
	TotalCreditsUsed    types.Int64         `tfsdk:"total_credits_used"`
	RunsCompleted       types.Int64         `tfsdk:"runs_completed"`
	LastCreditUsageTime types.Int64         `tfsdk:"last_credit_usage_time"`
	Periods             []CreditPeriodModel `tfsdk:"periods"`
}

func NewAdminAgentsCreditUsageDataSource() datasource.DataSource {
	return &AdminAgentsCreditUsageDataSource{}
}

func (d *AdminAgentsCreditUsageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_admin_agents_credit_usage"
}

func (d *AdminAgentsCreditUsageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads credit usage for every agent in a workspace through the Admin API. Requires admin_token with the workflows:read scope. " +
			"Each agent's periods hold only the first page Notion returns, starting with the current period. Use notion_admin_agent_credit_usage for one agent's full history.",
		Attributes: map[string]schema.Attribute{
			"workspace_id":    schema.StringAttribute{Description: "The workspace.", Required: true},
			"include_deleted": schema.BoolAttribute{Description: "Include deleted agents that still have usage. Defaults to false.", Optional: true},
			"agents": schema.ListNestedAttribute{
				Description: "Agents with their usage.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":                     computedString("The agent ID."),
					"name":                   computedString("The agent's name."),
					"type":                   computedString("custom or database_autofill."),
					"status":                 computedString("The agent's status."),
					"credit_limit":           int64Attr("The enforced per-agent credit limit, or null."),
					"total_credits_used":     int64Attr("All-time credits used."),
					"runs_completed":         int64Attr("All-time finished runs."),
					"last_credit_usage_time": int64Attr("Last credit use, epoch milliseconds, or null."),
					"periods":                creditPeriodSchema,
				}},
			},
		},
	}
}

func (d *AdminAgentsCreditUsageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state AdminAgentsCreditUsageModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}
	usage, err := admin.ListAgentsCreditUsage(ctx, state.WorkspaceID.ValueString(), state.IncludeDeleted.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Error listing agent credit usage", err.Error())
		return
	}
	state.Agents = make([]AgentCreditUsageEntry, 0, len(usage))
	for _, u := range usage {
		state.Agents = append(state.Agents, AgentCreditUsageEntry{
			ID: types.StringValue(u.ID), Name: types.StringValue(u.Name), Type: types.StringValue(u.Type), Status: types.StringValue(u.Status),
			CreditLimit: int64FromPtr(u.CreditLimit), TotalCreditsUsed: types.Int64Value(int64(u.TotalCreditsUsed)),
			RunsCompleted: types.Int64Value(int64(u.RunsCompleted)), LastCreditUsageTime: int64FromPtr(u.LastCreditUsageTime),
			Periods: creditPeriods(u.Periods),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- action notion_admin_agent_delete ----

type AdminAgentDeleteAction struct{ adminActionBase }

type AdminAgentDeleteModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
	AgentID     types.String `tfsdk:"agent_id"`
}

func NewAdminAgentDeleteAction() action.Action { return &AdminAgentDeleteAction{} }

func (a *AdminAgentDeleteAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_admin_agent_delete"
}

func (a *AdminAgentDeleteAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		Description: "Deletes a custom agent (moves it to Trash) through the Admin API. Requires admin_token with the workflows:write scope. " +
			"The personal agent (Notion AI) isn't supported.",
		Attributes: map[string]actionschema.Attribute{
			"workspace_id": actionschema.StringAttribute{Description: "The workspace.", Required: true},
			"agent_id":     actionschema.StringAttribute{Description: "The custom agent to delete.", Required: true},
		},
	}
}

func (a *AdminAgentDeleteAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg AdminAgentDeleteModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin := a.admin(resp)
	if admin == nil {
		return
	}
	if err := admin.DeleteAgent(ctx, cfg.WorkspaceID.ValueString(), cfg.AgentID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting agent", err.Error())
		return
	}
	progress(resp, "Agent %s deleted", cfg.AgentID.ValueString())
}
