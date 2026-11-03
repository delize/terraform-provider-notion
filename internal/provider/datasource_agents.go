package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// Read-only views of the Notion Agent APIs (generally available since 2026-11-02):
// notion_agent, notion_agents, and notion_agent_insights.

var (
	_ datasource.DataSource                   = &AgentDataSource{}
	_ datasource.DataSource                   = &AgentsDataSource{}
	_ datasource.DataSourceWithValidateConfig = &AgentsDataSource{}
	_ datasource.DataSource                   = &AgentInsightsDataSource{}
	_ datasource.DataSourceWithValidateConfig = &AgentInsightsDataSource{}
)

func agentDataSourceClient(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *notionapi.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*notionapi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *notionapi.Client, got: %T.", req.ProviderData))
		return nil
	}
	return client
}

// ---- notion_agent ----

type AgentDataSource struct {
	client *notionapi.Client
}

type AgentDataSourceModel struct {
	AgentID              types.String `tfsdk:"agent_id"`
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Description          types.String `tfsdk:"description"`
	AgentType            types.String `tfsdk:"agent_type"`
	Status               types.String `tfsdk:"status"`
	PauseReason          types.String `tfsdk:"pause_reason"`
	InstructionsPageID   types.String `tfsdk:"instructions_page_id"`
	EffectiveCreditLimit types.Int64  `tfsdk:"effective_credit_limit"`
	CreditLimitHidden    types.Bool   `tfsdk:"credit_limit_hidden"`
	LastRunAt            types.String `tfsdk:"last_run_at"`
	CreatedTime          types.String `tfsdk:"created_time"`
	LastEditedTime       types.String `tfsdk:"last_edited_time"`
	RawJSON              types.String `tfsdk:"raw_json"`
}

func NewAgentDataSource() datasource.DataSource { return &AgentDataSource{} }

func (d *AgentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agent"
}

func (d *AgentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads one Custom Agent through the Notion Agent APIs. " +
			"Works with a personal access token, or a connection token with the View sessions and interact with agents capability that the agent is shared with.",
		Attributes: map[string]schema.Attribute{
			"agent_id": schema.StringAttribute{
				Description: "The agent ID, or notion_ai for the personal agent.",
				Required:    true,
			},
			"id":                   schema.StringAttribute{Computed: true, Description: "The agent ID."},
			"name":                 schema.StringAttribute{Computed: true, Description: "The agent's name."},
			"description":          schema.StringAttribute{Computed: true, Description: "The agent's description, or null."},
			"agent_type":           schema.StringAttribute{Computed: true, Description: "notion_ai, custom_agent, autofill_custom_agent, or external."},
			"status":               schema.StringAttribute{Computed: true, Description: "active, disabled, or deleted."},
			"pause_reason":         schema.StringAttribute{Computed: true, Description: "Why the agent is paused when status is disabled, or null."},
			"instructions_page_id": schema.StringAttribute{Computed: true, Description: "Page holding the agent's instructions, or null when they are inline."},
			"effective_credit_limit": schema.Int64Attribute{
				Computed:    true,
				Description: "The enforced credit limit, folding in any workspace default. Null when uncapped or hidden.",
			},
			"credit_limit_hidden": schema.BoolAttribute{
				Computed:    true,
				Description: "True when the token lacks full access to the agent, so Notion hides the credit limit.",
			},
			"last_run_at": schema.StringAttribute{
				Computed:    true,
				Description: "ISO 8601 time of the most recent run. Null when it never ran or when the token lacks edit access.",
			},
			"created_time":     schema.StringAttribute{Computed: true, Description: "When the agent was created."},
			"last_edited_time": schema.StringAttribute{Computed: true, Description: "When the agent was last edited."},
			"raw_json": schema.StringAttribute{
				Computed:    true,
				Description: "The full agent object as JSON, for fields not modeled here such as model, connections, and triggers. Decode with jsondecode().",
			},
		},
	}
}

func (d *AgentDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = agentDataSourceClient(req, resp)
}

func (d *AgentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg AgentDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	token, err := tokenForClient(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Error reading agent", err.Error())
		return
	}
	a, raw, err := getAgent(ctx, token, cfg.AgentID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading agent", err.Error())
		return
	}

	limit, hidden := parseHiddenInt(a.CreditLimit)
	lastRun, _ := parseHiddenString(a.LastRunAt)
	cfg.ID = types.StringValue(a.ID)
	cfg.Name = types.StringValue(a.Name)
	cfg.Description = types.StringPointerValue(a.Description)
	cfg.AgentType = types.StringValue(a.AgentType)
	cfg.Status = types.StringValue(a.Status)
	cfg.PauseReason = types.StringPointerValue(a.PauseReason)
	cfg.InstructionsPageID = types.StringPointerValue(a.InstructionsPageID)
	cfg.EffectiveCreditLimit = types.Int64PointerValue(limit)
	cfg.CreditLimitHidden = types.BoolValue(hidden)
	cfg.LastRunAt = types.StringPointerValue(lastRun)
	cfg.CreatedTime = types.StringValue(a.CreatedTime)
	cfg.LastEditedTime = types.StringValue(a.LastEditedTime)
	cfg.RawJSON = types.StringValue(string(raw))

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// ---- notion_agents ----

type AgentsDataSource struct {
	client *notionapi.Client
}

type AgentsDataSourceModel struct {
	Query          types.String        `tfsdk:"query"`
	AgentType      types.String        `tfsdk:"agent_type"`
	Statuses       []types.String      `tfsdk:"statuses"`
	Filter         types.String        `tfsdk:"filter"`
	IncludeDeleted types.Bool          `tfsdk:"include_deleted"`
	Limit          types.Int64         `tfsdk:"limit"`
	Agents         []AgentSummaryModel `tfsdk:"agents"`
}

type AgentSummaryModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	AgentType            types.String `tfsdk:"agent_type"`
	Status               types.String `tfsdk:"status"`
	PauseReason          types.String `tfsdk:"pause_reason"`
	EffectiveCreditLimit types.Int64  `tfsdk:"effective_credit_limit"`
	LastRunAt            types.String `tfsdk:"last_run_at"`
	CreatedTime          types.String `tfsdk:"created_time"`
}

func NewAgentsDataSource() datasource.DataSource { return &AgentsDataSource{} }

func (d *AgentsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agents"
}

func (d *AgentsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists Custom Agents the token can access through the Notion Agent APIs. " +
			"A personal access token sees the agents its user can read. A connection token sees only agents shared with its connection.",
		Attributes: map[string]schema.Attribute{
			"query": schema.StringAttribute{
				Optional:    true,
				Description: "Case-insensitive substring match on agent names and descriptions.",
			},
			"agent_type": schema.StringAttribute{
				Optional:    true,
				Description: "Only return agents of this type: custom_agent or autofill_custom_agent. Conflicts with filter.",
			},
			"statuses": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Only return agents in these statuses: active, disabled, deleted. Listing deleted also needs include_deleted. Conflicts with filter.",
			},
			"filter": schema.StringAttribute{
				Optional: true,
				Description: "A raw Query agents filter as a JSON string, for conditions the shortcut attributes don't cover (created_by, created_time, model_mode, last_run_at, and and/or groups). " +
					"Example: jsonencode({ property = \"model_mode\", string = { equals = \"auto\" } }).",
			},
			"include_deleted": schema.BoolAttribute{
				Optional:    true,
				Description: "Include soft-deleted agents. Defaults to false.",
			},
			"limit": schema.Int64Attribute{
				Optional:    true,
				Description: "Maximum number of agents to return. Defaults to all.",
			},
			"agents": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Matching agents, newest first.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                     schema.StringAttribute{Computed: true, Description: "The agent ID."},
						"name":                   schema.StringAttribute{Computed: true, Description: "The agent's name."},
						"agent_type":             schema.StringAttribute{Computed: true, Description: "custom_agent or autofill_custom_agent."},
						"status":                 schema.StringAttribute{Computed: true, Description: "active, disabled, or deleted."},
						"pause_reason":           schema.StringAttribute{Computed: true, Description: "Why the agent is paused, or null."},
						"effective_credit_limit": schema.Int64Attribute{Computed: true, Description: "The enforced credit limit. Null when uncapped or hidden."},
						"last_run_at":            schema.StringAttribute{Computed: true, Description: "Most recent run. Null when never run or hidden."},
						"created_time":           schema.StringAttribute{Computed: true, Description: "When the agent was created."},
					},
				},
			},
		},
	}
}

func (d *AgentsDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var cfg AgentsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !cfg.Filter.IsNull() && (!cfg.AgentType.IsNull() || cfg.Statuses != nil) {
		resp.Diagnostics.AddAttributeError(path.Root("filter"), "Conflicting filters",
			"Use either filter or the agent_type and statuses shortcuts, not both.")
	}
	if !cfg.Filter.IsNull() && !cfg.Filter.IsUnknown() && !json.Valid([]byte(cfg.Filter.ValueString())) {
		resp.Diagnostics.AddAttributeError(path.Root("filter"), "Invalid filter", "filter must be a JSON object, for example from jsonencode().")
	}
	if !cfg.AgentType.IsNull() && !cfg.AgentType.IsUnknown() {
		if v := cfg.AgentType.ValueString(); v != "custom_agent" && v != "autofill_custom_agent" {
			resp.Diagnostics.AddAttributeError(path.Root("agent_type"), "Invalid agent_type",
				fmt.Sprintf("%q is not one of custom_agent, autofill_custom_agent.", v))
		}
	}
	for _, s := range cfg.Statuses {
		if s.IsUnknown() || s.IsNull() {
			continue
		}
		if v := s.ValueString(); v != "active" && v != "disabled" && v != "deleted" {
			resp.Diagnostics.AddAttributeError(path.Root("statuses"), "Invalid status",
				fmt.Sprintf("%q is not one of active, disabled, deleted.", v))
		}
	}
}

// buildAgentsQuery turns the data source config into a Query agents body.
func buildAgentsQuery(cfg AgentsDataSourceModel) (map[string]interface{}, error) {
	body := map[string]interface{}{}
	if !cfg.Query.IsNull() && cfg.Query.ValueString() != "" {
		body["query"] = cfg.Query.ValueString()
	}
	if cfg.IncludeDeleted.ValueBool() {
		body["include_deleted"] = true
	}

	var conditions []interface{}
	if !cfg.AgentType.IsNull() {
		conditions = append(conditions, map[string]interface{}{
			"property": "agent_type",
			"string":   map[string]interface{}{"equals": cfg.AgentType.ValueString()},
		})
	}
	if len(cfg.Statuses) > 0 {
		in := make([]string, 0, len(cfg.Statuses))
		for _, s := range cfg.Statuses {
			in = append(in, s.ValueString())
		}
		conditions = append(conditions, map[string]interface{}{
			"property": "status",
			"status":   map[string]interface{}{"in": in},
		})
	}
	switch {
	case !cfg.Filter.IsNull():
		raw, err := compactJSON(cfg.Filter.ValueString())
		if err != nil {
			return nil, fmt.Errorf("filter is not valid JSON: %w", err)
		}
		body["filter"] = raw
	case len(conditions) == 1:
		body["filter"] = conditions[0]
	case len(conditions) > 1:
		body["filter"] = map[string]interface{}{"and": conditions}
	}
	return body, nil
}

func (d *AgentsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = agentDataSourceClient(req, resp)
}

func (d *AgentsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg AgentsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	token, err := tokenForClient(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Error querying agents", err.Error())
		return
	}
	body, err := buildAgentsQuery(cfg)
	if err != nil {
		resp.Diagnostics.AddError("Error querying agents", err.Error())
		return
	}
	agents, err := queryAgents(ctx, token, body, int(cfg.Limit.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error querying agents", err.Error())
		return
	}

	cfg.Agents = make([]AgentSummaryModel, 0, len(agents))
	for _, a := range agents {
		limit, _ := parseHiddenInt(a.CreditLimit)
		lastRun, _ := parseHiddenString(a.LastRunAt)
		cfg.Agents = append(cfg.Agents, AgentSummaryModel{
			ID:                   types.StringValue(a.ID),
			Name:                 types.StringValue(a.Name),
			AgentType:            types.StringValue(a.AgentType),
			Status:               types.StringValue(a.Status),
			PauseReason:          types.StringPointerValue(a.PauseReason),
			EffectiveCreditLimit: types.Int64PointerValue(limit),
			LastRunAt:            types.StringPointerValue(lastRun),
			CreatedTime:          types.StringValue(a.CreatedTime),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// ---- notion_agent_insights ----

type AgentInsightsDataSource struct {
	client *notionapi.Client
}

type AgentInsightsDataSourceModel struct {
	AgentID           types.String  `tfsdk:"agent_id"`
	StartTime         types.Int64   `tfsdk:"start_time"`
	EndTime           types.Int64   `tfsdk:"end_time"`
	Name              types.String  `tfsdk:"name"`
	AgentType         types.String  `tfsdk:"agent_type"`
	Status            types.String  `tfsdk:"status"`
	TotalCreditsUsed  types.Float64 `tfsdk:"total_credits_used"`
	RunsCompleted     types.Int64   `tfsdk:"runs_completed"`
	CreditLimit       types.Int64   `tfsdk:"credit_limit"`
	CreditLimitHidden types.Bool    `tfsdk:"credit_limit_hidden"`
}

func NewAgentInsightsDataSource() datasource.DataSource { return &AgentInsightsDataSource{} }

func (d *AgentInsightsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agent_insights"
}

func (d *AgentInsightsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads credit usage and run counts for a Custom Agent through the Notion Agent APIs. " +
			"Pass notion_ai for the personal agent, which only works with a personal access token and covers only that token's user.",
		Attributes: map[string]schema.Attribute{
			"agent_id": schema.StringAttribute{Required: true, Description: "The agent ID, or notion_ai."},
			"start_time": schema.Int64Attribute{
				Optional:    true,
				Description: "Start of the window as epoch seconds. Set together with end_time. Defaults to the current billing period.",
			},
			"end_time": schema.Int64Attribute{
				Optional:    true,
				Description: "End of the window as epoch seconds. Set together with start_time.",
			},
			"name":               schema.StringAttribute{Computed: true, Description: "The agent's name."},
			"agent_type":         schema.StringAttribute{Computed: true, Description: "notion_ai, custom_agent, or autofill_custom_agent."},
			"status":             schema.StringAttribute{Computed: true, Description: "active, disabled, or deleted."},
			"total_credits_used": schema.Float64Attribute{Computed: true, Description: "Premium AI credits used in the window."},
			"runs_completed":     schema.Int64Attribute{Computed: true, Description: "Runs recorded in the window."},
			"credit_limit": schema.Int64Attribute{
				Computed:    true,
				Description: "The enforced per-agent limit, or the workspace's per-member default for notion_ai. Null when none is set or hidden.",
			},
			"credit_limit_hidden": schema.BoolAttribute{Computed: true, Description: "True when the token lacks full access to the agent."},
		},
	}
}

func (d *AgentInsightsDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var cfg AgentInsightsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.StartTime.IsNull() != cfg.EndTime.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("start_time"), "Incomplete window",
			"Set start_time and end_time together, or neither for the current billing period.")
	}
}

func (d *AgentInsightsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = agentDataSourceClient(req, resp)
}

func (d *AgentInsightsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg AgentInsightsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	token, err := tokenForClient(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Error reading agent insights", err.Error())
		return
	}
	in, err := getAgentInsights(ctx, token, cfg.AgentID.ValueString(), cfg.StartTime.ValueInt64Pointer(), cfg.EndTime.ValueInt64Pointer())
	if err != nil {
		resp.Diagnostics.AddError("Error reading agent insights", err.Error())
		return
	}

	limit, hidden := parseHiddenInt(in.CreditLimit)
	cfg.Name = types.StringValue(in.Name)
	cfg.AgentType = types.StringValue(in.AgentType)
	cfg.Status = types.StringValue(in.Status)
	cfg.TotalCreditsUsed = types.Float64Value(in.TotalCreditsUsed)
	cfg.RunsCompleted = types.Int64Value(int64(in.RunsCompleted))
	cfg.CreditLimit = types.Int64PointerValue(limit)
	cfg.CreditLimitHidden = types.BoolValue(hidden)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
