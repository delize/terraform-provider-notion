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
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Personal access token endpoints of the Admin API, workspace-scoped.
// Scopes: personal-access-token:read, and
// personal-access-token:write-high-impact to revoke. Token values are never
// returned.

type adminPATActor struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type adminPAT struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Status     string         `json:"status"`
	Creator    adminPATActor  `json:"creator"`
	CreatedAt  *float64       `json:"created_at"`
	ExpiresAt  *float64       `json:"expires_at"`
	LastUsedAt *float64       `json:"last_used_at"`
	RevokedAt  *float64       `json:"revoked_at"`
	RevokedBy  *adminPATActor `json:"revoked_by"`
}

var patStatuses = []string{"active", "expired", "revoked"}

func (c *adminClient) ListPATs(ctx context.Context, spaceID string, statuses, creatorIDs []string, search string) ([]adminPAT, error) {
	q := url.Values{}
	bracketValues(q, "status", statuses)
	bracketValues(q, "creator_ids", creatorIDs)
	if search != "" {
		q.Set("search", search)
	}
	return listInto[adminPAT](ctx, c, adminListSpec{Path: adminSpacePath(spaceID, "personal_access_tokens"), Query: q, PageSize: 100})
}

// RevokePAT permanently revokes a token. Repeating it is a no-op.
func (c *adminClient) RevokePAT(ctx context.Context, spaceID, botID string) error {
	return c.do(ctx, http.MethodDelete, adminSpacePath(spaceID, "personal_access_tokens", botID), nil, nil, nil)
}

var (
	_ datasource.DataSource      = &PATsDataSource{}
	_ action.ActionWithConfigure = &PATRevokeAction{}
)

// ---- notion_personal_access_tokens ----

type PATsDataSource struct{ adminDataSourceBase }

type PATsModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
	Statuses    []string     `tfsdk:"statuses"`
	CreatorIDs  []string     `tfsdk:"creator_ids"`
	Search      types.String `tfsdk:"search"`
	Tokens      []PATEntry   `tfsdk:"tokens"`
}

type PATEntry struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Status       types.String `tfsdk:"status"`
	CreatorID    types.String `tfsdk:"creator_id"`
	CreatorName  types.String `tfsdk:"creator_name"`
	CreatorEmail types.String `tfsdk:"creator_email"`
	CreatedAt    types.Int64  `tfsdk:"created_at"`
	ExpiresAt    types.Int64  `tfsdk:"expires_at"`
	LastUsedAt   types.Int64  `tfsdk:"last_used_at"`
	RevokedAt    types.Int64  `tfsdk:"revoked_at"`
	RevokedByID  types.String `tfsdk:"revoked_by_id"`
}

func NewPATsDataSource() datasource.DataSource { return &PATsDataSource{} }

func (d *PATsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_personal_access_tokens"
}

func (d *PATsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	str := func(desc string) dsschema.StringAttribute { return dsschema.StringAttribute{Description: desc, Computed: true} }
	num := func(desc string) dsschema.Int64Attribute { return dsschema.Int64Attribute{Description: desc, Computed: true} }
	resp.Schema = dsschema.Schema{
		Description: "Lists personal access token records in a workspace through the Admin API. Token values are never returned. " +
			"Requires admin_token with the personal-access-token:read scope.",
		Attributes: map[string]dsschema.Attribute{
			"workspace_id": dsschema.StringAttribute{Description: "The workspace.", Required: true},
			"statuses":     dsschema.ListAttribute{Description: "Only tokens with these statuses: active, expired, revoked.", Optional: true, ElementType: types.StringType},
			"creator_ids":  dsschema.ListAttribute{Description: "Only tokens created by these members.", Optional: true, ElementType: types.StringType},
			"search":       dsschema.StringAttribute{Description: "Case-insensitive match on the token name.", Optional: true},
			"tokens": dsschema.ListNestedAttribute{
				Description: "Token records.",
				Computed:    true,
				NestedObject: dsschema.NestedAttributeObject{Attributes: map[string]dsschema.Attribute{
					"id":            str("The token's ID. Pass it as bot_id to notion_personal_access_token_revoke."),
					"name":          str("Token name, or null."),
					"status":        str("active, expired, or revoked."),
					"creator_id":    str("Member who created the token."),
					"creator_name":  str("Creator's name, or null."),
					"creator_email": str("Creator's email, or null."),
					"created_at":    num("Creation, epoch milliseconds."),
					"expires_at":    num("Expiry, epoch milliseconds, or null."),
					"last_used_at":  num("Last authenticated request, epoch milliseconds, updated at most hourly, or null."),
					"revoked_at":    num("Revocation, epoch milliseconds, or null."),
					"revoked_by_id": str("User or bot that revoked the token, or null."),
				}},
			},
		},
	}
}

func (d *PATsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state PATsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, s := range state.Statuses {
		if !slices.Contains(patStatuses, s) {
			resp.Diagnostics.AddAttributeError(path.Root("statuses"), "Invalid status", fmt.Sprintf("%q is not valid. Use active, expired, or revoked.", s))
			return
		}
	}
	admin, err := adminClientFor(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}
	pats, err := admin.ListPATs(ctx, state.WorkspaceID.ValueString(), state.Statuses, state.CreatorIDs, state.Search.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error listing personal access tokens", err.Error())
		return
	}
	state.Tokens = make([]PATEntry, 0, len(pats))
	for _, p := range pats {
		e := PATEntry{
			ID: types.StringValue(p.ID), Name: nullableString(p.Name), Status: types.StringValue(p.Status),
			CreatorID: types.StringValue(p.Creator.ID), CreatorName: nullableString(p.Creator.Name), CreatorEmail: nullableString(p.Creator.Email),
			CreatedAt: int64FromPtr(p.CreatedAt), ExpiresAt: int64FromPtr(p.ExpiresAt), LastUsedAt: int64FromPtr(p.LastUsedAt),
			RevokedAt: int64FromPtr(p.RevokedAt), RevokedByID: types.StringNull(),
		}
		if p.RevokedBy != nil {
			e.RevokedByID = nullableString(p.RevokedBy.ID)
		}
		state.Tokens = append(state.Tokens, e)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- action notion_personal_access_token_revoke ----

type PATRevokeAction struct{ adminActionBase }

type PATRevokeModel struct {
	WorkspaceID types.String `tfsdk:"workspace_id"`
	BotID       types.String `tfsdk:"bot_id"`
}

func NewPATRevokeAction() action.Action { return &PATRevokeAction{} }

func (a *PATRevokeAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_personal_access_token_revoke"
}

func (a *PATRevokeAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		Description: "Permanently revokes a personal access token in a workspace through the Admin API. The token stops authenticating API requests and Notion Workers. " +
			"Requires admin_token with the personal-access-token:write-high-impact scope.",
		Attributes: map[string]actionschema.Attribute{
			"workspace_id": actionschema.StringAttribute{Description: "The workspace.", Required: true},
			"bot_id":       actionschema.StringAttribute{Description: "The token's id from notion_personal_access_tokens.", Required: true},
		},
	}
}

func (a *PATRevokeAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg PATRevokeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin := a.admin(resp)
	if admin == nil {
		return
	}
	if err := admin.RevokePAT(ctx, cfg.WorkspaceID.ValueString(), cfg.BotID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error revoking personal access token", err.Error())
		return
	}
	progress(resp, "Personal access token %s revoked", cfg.BotID.ValueString())
}
