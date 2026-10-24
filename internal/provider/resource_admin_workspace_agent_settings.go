package provider

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// notion_admin_workspace_agent_settings sets who may create custom agents
// in a workspace and the workspace's default agent credit limit. The Admin
// API has update endpoints for both but no way to read them, so Read keeps
// the last applied values and drift made in Notion isn't detected. Destroy
// leaves both settings as they are.

var (
	_ resource.Resource                   = &AdminWorkspaceAgentSettingsResource{}
	_ resource.ResourceWithImportState    = &AdminWorkspaceAgentSettingsResource{}
	_ resource.ResourceWithValidateConfig = &AdminWorkspaceAgentSettingsResource{}
)

type AdminWorkspaceAgentSettingsResource struct {
	client *notionapi.Client
}

type AdminWorkspaceAgentSettingsModel struct {
	ID                       types.String `tfsdk:"id"`
	WorkspaceID              types.String `tfsdk:"workspace_id"`
	CreationPolicy           types.String `tfsdk:"creation_policy"`
	DisableExistingAgents    types.Bool   `tfsdk:"disable_existing_agents"`
	DefaultAgentCreditLimit  types.Int64  `tfsdk:"default_agent_credit_limit"`
	CurrentPolicyDescription types.String `tfsdk:"current_policy_description"`
}

func NewAdminWorkspaceAgentSettingsResource() resource.Resource {
	return &AdminWorkspaceAgentSettingsResource{}
}

func (r *AdminWorkspaceAgentSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_admin_workspace_agent_settings"
}

func (r *AdminWorkspaceAgentSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Sets who can create custom agents in a workspace and the workspace's default agent credit limit, through the Admin API. " +
			"Requires admin_token with the workflows:write scope. The API can't read these settings back, so changes made in Notion aren't detected, " +
			"and destroying the resource leaves both settings as they are.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Same as workspace_id.", Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"workspace_id": schema.StringAttribute{
				Description: "The workspace.", Required: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"creation_policy": schema.StringAttribute{
				Description: "Who can create custom agents: all_workspace_members, workspace_owners_only, or disabled. " +
					"Removing it from the configuration leaves the current policy in place.",
				Optional: true,
			},
			"disable_existing_agents": schema.BoolAttribute{
				Description: "With creation_policy = \"disabled\", also disable every existing custom agent in the workspace. " +
					"Sent only when the policy changes to disabled, and re-enabling agents afterwards is a per-agent change.",
				Optional: true,
			},
			"default_agent_credit_limit": schema.Int64Attribute{
				Description: "Default credit limit for agents going forward, in whole credits. Removing it from the configuration removes the default cap.",
				Optional:    true,
			},
			"current_policy_description": schema.StringAttribute{
				Description: "Notion's description of the policy after the last change to creation_policy.",
				Computed:    true,
			},
		},
	}
}

func (r *AdminWorkspaceAgentSettingsResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg AdminWorkspaceAgentSettingsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if v := cfg.CreationPolicy; !v.IsNull() && !v.IsUnknown() && !slices.Contains(agentCreationPolicies, v.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("creation_policy"), "Invalid creation policy",
			fmt.Sprintf("%q is not valid. Use all_workspace_members, workspace_owners_only, or disabled.", v.ValueString()))
	}
	if cfg.DisableExistingAgents.ValueBool() && !cfg.CreationPolicy.IsUnknown() && cfg.CreationPolicy.ValueString() != "disabled" {
		resp.Diagnostics.AddAttributeError(path.Root("disable_existing_agents"), "Needs a disabled policy",
			"disable_existing_agents only takes effect with creation_policy = \"disabled\".")
	}
	if v := cfg.DefaultAgentCreditLimit; !v.IsNull() && !v.IsUnknown() && v.ValueInt64() < 0 {
		resp.Diagnostics.AddAttributeError(path.Root("default_agent_credit_limit"), "Invalid credit limit", "The limit must be zero or more whole credits.")
	}
}

func (r *AdminWorkspaceAgentSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := providerDataFrom(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", err.Error())
		return
	}
	r.client = client
}

func (r *AdminWorkspaceAgentSettingsResource) apply(ctx context.Context, plan, state *AdminWorkspaceAgentSettingsModel) error {
	admin, err := adminClientFor(r.client)
	if err != nil {
		return err
	}
	ws := plan.WorkspaceID.ValueString()
	plan.ID = types.StringValue(ws)
	if state != nil {
		plan.CurrentPolicyDescription = state.CurrentPolicyDescription
	}
	if plan.CurrentPolicyDescription.IsUnknown() {
		plan.CurrentPolicyDescription = types.StringNull()
	}

	if !plan.CreationPolicy.IsNull() && (state == nil || !plan.CreationPolicy.Equal(state.CreationPolicy)) {
		res, err := admin.UpdateAgentCreationPolicy(ctx, ws, plan.CreationPolicy.ValueString(), plan.DisableExistingAgents.ValueBool())
		if err != nil {
			return fmt.Errorf("updating agent creation policy: %w", err)
		}
		plan.CurrentPolicyDescription = types.StringValue(res.CurrentPolicyDescription)
	}

	limitChanged := state == nil && !plan.DefaultAgentCreditLimit.IsNull() ||
		state != nil && !plan.DefaultAgentCreditLimit.Equal(state.DefaultAgentCreditLimit)
	if limitChanged {
		if _, err := admin.UpdateWorkspaceCreditLimit(ctx, ws, int64PtrFrom(plan.DefaultAgentCreditLimit)); err != nil {
			return fmt.Errorf("updating default agent credit limit: %w", err)
		}
	}
	return nil
}

func (r *AdminWorkspaceAgentSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AdminWorkspaceAgentSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan, nil); err != nil {
		resp.Diagnostics.AddError("Error applying workspace agent settings", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AdminWorkspaceAgentSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state AdminWorkspaceAgentSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error applying workspace agent settings", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read has nothing to fetch, because the API has no read endpoint for
// these settings. State keeps the last applied values.
func (r *AdminWorkspaceAgentSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AdminWorkspaceAgentSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Delete leaves both settings as they are.
func (r *AdminWorkspaceAgentSettingsResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {}

func (r *AdminWorkspaceAgentSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(req.ID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("workspace_id"), types.StringValue(req.ID))...)
}
