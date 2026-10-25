package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// MCP client connection endpoints of the Admin API (organization-level).
// Scopes: mcp-client-connection:read, and mcp-client-connection:write-high-impact
// for revoking a connection or changing enterprise-managed access.

type adminMCPConnection struct {
	Client struct {
		Key  string `json:"key"`
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"client"`
	User struct {
		ID string `json:"id"`
	} `json:"user"`
	Workspaces []struct {
		ID string `json:"id"`
	} `json:"workspaces"`
	IsEnterpriseManaged bool     `json:"is_enterprise_managed"`
	IdpIssuer           string   `json:"idp_issuer"`
	CreatedAt           *float64 `json:"created_at"`
	LastActiveAt        *float64 `json:"last_active_at"`
}

func (c *adminClient) ListMCPConnections(ctx context.Context, userIDs, workspaceIDs []string) ([]adminMCPConnection, error) {
	q := url.Values{}
	bracketValues(q, "user_ids", userIDs)
	bracketValues(q, "workspace_ids", workspaceIDs)
	return listInto[adminMCPConnection](ctx, c, adminListSpec{Path: "/v1/mcp_client_connections", Query: q, PageSize: 100})
}

// RevokeMCPConnection revokes one member's active tokens for one client in
// one workspace. They can connect again if workspace policy allows it.
func (c *adminClient) RevokeMCPConnection(ctx context.Context, clientKey, userID, workspaceID string) (int64, error) {
	var out struct {
		RevokedTokenCount float64 `json:"revoked_token_count"`
	}
	body := map[string]string{"client_key": clientKey, "user_id": userID, "workspace_id": workspaceID}
	err := c.do(ctx, http.MethodPost, "/v1/mcp_client_connections/revoke", nil, body, &out)
	return int64(out.RevokedTokenCount), err
}

// SetMCPEnterpriseAccess denies (and revokes current tokens) or restores a
// member's enterprise-managed MCP access in a workspace. Idempotent.
func (c *adminClient) SetMCPEnterpriseAccess(ctx context.Context, userID, workspaceID, access string) (int64, error) {
	var out struct {
		Access            string  `json:"access"`
		RevokedTokenCount float64 `json:"revoked_token_count"`
	}
	body := map[string]string{"access": access, "user_id": userID, "workspace_id": workspaceID}
	err := c.do(ctx, http.MethodPut, "/v1/mcp_client_connections/enterprise_managed_access", nil, body, &out)
	return int64(out.RevokedTokenCount), err
}

var mcpAccessValues = []string{"allowed", "denied"}

var (
	_ datasource.DataSource               = &MCPConnectionsDataSource{}
	_ action.ActionWithConfigure          = &MCPConnectionRevokeAction{}
	_ resource.Resource                   = &MCPEnterpriseAccessResource{}
	_ resource.ResourceWithImportState    = &MCPEnterpriseAccessResource{}
	_ resource.ResourceWithValidateConfig = &MCPEnterpriseAccessResource{}
)

// ---- notion_mcp_client_connections ----

type MCPConnectionsDataSource struct{ adminDataSourceBase }

type MCPConnectionsModel struct {
	UserIDs      []string             `tfsdk:"user_ids"`
	WorkspaceIDs []string             `tfsdk:"workspace_ids"`
	Connections  []MCPConnectionEntry `tfsdk:"connections"`
}

type MCPConnectionEntry struct {
	ClientKey           types.String `tfsdk:"client_key"`
	ClientName          types.String `tfsdk:"client_name"`
	ClientType          types.String `tfsdk:"client_type"`
	UserID              types.String `tfsdk:"user_id"`
	WorkspaceIDs        []string     `tfsdk:"workspace_ids"`
	IsEnterpriseManaged types.Bool   `tfsdk:"is_enterprise_managed"`
	IdpIssuer           types.String `tfsdk:"idp_issuer"`
	CreatedAt           types.Int64  `tfsdk:"created_at"`
	LastActiveAt        types.Int64  `tfsdk:"last_active_at"`
}

func NewMCPConnectionsDataSource() datasource.DataSource { return &MCPConnectionsDataSource{} }

func (d *MCPConnectionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mcp_client_connections"
}

func (d *MCPConnectionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		Description: "Lists members' OAuth connections from MCP clients (such as Claude Code or Codex) to Notion MCP, through the Admin API. " +
			"Workspace-owned internal connections aren't included. Requires admin_token with the mcp-client-connection:read scope.",
		Attributes: map[string]dsschema.Attribute{
			"user_ids":      dsschema.ListAttribute{Description: "Only these members.", Optional: true, ElementType: types.StringType},
			"workspace_ids": dsschema.ListAttribute{Description: "Only these workspaces. Also limits each result's workspace_ids.", Optional: true, ElementType: types.StringType},
			"connections": dsschema.ListNestedAttribute{
				Description: "Connections grouped by member, client, and connection settings.",
				Computed:    true,
				NestedObject: dsschema.NestedAttributeObject{Attributes: map[string]dsschema.Attribute{
					"client_key":            dsschema.StringAttribute{Description: "Stable client key, used by notion_mcp_client_connection_revoke.", Computed: true},
					"client_name":           dsschema.StringAttribute{Description: "Client name from Notion, or reported by a custom client.", Computed: true},
					"client_type":           dsschema.StringAttribute{Description: "Client category such as claude, cursor, or other.", Computed: true},
					"user_id":               dsschema.StringAttribute{Description: "The connected member.", Computed: true},
					"workspace_ids":         dsschema.ListAttribute{Description: "Workspaces where the member and client are connected.", Computed: true, ElementType: types.StringType},
					"is_enterprise_managed": dsschema.BoolAttribute{Description: "Whether the connections use enterprise-managed authentication.", Computed: true},
					"idp_issuer":            dsschema.StringAttribute{Description: "Identity provider issuer URL, for enterprise-managed connections only.", Computed: true},
					"created_at":            dsschema.Int64Attribute{Description: "Earliest connection, epoch milliseconds.", Computed: true},
					"last_active_at":        dsschema.Int64Attribute{Description: "Most recent use, epoch milliseconds, or null.", Computed: true},
				}},
			},
		},
	}
}

func (d *MCPConnectionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state MCPConnectionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}
	conns, err := admin.ListMCPConnections(ctx, state.UserIDs, state.WorkspaceIDs)
	if err != nil {
		resp.Diagnostics.AddError("Error listing MCP client connections", err.Error())
		return
	}
	state.Connections = make([]MCPConnectionEntry, 0, len(conns))
	for _, c := range conns {
		ws := make([]string, 0, len(c.Workspaces))
		for _, w := range c.Workspaces {
			ws = append(ws, w.ID)
		}
		state.Connections = append(state.Connections, MCPConnectionEntry{
			ClientKey: types.StringValue(c.Client.Key), ClientName: types.StringValue(c.Client.Name), ClientType: types.StringValue(c.Client.Type),
			UserID: types.StringValue(c.User.ID), WorkspaceIDs: ws, IsEnterpriseManaged: types.BoolValue(c.IsEnterpriseManaged),
			IdpIssuer: nullableString(c.IdpIssuer), CreatedAt: int64FromPtr(c.CreatedAt), LastActiveAt: int64FromPtr(c.LastActiveAt),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- action notion_mcp_client_connection_revoke ----

type MCPConnectionRevokeAction struct{ adminActionBase }

type MCPConnectionRevokeModel struct {
	ClientKey   types.String `tfsdk:"client_key"`
	UserID      types.String `tfsdk:"user_id"`
	WorkspaceID types.String `tfsdk:"workspace_id"`
}

func NewMCPConnectionRevokeAction() action.Action { return &MCPConnectionRevokeAction{} }

func (a *MCPConnectionRevokeAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mcp_client_connection_revoke"
}

func (a *MCPConnectionRevokeAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		Description: "Revokes one member's active OAuth tokens for one MCP client in one workspace, through the Admin API. " +
			"The member can connect again if workspace policy allows it. Requires admin_token with the mcp-client-connection:write-high-impact scope.",
		Attributes: map[string]actionschema.Attribute{
			"client_key":   actionschema.StringAttribute{Description: "The client key from notion_mcp_client_connections.", Required: true},
			"user_id":      actionschema.StringAttribute{Description: "The member.", Required: true},
			"workspace_id": actionschema.StringAttribute{Description: "The workspace.", Required: true},
		},
	}
}

func (a *MCPConnectionRevokeAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg MCPConnectionRevokeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin := a.admin(resp)
	if admin == nil {
		return
	}
	n, err := admin.RevokeMCPConnection(ctx, cfg.ClientKey.ValueString(), cfg.UserID.ValueString(), cfg.WorkspaceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error revoking MCP client connection", err.Error())
		return
	}
	progress(resp, "Revoked %d MCP client token(s) for user %s", n, cfg.UserID.ValueString())
}

// ---- notion_mcp_enterprise_access ----

// MCPEnterpriseAccessResource holds one member's enterprise-managed MCP
// access in one workspace at allowed or denied. The API can set it but not
// read it. Destroy leaves the access as it is unless restore_on_destroy is
// true, so removing a block never silently re-opens access.
type MCPEnterpriseAccessResource struct {
	client *notionapi.Client
}

type MCPEnterpriseAccessModel struct {
	ID               types.String `tfsdk:"id"`
	WorkspaceID      types.String `tfsdk:"workspace_id"`
	UserID           types.String `tfsdk:"user_id"`
	Access           types.String `tfsdk:"access"`
	RestoreOnDestroy types.Bool   `tfsdk:"restore_on_destroy"`
}

func NewMCPEnterpriseAccessResource() resource.Resource { return &MCPEnterpriseAccessResource{} }

func (r *MCPEnterpriseAccessResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mcp_enterprise_access"
}

func (r *MCPEnterpriseAccessResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Denies or restores one member's enterprise-managed MCP client access in a workspace, through the Admin API. " +
			"Denying blocks future connections and revokes current tokens. Only applies to enterprise-managed connections. " +
			"Requires admin_token with the mcp-client-connection:write-high-impact scope. The API can't read the setting back, so changes made in Notion aren't detected.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "<workspace_id>/<user_id>.", Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"workspace_id": schema.StringAttribute{Description: "The workspace.", Required: true, PlanModifiers: replace},
			"user_id":      schema.StringAttribute{Description: "The member.", Required: true, PlanModifiers: replace},
			"access":       schema.StringAttribute{Description: "denied or allowed. Changes in place.", Required: true},
			"restore_on_destroy": schema.BoolAttribute{
				Description: "Set access back to allowed when the resource is destroyed. Defaults to false, which leaves the access as it is.",
				Optional:    true, Computed: true, Default: booldefault.StaticBool(false),
			},
		},
	}
}

func (r *MCPEnterpriseAccessResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg MCPEnterpriseAccessModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Access.IsNull() || cfg.Access.IsUnknown() {
		return
	}
	if !slices.Contains(mcpAccessValues, cfg.Access.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("access"), "Invalid access", fmt.Sprintf("%q is not valid. Use denied or allowed.", cfg.Access.ValueString()))
	}
}

func (r *MCPEnterpriseAccessResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *MCPEnterpriseAccessResource) set(ctx context.Context, m *MCPEnterpriseAccessModel, access string) error {
	admin, err := adminClientFor(r.client)
	if err != nil {
		return err
	}
	_, err = admin.SetMCPEnterpriseAccess(ctx, m.UserID.ValueString(), m.WorkspaceID.ValueString(), access)
	return err
}

func (r *MCPEnterpriseAccessResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MCPEnterpriseAccessModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.set(ctx, &plan, plan.Access.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error setting MCP enterprise access", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.WorkspaceID.ValueString() + "/" + plan.UserID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MCPEnterpriseAccessResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state MCPEnterpriseAccessModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !plan.Access.Equal(state.Access) {
		if err := r.set(ctx, &plan, plan.Access.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error setting MCP enterprise access", err.Error())
			return
		}
	}
	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read has nothing to fetch, because the API can't read this setting.
func (r *MCPEnterpriseAccessResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MCPEnterpriseAccessModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.RestoreOnDestroy.IsNull() {
		state.RestoreOnDestroy = types.BoolValue(false)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *MCPEnterpriseAccessResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MCPEnterpriseAccessModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !state.RestoreOnDestroy.ValueBool() || state.Access.ValueString() == "allowed" {
		return
	}
	if err := r.set(ctx, &state, "allowed"); err != nil && !isAdminNotFound(err) {
		resp.Diagnostics.AddError("Error restoring MCP enterprise access", err.Error())
	}
}

func (r *MCPEnterpriseAccessResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	ws, user, err := parseCompositeID(req.ID)
	if err != nil || ws == "" || user == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("expected '<workspace_id>/<user_id>', got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(req.ID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("workspace_id"), types.StringValue(ws))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), types.StringValue(user))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("restore_on_destroy"), types.BoolValue(false))...)
}
