package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// RevokeUserSessions signs a managed user out everywhere. The user is
// identified by primary email or Notion user ID. Scope:
// managed-user-session:write.
func (c *adminClient) RevokeUserSessions(ctx context.Context, userID, email string) (string, error) {
	user := map[string]string{"type": "id", "id": userID}
	if email != "" {
		user = map[string]string{"type": "email", "email": email}
	}
	var out struct {
		Status string `json:"status"`
	}
	err := c.do(ctx, http.MethodPost, "/v1/managed_users/revoke_session", nil, map[string]interface{}{"user": user}, &out)
	return out.Status, err
}

var (
	_ action.ActionWithConfigure      = &RevokeUserSessionsAction{}
	_ action.ActionWithValidateConfig = &RevokeUserSessionsAction{}
)

type RevokeUserSessionsAction struct{ adminActionBase }

type RevokeUserSessionsModel struct {
	UserID types.String `tfsdk:"user_id"`
	Email  types.String `tfsdk:"email"`
}

func NewRevokeUserSessionsAction() action.Action { return &RevokeUserSessionsAction{} }

func (a *RevokeUserSessionsAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_revoke_user_sessions"
}

func (a *RevokeUserSessionsAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		Description: "Revokes all active sessions of a managed user, signing them out, through the Notion Admin API. " +
			"Requires admin_token with the managed-user-session:write scope. Set exactly one of user_id or email.",
		Attributes: map[string]actionschema.Attribute{
			"user_id": actionschema.StringAttribute{Description: "The managed user's Notion user ID.", Optional: true},
			"email":   actionschema.StringAttribute{Description: "The managed user's primary email address.", Optional: true},
		},
	}
}

func (a *RevokeUserSessionsAction) ValidateConfig(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var cfg RevokeUserSessionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.UserID.IsUnknown() || cfg.Email.IsUnknown() {
		return
	}
	if cfg.UserID.IsNull() == cfg.Email.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("user_id"), "Choose one user identifier", "Set exactly one of user_id or email.")
	}
}

func (a *RevokeUserSessionsAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg RevokeUserSessionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin := a.admin(resp)
	if admin == nil {
		return
	}
	who := cfg.UserID.ValueString()
	if who == "" {
		who = cfg.Email.ValueString()
	}
	status, err := admin.RevokeUserSessions(ctx, cfg.UserID.ValueString(), cfg.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error revoking user sessions", err.Error())
		return
	}
	progress(resp, "Sessions for %s: %s", who, status)
	// The response is only {"status":"revoked"}, so it can't show whether
	// anyone was signed out. Accounts outside the organization's verified
	// domains aren't managed users, and the Help Center says a logout call
	// for one does nothing.
	resp.Diagnostics.AddWarning("Check that the user is a managed user",
		fmt.Sprintf("Notion accepted the session revoke for %s, but it only signs out managed users (accounts on the organization's verified domains). "+
			"For any other account the call has no effect.", who))
}
