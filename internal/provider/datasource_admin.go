package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// Admin API data sources (reference added 2026-08-24). All need admin_token.
// Each has a single read call that pages through the whole list.

var (
	_ datasource.DataSource = &WorkspacesDataSource{}
	_ datasource.DataSource = &PermissionGroupsDataSource{}
	_ datasource.DataSource = &AdminUsersDataSource{}
)

// adminDataSourceBase holds the shared Configure.
type adminDataSourceBase struct {
	client *notionapi.Client
}

func (d *adminDataSourceBase) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := providerDataFrom(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected DataSource Configure Type", err.Error())
		return
	}
	d.client = client
}

func computedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{Description: desc, Computed: true}
}

// ---- notion_workspaces ----

type WorkspacesDataSource struct{ adminDataSourceBase }

type WorkspacesDataSourceModel struct {
	Workspaces []WorkspaceModel `tfsdk:"workspaces"`
}

type WorkspaceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func NewWorkspacesDataSource() datasource.DataSource { return &WorkspacesDataSource{} }

func (d *WorkspacesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workspaces"
}

func (d *WorkspacesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the live workspaces your Notion organization owns, through the Admin API. " +
			"Requires admin_token with the workspace:read scope. Use a workspace id as workspace_id on the other Admin API resources.",
		Attributes: map[string]schema.Attribute{
			"workspaces": schema.ListNestedAttribute{
				Description: "Workspaces ordered by ID.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   computedString("The workspace ID."),
						"name": computedString("The workspace name, or null when it has none."),
					},
				},
			},
		},
	}
}

func (d *WorkspacesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	admin, err := adminClientFor(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}
	spaces, err := admin.ListWorkspaces(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing workspaces", err.Error())
		return
	}
	state := WorkspacesDataSourceModel{Workspaces: []WorkspaceModel{}}
	for _, s := range spaces {
		state.Workspaces = append(state.Workspaces, WorkspaceModel{
			ID:   types.StringValue(s.ID),
			Name: types.StringPointerValue(s.Name),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- notion_permission_groups ----

type PermissionGroupsDataSource struct{ adminDataSourceBase }

type PermissionGroupsDataSourceModel struct {
	WorkspaceID types.String           `tfsdk:"workspace_id"`
	Groups      []PermissionGroupEntry `tfsdk:"groups"`
}

type PermissionGroupEntry struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func NewPermissionGroupsDataSource() datasource.DataSource { return &PermissionGroupsDataSource{} }

func (d *PermissionGroupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission_groups"
}

func (d *PermissionGroupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the permission groups in a workspace through the Admin API, both admin-managed and SCIM-managed. " +
			"Requires admin_token with the permission-group:read scope.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": schema.StringAttribute{
				Description: "The workspace to list groups in.",
				Required:    true,
			},
			"groups": schema.ListNestedAttribute{
				Description: "Permission groups in the workspace. Legacy groups with duplicate names can appear more than once.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   computedString("The group ID."),
						"name": computedString("The group name."),
					},
				},
			},
		},
	}
}

func (d *PermissionGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state PermissionGroupsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}
	groups, err := admin.ListGroups(ctx, state.WorkspaceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error listing permission groups", err.Error())
		return
	}
	state.Groups = []PermissionGroupEntry{}
	for _, g := range groups {
		state.Groups = append(state.Groups, PermissionGroupEntry{ID: types.StringValue(g.ID), Name: types.StringValue(g.Name)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- notion_admin_users ----

type AdminUsersDataSource struct{ adminDataSourceBase }

type AdminUsersDataSourceModel struct {
	WorkspaceID types.String     `tfsdk:"workspace_id"`
	Users       []AdminUserEntry `tfsdk:"users"`
}

type AdminUserEntry struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Type           types.String `tfsdk:"type"`
	MembershipType types.String `tfsdk:"membership_type"`
	Email          types.String `tfsdk:"email"`
}

func NewAdminUsersDataSource() datasource.DataSource { return &AdminUsersDataSource{} }

func (d *AdminUsersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_admin_users"
}

func (d *AdminUsersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists a workspace's active members (including restricted members) and guest bots through the Admin API. " +
			"Unlike notion_users, it covers the whole workspace, not only users the integration can see. " +
			"Page guests and expired temporary memberships are not included. Requires admin_token with the user:read scope.",
		Attributes: map[string]schema.Attribute{
			"workspace_id": schema.StringAttribute{
				Description: "The workspace to list users in.",
				Required:    true,
			},
			"users": schema.ListNestedAttribute{
				Description: "People and bots in the workspace.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   computedString("The user ID."),
						"name": computedString("The user's name, or null."),
						"type": computedString(`"person" or "bot".`),
						"membership_type": computedString("For people, the effective workspace membership: none, page_guest, restricted_member, member, membership_admin, or owner. " +
							"Null for bots."),
						"email": computedString("For people, the email address when Notion has one. Null for bots."),
					},
				},
			},
		},
	}
}

func (d *AdminUsersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state AdminUsersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}
	users, err := admin.ListUsers(ctx, state.WorkspaceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error listing workspace users", err.Error())
		return
	}
	state.Users = make([]AdminUserEntry, 0, len(users))
	for _, u := range users {
		state.Users = append(state.Users, adminUserEntry(u))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func adminUserEntry(u adminUser) AdminUserEntry {
	e := AdminUserEntry{
		ID:             types.StringValue(u.ID),
		Name:           types.StringPointerValue(u.Name),
		Type:           types.StringValue(u.Type),
		MembershipType: types.StringNull(),
		Email:          types.StringNull(),
	}
	if u.Type == "person" {
		if u.MembershipType != "" {
			e.MembershipType = types.StringValue(u.MembershipType)
		}
		if u.Person != nil && u.Person.Email != "" {
			e.Email = types.StringValue(u.Person.Email)
		}
	}
	return e
}
