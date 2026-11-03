package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// notion_agent_settings manages the name, description, model, enabled status,
// and credit limit of an existing Custom Agent through the Notion Agent APIs
// (generally available since 2026-11-02). The API can't create agents, so the
// resource adopts one by ID. Destroying the resource only removes it from
// state. It never deletes or changes the agent.
//
// Every setting is managed only when it is configured. The Optional+Computed
// ones (name, description, model_mode, model_id) read back from Notion when
// unset and keep their prior value in the plan, so nothing is sent for them.

var (
	_ resource.Resource                   = &AgentSettingsResource{}
	_ resource.ResourceWithImportState    = &AgentSettingsResource{}
	_ resource.ResourceWithValidateConfig = &AgentSettingsResource{}
)

type AgentSettingsResource struct {
	client *notionapi.Client
}

type AgentSettingsModel struct {
	ID                   types.String `tfsdk:"id"`
	AgentID              types.String `tfsdk:"agent_id"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	CreditLimit          types.Int64  `tfsdk:"credit_limit"`
	UnlimitedCredits     types.Bool   `tfsdk:"unlimited_credits"`
	Name                 types.String `tfsdk:"name"`
	Description          types.String `tfsdk:"description"`
	ModelMode            types.String `tfsdk:"model_mode"`
	ModelID              types.String `tfsdk:"model_id"`
	AgentType            types.String `tfsdk:"agent_type"`
	Status               types.String `tfsdk:"status"`
	PauseReason          types.String `tfsdk:"pause_reason"`
	EffectiveCreditLimit types.Int64  `tfsdk:"effective_credit_limit"`
}

func NewAgentSettingsResource() resource.Resource {
	return &AgentSettingsResource{}
}

func (r *AgentSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agent_settings"
}

func (r *AgentSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the name, description, model, enabled status, and credit limit of an existing Notion Custom Agent through the Notion Agent APIs. " +
			"The API can't create agents, so this resource adopts one by ID. Destroying it only removes it from Terraform state and leaves the agent and its settings as they are.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Same as agent_id.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"agent_id": schema.StringAttribute{
				Description: "ID of the Custom Agent. The personal agent (notion_ai) has no status or credit limit controls and is rejected.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the agent may run. Omit to leave the status unmanaged. Needs edit access to the agent. " +
					"Pauses Notion applies on its own, such as a reached credit or run limit, are not treated as drift. See status and pause_reason for those.",
				Optional: true,
			},
			"credit_limit": schema.Int64Attribute{
				Description: "The agent's own credit limit, a non-negative integer. Omit to leave it unmanaged. Needs full access to the agent. " +
					"The API only reports the effective limit afterwards, so a change made outside Terraform is not detected.",
				Optional: true,
			},
			"unlimited_credits": schema.BoolAttribute{
				Description: "Set to true to clear the agent's own credit limit. Conflicts with credit_limit. A workspace default limit can still apply.",
				Optional:    true,
			},
			"name": schema.StringAttribute{
				Description: "The agent's name. Omit to leave it unmanaged, in which case it reads back from Notion. Needs edit access to the agent.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"description": schema.StringAttribute{
				Description: "The agent's short description. Set to an empty string to clear it. Omit to leave it unmanaged. " +
					"An agent without a description reads back as an empty string. Needs edit access to the agent.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"model_mode": schema.StringAttribute{
				Description: "auto to let Notion choose the model for each run, or pinned to use model_id. Omit to leave the model unmanaged. " +
					"Changing the model also updates the agent's published version.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"model_id": schema.StringAttribute{
				Description: "Public model ID to pin, for example claude-sonnet-5. Required when model_mode is pinned and not allowed otherwise. " +
					"Null when the model is auto. Notion reports an early-access model without an ID, so the configured ID is kept in that case.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					modelIDPlanModifier{},
				},
			},
			"agent_type": schema.StringAttribute{
				Description: "custom_agent or autofill_custom_agent.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Description: "active, disabled, or deleted, as reported by Notion.",
				Computed:    true,
			},
			"pause_reason": schema.StringAttribute{
				Description: "Why the agent is paused when status is disabled, for example disabled_from_api or credit_limit. Null when active.",
				Computed:    true,
			},
			"effective_credit_limit": schema.Int64Attribute{
				Description: "The limit Notion enforces, folding in the agent's own limit and any workspace default. Null when uncapped or when the token lacks full access to the agent.",
				Computed:    true,
			},
		},
	}
}

func (r *AgentSettingsResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg AgentSettingsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	validateAgentModelConfig(cfg, &resp.Diagnostics)
	if !cfg.Name.IsNull() && !cfg.Name.IsUnknown() && cfg.Name.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid name",
			"name can't be empty. Omit it to leave the agent's name unmanaged.")
	}
	if !cfg.AgentID.IsUnknown() && cfg.AgentID.ValueString() == "notion_ai" {
		resp.Diagnostics.AddAttributeError(path.Root("agent_id"), "Personal agent not supported",
			"notion_ai is the personal agent, which has no status or credit limit controls. Use a Custom Agent ID.")
	}
	if !cfg.CreditLimit.IsNull() && !cfg.CreditLimit.IsUnknown() && cfg.CreditLimit.ValueInt64() < 0 {
		resp.Diagnostics.AddAttributeError(path.Root("credit_limit"), "Invalid credit limit",
			"credit_limit must be zero or more. Use unlimited_credits = true to clear the limit.")
	}
	if !cfg.UnlimitedCredits.IsNull() && !cfg.UnlimitedCredits.IsUnknown() {
		if !cfg.UnlimitedCredits.ValueBool() {
			resp.Diagnostics.AddAttributeError(path.Root("unlimited_credits"), "Invalid unlimited_credits",
				"Set unlimited_credits to true to clear the limit, or omit it.")
		}
		if !cfg.CreditLimit.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("unlimited_credits"), "Conflicting credit settings",
				"Set either credit_limit or unlimited_credits, not both.")
		}
	}
}

// validateAgentModelConfig checks model_mode and model_id at plan time.
// model_id needs model_mode = "pinned" in the same config, and pinned needs
// model_id. Unknown values are skipped until they are known.
func validateAgentModelConfig(cfg AgentSettingsModel, diags *diag.Diagnostics) {
	if cfg.ModelMode.IsUnknown() || cfg.ModelID.IsUnknown() {
		return
	}
	mode := cfg.ModelMode.ValueString()
	if !cfg.ModelMode.IsNull() && mode != "auto" && mode != "pinned" {
		diags.AddAttributeError(path.Root("model_mode"), "Invalid model_mode",
			fmt.Sprintf("model_mode must be \"auto\" or \"pinned\", got %q.", mode))
		return
	}
	if !cfg.ModelID.IsNull() {
		if mode != "pinned" {
			diags.AddAttributeError(path.Root("model_id"), "model_id needs a pinned model",
				"model_id is only allowed with model_mode = \"pinned\". Remove it for auto, or set model_mode = \"pinned\".")
		} else if cfg.ModelID.ValueString() == "" {
			diags.AddAttributeError(path.Root("model_id"), "Invalid model_id", "model_id can't be empty.")
		}
	}
	if mode == "pinned" && cfg.ModelID.IsNull() {
		diags.AddAttributeError(path.Root("model_id"), "Missing model_id",
			"model_mode = \"pinned\" needs model_id, the public ID of the model to pin.")
	}
}

// modelIDPlanModifier plans model_id when it isn't configured. Auto has no
// model ID, so the plan is null. When model_mode keeps its prior value the
// prior ID stays, which leaves an unmanaged model alone. Otherwise the ID is
// known after apply.
type modelIDPlanModifier struct{}

func (modelIDPlanModifier) Description(context.Context) string {
	return "Null for auto, the prior value while model_mode is unchanged, unknown otherwise."
}

func (m modelIDPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (modelIDPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if !req.ConfigValue.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var mode types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("model_mode"), &mode)...)
	if resp.Diagnostics.HasError() || mode.IsUnknown() {
		return
	}
	if mode.ValueString() == "auto" {
		resp.PlanValue = types.StringNull()
		return
	}
	if req.State.Raw.IsNull() {
		return
	}
	var priorMode types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("model_mode"), &priorMode)...)
	if mode.Equal(priorMode) {
		resp.PlanValue = req.StateValue
	}
}

func (r *AgentSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*notionapi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *notionapi.Client, got: %T.", req.ProviderData))
		return
	}
	r.client = client
}

func (r *AgentSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AgentSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, nil, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AgentSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state AgentSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// apply pushes the managed settings, then refreshes the computed attributes.
// prior is nil on create. Name, description, model, and status go out in one
// PATCH built by agentUpdateFields, and the credit limit has its own
// endpoint. Status is only changed when the agent's current state differs
// from the desired one, because Notion can refuse to re-enable an agent it
// paused itself.
func (r *AgentSettingsResource) apply(ctx context.Context, plan, prior *AgentSettingsModel, diags *diag.Diagnostics) {
	token, err := tokenForClient(r.client)
	if err != nil {
		diags.AddError("Error configuring agent settings", err.Error())
		return
	}
	agentID := plan.AgentID.ValueString()

	current, _, err := getAgent(ctx, token, agentID)
	if err != nil {
		diags.AddError("Error reading agent", err.Error())
		return
	}
	if current.Status == "deleted" {
		diags.AddError("Agent is deleted", fmt.Sprintf("Agent %s has been deleted in Notion.", agentID))
		return
	}

	fields, why := agentUpdateFields(plan, prior, current)
	if why != "" {
		diags.AddWarning("Agent status left unchanged", why)
	}
	if len(fields) > 0 {
		if _, err := updateAgent(ctx, token, agentID, fields); err != nil {
			diags.AddError("Error updating agent", err.Error())
			return
		}
	}

	creditChanged := prior == nil ||
		!plan.CreditLimit.Equal(prior.CreditLimit) ||
		!plan.UnlimitedCredits.Equal(prior.UnlimitedCredits)
	if creditChanged {
		switch {
		case plan.UnlimitedCredits.ValueBool():
			if _, err := updateAgentCreditLimit(ctx, token, agentID, nil); err != nil {
				diags.AddError("Error clearing agent credit limit", err.Error())
				return
			}
		case !plan.CreditLimit.IsNull() && !plan.CreditLimit.IsUnknown():
			v := plan.CreditLimit.ValueInt64()
			if _, err := updateAgentCreditLimit(ctx, token, agentID, &v); err != nil {
				diags.AddError("Error setting agent credit limit", err.Error())
				return
			}
		}
	}

	refreshed, _, err := getAgent(ctx, token, agentID)
	if err != nil {
		diags.AddError("Error reading agent after update", err.Error())
		return
	}
	plan.ID = types.StringValue(agentID)
	if warn := setAgentComputed(plan, refreshed); warn != "" {
		diags.AddWarning("Pinned model has no public ID", warn)
	}
}

// agentUpdateFields builds the PATCH /v1/agents/{id} body from the diff
// between plan and prior state. prior is nil on create. A field goes in only
// when it is configured, its planned value differs from the prior state, and
// it differs from what the agent has now. That keeps unmanaged fields out of
// the request and avoids resending a model, which would update the agent's
// published version. Status follows statusChangeNeeded instead, and the
// returned string is its warning.
func agentUpdateFields(plan, prior *AgentSettingsModel, current *agentObject) (map[string]interface{}, string) {
	fields := map[string]interface{}{}
	why := ""
	if !plan.Enabled.IsNull() && !plan.Enabled.IsUnknown() {
		var send bool
		send, why = statusChangeNeeded(plan.Enabled.ValueBool(), current)
		if send {
			status := "disabled"
			if plan.Enabled.ValueBool() {
				status = "active"
			}
			fields["status"] = status
		}
	}

	changed := func(planned types.String, priorValue func(*AgentSettingsModel) types.String) bool {
		if planned.IsNull() || planned.IsUnknown() {
			return false
		}
		return prior == nil || !planned.Equal(priorValue(prior))
	}

	if changed(plan.Name, func(m *AgentSettingsModel) types.String { return m.Name }) &&
		plan.Name.ValueString() != current.Name {
		fields["name"] = plan.Name.ValueString()
	}

	currentDescription := ""
	if current.Description != nil {
		currentDescription = *current.Description
	}
	if changed(plan.Description, func(m *AgentSettingsModel) types.String { return m.Description }) &&
		plan.Description.ValueString() != currentDescription {
		fields["description"] = plan.Description.ValueString()
	}

	modelChanged := changed(plan.ModelMode, func(m *AgentSettingsModel) types.String { return m.ModelMode }) ||
		changed(plan.ModelID, func(m *AgentSettingsModel) types.String { return m.ModelID })
	if modelChanged && !plan.ModelMode.IsNull() && !plan.ModelMode.IsUnknown() {
		want := agentModel{Mode: plan.ModelMode.ValueString()}
		if want.Mode == "pinned" {
			id := plan.ModelID.ValueString()
			want.ID = &id
		}
		if !sameAgentModel(want, current.Model) {
			fields["model"] = want
		}
	}
	return fields, why
}

// sameAgentModel reports whether the agent already runs on the wanted model.
// A pinned model that reads back without an ID (early access) can't be
// compared, so it counts as different.
func sameAgentModel(want agentModel, have *agentModel) bool {
	if have == nil || have.Mode != want.Mode {
		return false
	}
	if want.Mode != "pinned" {
		return true
	}
	return have.ID != nil && want.ID != nil && *have.ID == *want.ID
}

// statusChangeNeeded decides whether to PATCH the status. It returns a
// warning message when the desired state can't be reached because Notion
// paused the agent for a reason this resource doesn't control.
func statusChangeNeeded(wantEnabled bool, a *agentObject) (bool, string) {
	reason := ""
	if a.PauseReason != nil {
		reason = *a.PauseReason
	}
	if wantEnabled {
		switch {
		case a.Status == "active":
			return false, ""
		case manualPauseReasons[reason]:
			return true, ""
		default:
			return false, fmt.Sprintf("Agent %s is paused by Notion (pause_reason %q). That pause isn't controlled through enabled, so the agent stays paused until the cause is resolved in Notion.", a.ID, reason)
		}
	}
	return a.Status == "active", ""
}

// enabledFromAgent maps the agent's status onto the enabled attribute.
// Automatic pauses keep the prior value so they don't show up as drift.
func enabledFromAgent(prior types.Bool, a *agentObject) types.Bool {
	if prior.IsNull() || prior.IsUnknown() {
		return prior
	}
	if a.Status == "active" {
		return types.BoolValue(true)
	}
	if a.PauseReason != nil && manualPauseReasons[*a.PauseReason] {
		return types.BoolValue(false)
	}
	return prior
}

// setAgentComputed copies what Notion reports into the model. A missing
// description reads as "". The model fields keep their prior values when the
// response has no model, and model_id keeps its prior value when a pinned
// model reads back without an ID, which Notion does for early-access models.
// In that last case it returns a warning so the user knows the ID wasn't
// checked.
func setAgentComputed(m *AgentSettingsModel, a *agentObject) string {
	m.Name = types.StringValue(a.Name)
	description := ""
	if a.Description != nil {
		description = *a.Description
	}
	m.Description = types.StringValue(description)
	m.AgentType = types.StringValue(a.AgentType)
	m.Status = types.StringValue(a.Status)
	m.PauseReason = types.StringPointerValue(a.PauseReason)
	limit, _ := parseHiddenInt(a.CreditLimit)
	m.EffectiveCreditLimit = types.Int64PointerValue(limit)

	known := func(v types.String) types.String {
		if v.IsUnknown() {
			return types.StringNull()
		}
		return v
	}
	warn := ""
	switch {
	case a.Model == nil:
		// The docs don't make model optional, but keep what we have rather
		// than wiping it if a response leaves it out.
		m.ModelMode = known(m.ModelMode)
		m.ModelID = known(m.ModelID)
	case a.Model.Mode == "pinned" && a.Model.ID == nil:
		m.ModelMode = types.StringValue("pinned")
		m.ModelID = known(m.ModelID)
		if !m.ModelID.IsNull() {
			warn = fmt.Sprintf("Agent %s is pinned to a model Notion reports without a public ID, which happens for early-access models. "+
				"model_id keeps %q and can't be checked against Notion until the model has a public ID.", a.ID, m.ModelID.ValueString())
		}
	default:
		m.ModelMode = types.StringValue(a.Model.Mode)
		m.ModelID = types.StringPointerValue(a.Model.ID)
	}
	return warn
}

func (r *AgentSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AgentSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, err := tokenForClient(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error reading agent", err.Error())
		return
	}

	a, _, err := getAgent(ctx, token, state.AgentID.ValueString())
	var nf *agentNotFoundError
	if errors.As(err, &nf) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading agent", err.Error())
		return
	}
	if a.Status == "deleted" {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(state.AgentID.ValueString())
	state.Enabled = enabledFromAgent(state.Enabled, a)
	if warn := setAgentComputed(&state, a); warn != "" {
		resp.Diagnostics.AddWarning("Pinned model has no public ID", warn)
	}
	// credit_limit keeps its configured value: GET only returns the
	// effective limit, so the agent's own limit can't be read back.

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Delete leaves the agent untouched. Removing the resource only stops
// Terraform from managing these settings.
func (r *AgentSettingsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *AgentSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("agent_id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
