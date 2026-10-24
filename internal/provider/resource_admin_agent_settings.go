package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// notion_admin_agent_settings governs an existing custom agent: its admin
// status, admin credit limit, and sharing permissions. It never creates or
// deletes the agent. Destroy only removes it from state, leaving the agent
// and its settings as they are. Deleting an agent is the separate
// notion_admin_agent_delete action.

var (
	_ resource.Resource                   = &AdminAgentSettingsResource{}
	_ resource.ResourceWithImportState    = &AdminAgentSettingsResource{}
	_ resource.ResourceWithValidateConfig = &AdminAgentSettingsResource{}
)

type AdminAgentSettingsResource struct {
	client *notionapi.Client
}

type AdminAgentSettingsModel struct {
	ID                  types.String `tfsdk:"id"`
	WorkspaceID         types.String `tfsdk:"workspace_id"`
	AgentID             types.String `tfsdk:"agent_id"`
	AdminStatus         types.String `tfsdk:"admin_status"`
	CreditLimit         types.Int64  `tfsdk:"credit_limit"`
	Permissions         types.Set    `tfsdk:"permissions"`
	Name                types.String `tfsdk:"name"`
	Type                types.String `tfsdk:"type"`
	Status              types.String `tfsdk:"status"`
	EnforcedCreditLimit types.Int64  `tfsdk:"enforced_credit_limit"`
}

type AgentPermissionModel struct {
	PrincipalType types.String `tfsdk:"principal_type"`
	PrincipalID   types.String `tfsdk:"principal_id"`
	Role          types.String `tfsdk:"role"`
}

var agentPermissionAttrTypes = map[string]attr.Type{
	"principal_type": types.StringType,
	"principal_id":   types.StringType,
	"role":           types.StringType,
}

func NewAdminAgentSettingsResource() resource.Resource {
	return &AdminAgentSettingsResource{}
}

func (r *AdminAgentSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_admin_agent_settings"
}

func (r *AdminAgentSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Governs an existing Notion custom agent through the Admin API: admin status, admin credit limit, and sharing permissions. " +
			"It never creates or deletes the agent, and destroying it leaves the agent and its settings as they are. " +
			"Requires admin_token with the workflows:read and workflows:write scopes. Unset attributes are left alone.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Composite ID <workspace_id>/<agent_id>.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"workspace_id": schema.StringAttribute{Description: "The workspace the agent belongs to.", Required: true, PlanModifiers: replace},
			"agent_id": schema.StringAttribute{
				Description: "The custom agent's ID. The personal agent (Notion AI) isn't supported.", Required: true, PlanModifiers: replace,
			},
			"admin_status": schema.StringAttribute{
				Description: "active or disabled. Disabling pauses the agent's triggers. Notion rejects either change while the agent is paused for a credit or operational limit.",
				Optional:    true,
			},
			"credit_limit": schema.Int64Attribute{
				Description: "The workspace admin's per-agent credit limit, in whole credits. Removing it from the configuration clears the limit. " +
					"The API can't read this value back, so changes made in Notion aren't detected.",
				Optional: true,
			},
			"permissions": schema.SetNestedAttribute{
				Description: "Who the agent is shared with. When set, the list is authoritative: grants not listed here are removed, including comment-only grants made in Notion. " +
					"Leave it unset to not manage sharing.",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"principal_type": schema.StringAttribute{Description: "user, group, or workspace (every workspace member, never guests).", Required: true},
						"principal_id":   schema.StringAttribute{Description: "The user or group ID. Leave unset for workspace.", Optional: true},
						"role":           schema.StringAttribute{Description: "edit, full_access, or view_and_interact.", Required: true},
					},
				},
			},
			"name":   schema.StringAttribute{Description: "The agent's name.", Computed: true},
			"type":   schema.StringAttribute{Description: "custom or database_autofill.", Computed: true},
			"status": schema.StringAttribute{Description: "The agent's current status: active, deleted, or a paused reason such as credit_limit.", Computed: true},
			"enforced_credit_limit": schema.Int64Attribute{
				Description: "The limit that actually applies: the lower of the admin and creator limits, or the workspace default. Null when none applies.",
				Computed:    true,
			},
		},
	}
}

func (r *AdminAgentSettingsResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg AdminAgentSettingsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if v := cfg.AdminStatus; !v.IsNull() && !v.IsUnknown() && !slices.Contains(agentAdminStatuses, v.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("admin_status"), "Invalid admin status",
			fmt.Sprintf("%q is not valid. Use active or disabled.", v.ValueString()))
	}
	if v := cfg.CreditLimit; !v.IsNull() && !v.IsUnknown() && v.ValueInt64() < 0 {
		resp.Diagnostics.AddAttributeError(path.Root("credit_limit"), "Invalid credit limit", "credit_limit must be zero or more whole credits.")
	}
	validateAgentPermissions(ctx, cfg.Permissions, &resp.Diagnostics)
}

func validateAgentPermissions(ctx context.Context, set types.Set, diags *diag.Diagnostics) {
	if set.IsNull() || set.IsUnknown() {
		return
	}
	var perms []AgentPermissionModel
	diags.Append(set.ElementsAs(ctx, &perms, false)...)
	seen := map[string]bool{}
	for _, p := range perms {
		if p.PrincipalType.IsUnknown() || p.PrincipalID.IsUnknown() || p.Role.IsUnknown() {
			continue
		}
		typ := p.PrincipalType.ValueString()
		at := path.Root("permissions")
		if !slices.Contains(agentPrincipalTypes, typ) {
			diags.AddAttributeError(at, "Invalid principal type", fmt.Sprintf("%q is not valid. Use user, group, or workspace.", typ))
			continue
		}
		if !slices.Contains(agentGrantRoles, p.Role.ValueString()) {
			diags.AddAttributeError(at, "Invalid role",
				fmt.Sprintf("%q is not a role the API can grant. Use %s.", p.Role.ValueString(), strings.Join(agentGrantRoles, ", ")))
		}
		if typ == "workspace" && !p.PrincipalID.IsNull() {
			diags.AddAttributeError(at, "Unexpected principal_id", "A workspace grant has no principal_id.")
		}
		if typ != "workspace" && p.PrincipalID.IsNull() {
			diags.AddAttributeError(at, "Missing principal_id", fmt.Sprintf("A %s grant needs principal_id.", typ))
		}
		k := newAgentPrincipal(typ, p.PrincipalID.ValueString()).key()
		if seen[k] {
			diags.AddAttributeError(at, "Duplicate principal", "Each principal can appear once in permissions.")
		}
		seen[k] = true
	}
}

func (r *AdminAgentSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AdminAgentSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AdminAgentSettingsModel
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

func (r *AdminAgentSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state AdminAgentSettingsModel
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

// apply pushes the planned settings. state is nil on create. The credit
// limit goes first, because raising it can lift a credit pause that would
// otherwise make Notion reject a status change.
func (r *AdminAgentSettingsResource) apply(ctx context.Context, plan, state *AdminAgentSettingsModel, diags *diag.Diagnostics) {
	admin, err := adminClientFor(r.client)
	if err != nil {
		diags.AddError("Admin API token required", err.Error())
		return
	}
	ws, agent := plan.WorkspaceID.ValueString(), plan.AgentID.ValueString()
	plan.ID = types.StringValue(ws + "/" + agent)

	limitChanged := state == nil && !plan.CreditLimit.IsNull() || state != nil && !plan.CreditLimit.Equal(state.CreditLimit)
	if limitChanged {
		if _, err := admin.UpdateAgentCreditLimit(ctx, ws, agent, int64PtrFrom(plan.CreditLimit)); err != nil {
			diags.AddError("Error updating agent credit limit", err.Error())
			return
		}
	}

	statusChanged := !plan.AdminStatus.IsNull() && (state == nil || !plan.AdminStatus.Equal(state.AdminStatus))
	if statusChanged {
		if _, err := admin.UpdateAgentStatus(ctx, ws, agent, plan.AdminStatus.ValueString()); err != nil {
			diags.AddError("Error updating agent status", err.Error()+
				"\n\nNotion rejects status changes while an agent is paused for a credit or operational limit. Resolve that first.")
			return
		}
	}

	if !plan.Permissions.IsNull() {
		if !r.syncPermissions(ctx, admin, ws, agent, plan.Permissions, diags) {
			return
		}
	}

	if found := r.refresh(ctx, admin, plan, diags); !found && !diags.HasError() {
		diags.AddError("Agent not found", fmt.Sprintf("Agent %s was not found in workspace %s, or it has been deleted.", agent, ws))
	}
}

// syncPermissions makes the agent's grants match want exactly.
func (r *AdminAgentSettingsResource) syncPermissions(ctx context.Context, admin *adminClient, ws, agent string, want types.Set, diags *diag.Diagnostics) bool {
	var desired []AgentPermissionModel
	diags.Append(want.ElementsAs(ctx, &desired, false)...)
	current, err := admin.GetAgentPermissions(ctx, ws, agent)
	if err != nil {
		diags.AddError("Error reading agent permissions", err.Error())
		return false
	}
	have := map[string]adminAgentPermission{}
	for _, p := range current {
		have[p.Principal.key()] = p
	}
	var set []agentPermissionSet
	wantKeys := map[string]bool{}
	for _, d := range desired {
		pr := newAgentPrincipal(d.PrincipalType.ValueString(), d.PrincipalID.ValueString())
		wantKeys[pr.key()] = true
		if h, ok := have[pr.key()]; !ok || h.Role != d.Role.ValueString() {
			set = append(set, agentPermissionSet{Principal: pr, Role: d.Role.ValueString()})
		}
	}
	var remove []adminAgentPrincipal
	for k, h := range have {
		if !wantKeys[k] {
			remove = append(remove, h.Principal)
		}
	}
	if len(set) == 0 && len(remove) == 0 {
		return true
	}
	if err := admin.UpdateAgentPermissions(ctx, ws, agent, set, remove); err != nil {
		diags.AddError("Error updating agent permissions", err.Error())
		return false
	}
	return true
}

// refresh reads computed fields, plus admin_status and permissions when the
// configuration manages them. It returns false when the agent is gone.
func (r *AdminAgentSettingsResource) refresh(ctx context.Context, admin *adminClient, m *AdminAgentSettingsModel, diags *diag.Diagnostics) bool {
	ws, agent := m.WorkspaceID.ValueString(), m.AgentID.ValueString()
	usage, err := admin.GetAgentCreditUsage(ctx, ws, agent)
	if isAdminNotFound(err) {
		return false
	}
	if err != nil {
		diags.AddError("Error reading agent", err.Error())
		return true
	}
	if usage.Status == "deleted" {
		return false
	}
	m.Name = types.StringValue(usage.Name)
	m.Type = types.StringValue(usage.Type)
	m.Status = types.StringValue(usage.Status)
	m.EnforcedCreditLimit = int64FromPtr(usage.CreditLimit)

	if !m.AdminStatus.IsNull() {
		// Disabling through the API sets disabled_from_workspace_settings.
		// Any other status means the admin toggle is on, even when the
		// agent is paused for another reason.
		if usage.Status == "disabled_from_workspace_settings" {
			m.AdminStatus = types.StringValue("disabled")
		} else {
			m.AdminStatus = types.StringValue("active")
		}
	}

	if !m.Permissions.IsNull() {
		current, err := admin.GetAgentPermissions(ctx, ws, agent)
		if err != nil {
			diags.AddError("Error reading agent permissions", err.Error())
			return true
		}
		var prior []AgentPermissionModel
		diags.Append(m.Permissions.ElementsAs(ctx, &prior, false)...)
		priorIDs := map[string]string{}
		for _, p := range prior {
			pr := newAgentPrincipal(p.PrincipalType.ValueString(), p.PrincipalID.ValueString())
			priorIDs[pr.key()] = p.PrincipalID.ValueString()
		}
		elems := make([]attr.Value, 0, len(current))
		for _, c := range current {
			id := types.StringNull()
			if c.Principal.Type != "workspace" {
				id = types.StringValue(c.Principal.id())
				if p, ok := priorIDs[c.Principal.key()]; ok {
					id = types.StringValue(p) // keep the configured form
				}
			}
			obj, d := types.ObjectValue(agentPermissionAttrTypes, map[string]attr.Value{
				"principal_type": types.StringValue(c.Principal.Type),
				"principal_id":   id,
				"role":           types.StringValue(c.Role),
			})
			diags.Append(d...)
			elems = append(elems, obj)
		}
		set, d := types.SetValue(types.ObjectType{AttrTypes: agentPermissionAttrTypes}, elems)
		diags.Append(d...)
		m.Permissions = set
	}
	return true
}

func (r *AdminAgentSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AdminAgentSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}
	if !r.refresh(ctx, admin, &state, &resp.Diagnostics) {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Delete leaves the agent and its settings untouched.
func (r *AdminAgentSettingsResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {}

func (r *AdminAgentSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("expected '<workspace_id>/<agent_id>', got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(req.ID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("workspace_id"), types.StringValue(parts[0]))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("agent_id"), types.StringValue(parts[1]))...)
}
