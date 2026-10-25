package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// notion_permission_group manages an admin-managed permission group through
// the Admin API (reference added 2026-08-24). Requires admin_token with the
// permission-group:read and permission-group:write scopes.

var (
	_ resource.Resource                   = &PermissionGroupResource{}
	_ resource.ResourceWithImportState    = &PermissionGroupResource{}
	_ resource.ResourceWithValidateConfig = &PermissionGroupResource{}
)

type PermissionGroupResource struct {
	client *notionapi.Client
}

type PermissionGroupModel struct {
	ID                types.String `tfsdk:"id"`
	WorkspaceID       types.String `tfsdk:"workspace_id"`
	Name              types.String `tfsdk:"name"`
	TransferToOwnerID types.String `tfsdk:"transfer_to_owner_id"`
}

func NewPermissionGroupResource() resource.Resource {
	return &PermissionGroupResource{}
}

func (r *PermissionGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission_group"
}

func (r *PermissionGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an admin-managed permission group in a Notion workspace through the Admin API. " +
			"Requires admin_token (Enterprise plan) with the permission-group:read and permission-group:write scopes. " +
			"SCIM-managed groups can be imported and read, but Notion refuses changes to them.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the group.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"workspace_id": schema.StringAttribute{
				Description: "The ID of the workspace the group belongs to. See the notion_workspaces data source. Changing this forces a new group.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the group, 1 to 200 characters. Must be unique among active groups (exact, case-sensitive match).",
				Required:    true,
			},
			"transfer_to_owner_id": schema.StringAttribute{
				Description: "An active workspace owner who receives access that deleting the group would otherwise orphan. " +
					"Only used on destroy. Notion requires it when the group holds access nobody else has, and refuses the delete without it.",
				Optional: true,
			},
		},
	}
}

func (r *PermissionGroupResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg PermissionGroupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Name.IsNull() || cfg.Name.IsUnknown() {
		return
	}
	name := cfg.Name.ValueString()
	if strings.TrimSpace(name) == "" || len([]rune(name)) > 200 {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid group name",
			"A permission group name must have 1 to 200 characters and at least one non-space character.")
	}
}

func (r *PermissionGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PermissionGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PermissionGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}

	g, err := admin.CreateGroup(ctx, plan.WorkspaceID.ValueString(), plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating permission group", err.Error())
		return
	}
	plan.ID = types.StringValue(g.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PermissionGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PermissionGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}

	g, err := admin.GetGroup(ctx, state.WorkspaceID.ValueString(), state.ID.ValueString())
	if isAdminNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading permission group", err.Error())
		return
	}
	state.Name = types.StringValue(g.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *PermissionGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state PermissionGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}

	plan.ID = state.ID
	// transfer_to_owner_id only matters on destroy, so a change to it alone
	// is saved to state without calling the API.
	if plan.Name.ValueString() != state.Name.ValueString() {
		if _, err := admin.RenameGroup(ctx, plan.WorkspaceID.ValueString(), plan.ID.ValueString(), plan.Name.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error renaming permission group", err.Error())
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PermissionGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PermissionGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}

	err = admin.DeleteGroup(ctx, state.WorkspaceID.ValueString(), state.ID.ValueString(), state.TransferToOwnerID.ValueString())
	if err == nil || isAdminNotFound(err) {
		return
	}
	msg := err.Error()
	if isAdminConflict(err) {
		msg += "\n\nThe group may hold access that would be orphaned, or it may be SCIM-managed. " +
			"For orphaned access, set transfer_to_owner_id to an active workspace owner, apply, then destroy again. " +
			"SCIM-managed groups have to be deleted in the identity provider."
	}
	resp.Diagnostics.AddError("Error deleting permission group", msg)
}

func (r *PermissionGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID",
			fmt.Sprintf("expected '<workspace_id>/<group_id>', got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("workspace_id"), types.StringValue(parts[0]))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(parts[1]))...)
}
