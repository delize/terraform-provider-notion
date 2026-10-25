package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// notion_permission_group_member manages one direct user membership in a
// permission group through the Admin API. Nested-group memberships aren't
// exposed by the API, so they aren't modeled.

var (
	_ resource.Resource                   = &PermissionGroupMemberResource{}
	_ resource.ResourceWithImportState    = &PermissionGroupMemberResource{}
	_ resource.ResourceWithValidateConfig = &PermissionGroupMemberResource{}
)

type PermissionGroupMemberResource struct {
	client *notionapi.Client
}

type PermissionGroupMemberModel struct {
	ID          types.String `tfsdk:"id"`
	WorkspaceID types.String `tfsdk:"workspace_id"`
	GroupID     types.String `tfsdk:"group_id"`
	UserID      types.String `tfsdk:"user_id"`
	Role        types.String `tfsdk:"role"`
}

func NewPermissionGroupMemberResource() resource.Resource {
	return &PermissionGroupMemberResource{}
}

func (r *PermissionGroupMemberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission_group_member"
}

func (r *PermissionGroupMemberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Manages a direct user membership in an admin-managed Notion permission group through the Admin API. " +
			"Requires admin_token (Enterprise plan) with the permission-group:read and permission-group:write scopes.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Composite ID in the form <workspace_id>/<group_id>/<user_id>.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"workspace_id": schema.StringAttribute{
				Description: "The ID of the workspace. Changing this forces a new membership.",
				Required:    true, PlanModifiers: replace,
			},
			"group_id": schema.StringAttribute{
				Description: "The ID of the permission group. Changing this forces a new membership.",
				Required:    true, PlanModifiers: replace,
			},
			"user_id": schema.StringAttribute{
				Description: "The ID of the user. Must be a workspace member, guests aren't supported. Changing this forces a new membership.",
				Required:    true, PlanModifiers: replace,
			},
			"role": schema.StringAttribute{
				Description: "The user's role in the group: owner or member. Restricted members can't be owners. Changes in place.",
				Required:    true,
			},
		},
	}
}

func (r *PermissionGroupMemberResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg PermissionGroupMemberModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Role.IsNull() || cfg.Role.IsUnknown() {
		return
	}
	if !slices.Contains(groupMemberRoles, cfg.Role.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("role"), "Invalid group role",
			fmt.Sprintf("%q is not a group role. Use one of: %s.", cfg.Role.ValueString(), strings.Join(groupMemberRoles, ", ")))
	}
}

func (r *PermissionGroupMemberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func memberCompositeID(m PermissionGroupMemberModel) types.String {
	return types.StringValue(strings.Join([]string{m.WorkspaceID.ValueString(), m.GroupID.ValueString(), m.UserID.ValueString()}, "/"))
}

func (r *PermissionGroupMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PermissionGroupMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}

	_, err = admin.AddGroupMember(ctx, plan.WorkspaceID.ValueString(), plan.GroupID.ValueString(), plan.UserID.ValueString(), plan.Role.ValueString())
	if err != nil {
		msg := err.Error()
		if isAdminConflict(err) {
			msg += "\n\nThe group may be SCIM-managed (change memberships in the identity provider), or the user may already be a member. " +
				"An existing membership can be imported with '<workspace_id>/<group_id>/<user_id>'."
		}
		resp.Diagnostics.AddError("Error adding permission group member", msg)
		return
	}
	plan.ID = memberCompositeID(plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PermissionGroupMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PermissionGroupMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}

	m, err := admin.FindGroupMember(ctx, state.WorkspaceID.ValueString(), state.GroupID.ValueString(), state.UserID.ValueString())
	if isAdminNotFound(err) {
		// The group itself is gone, so the membership is too.
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading permission group member", err.Error())
		return
	}
	if m == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	// user_id keeps the form from config. Notion accepts IDs with or
	// without hyphens and FindGroupMember compares them normalized.
	state.Role = types.StringValue(m.Role)
	state.ID = memberCompositeID(state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *PermissionGroupMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PermissionGroupMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}

	_, err = admin.UpdateGroupMember(ctx, plan.WorkspaceID.ValueString(), plan.GroupID.ValueString(), plan.UserID.ValueString(), plan.Role.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating permission group member", err.Error())
		return
	}
	plan.ID = memberCompositeID(plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PermissionGroupMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PermissionGroupMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}

	err = admin.RemoveGroupMember(ctx, state.WorkspaceID.ValueString(), state.GroupID.ValueString(), state.UserID.ValueString())
	if err != nil && !isAdminNotFound(err) {
		resp.Diagnostics.AddError("Error removing permission group member", err.Error())
	}
}

func (r *PermissionGroupMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError("Invalid import ID",
			fmt.Sprintf("expected '<workspace_id>/<group_id>/<user_id>', got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(req.ID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("workspace_id"), types.StringValue(parts[0]))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), types.StringValue(parts[1]))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), types.StringValue(parts[2]))...)
}
