package provider

import (
	"context"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var (
	_ provider.Provider            = &NotionProvider{}
	_ provider.ProviderWithActions = &NotionProvider{}
)

type NotionProvider struct {
	version string
}

type NotionProviderModel struct {
	Token               types.String `tfsdk:"token"`
	SilentNotifications types.Bool   `tfsdk:"silent_notifications"`
	AdminToken          types.String `tfsdk:"admin_token"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &NotionProvider{
			version: version,
		}
	}
}

func (p *NotionProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "notion"
	resp.Version = p.version
}

func (p *NotionProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Interact with Notion.",
		Attributes: map[string]schema.Attribute{
			"token": schema.StringAttribute{
				Description: "Notion API token. Can also be set via the NOTION_TOKEN environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"silent_notifications": schema.BoolAttribute{
				Description: "Create and update pages without sending Notion notifications. Skips page update, @mention, and Person property notifications. " +
					"Database automations, reminders, and webhooks still run. Defaults to false.",
				Optional: true,
			},
			"admin_token": schema.StringAttribute{
				Description: "Notion Admin API organization bot token, used only by the Admin API resources and data sources " +
					"(permission groups, organization users, workspaces). Requires an Enterprise plan. " +
					"Can also be set via the NOTION_ADMIN_TOKEN environment variable.",
				Optional:  true,
				Sensitive: true,
			},
		},
	}
}

func (p *NotionProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config NotionProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token := os.Getenv("NOTION_TOKEN")
	if !config.Token.IsNull() {
		token = config.Token.ValueString()
	}

	adminToken := os.Getenv("NOTION_ADMIN_TOKEN")
	if !config.AdminToken.IsNull() {
		adminToken = config.AdminToken.ValueString()
	}

	if token == "" && adminToken == "" {
		resp.Diagnostics.AddError(
			"Missing Notion API Token",
			"The provider cannot create the Notion API client as there is a missing or empty value for the Notion API token. "+
				"Set the token value in the configuration or use the NOTION_TOKEN environment variable. "+
				"For the Admin API resources only, admin_token or NOTION_ADMIN_TOKEN is enough.",
		)
		return
	}

	// Wire the SDK with a retry-capable http.Client so transient 5xx /
	// HTML-from-edge responses don't bubble up as the cryptic
	// "invalid character '<' looking for beginning of value" decode
	// error. See retry_transport.go for the full policy.
	//
	// silent_notifications adds notifications.mode=silent to page create
	// and update bodies (Notion API 2026-10-08). See notion_notifications.go.
	httpClient := newProviderHTTPClient(config.SilentNotifications.ValueBool())
	if token == "" {
		// Admin-only provider block. Regular resources still receive a
		// client, but every call fails with an error that names the
		// missing token. See missingTokenTransport.
		httpClient = &http.Client{Transport: missingTokenTransport{}}
	}
	client := notionapi.NewClient(
		notionapi.Token(token),
		notionapi.WithHTTPClient(httpClient),
	)
	registerClientToken(client, token)
	registerHTTPClient(token, httpClient)
	registerAdminToken(client, adminToken)

	resp.ResourceData = client
	resp.DataSourceData = client
	resp.ActionData = client
}

// Actions are one-shot Admin API operations. They need Terraform 1.14+.
func (p *NotionProvider) Actions(_ context.Context) []func() action.Action {
	return []func() action.Action{
		NewLegalHoldExportAction,
		NewRevokeUserSessionsAction,
	}
}

func (p *NotionProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewPageResource,
		NewBlockResource,
		NewDatabaseResource,
		NewDatabaseEntryResource,
		NewDatabasePropertySelectResource,
		NewDatabasePropertyMultiSelectResource,
		NewDatabasePropertyStatusResource,
		NewDatabasePropertyNumberResource,
		NewDatabasePropertyRelationResource,
		NewDatabasePropertyRollupResource,
		newDatabasePropertyBasicResource("rich_text", notionapi.PropertyConfigTypeRichText),
		newDatabasePropertyBasicResource("date", notionapi.PropertyConfigTypeDate),
		newDatabasePropertyBasicResource("people", notionapi.PropertyConfigTypePeople),
		newDatabasePropertyBasicResource("checkbox", notionapi.PropertyConfigTypeCheckbox),
		newDatabasePropertyBasicResource("url", notionapi.PropertyConfigTypeURL),
		newDatabasePropertyBasicResource("email", notionapi.PropertyConfigTypeEmail),
		newDatabasePropertyBasicResource("created_time", notionapi.PropertyConfigCreatedTime),
		newDatabasePropertyBasicResource("created_by", notionapi.PropertyConfigCreatedBy),
		newDatabasePropertyBasicResource("last_edited_time", notionapi.PropertyConfigLastEditedTime),
		newDatabasePropertyBasicResource("last_edited_by", notionapi.PropertyConfigLastEditedBy),
		NewViewResource,
		NewPermissionGroupResource,
		NewPermissionGroupMemberResource,
		NewLegalHoldResource,
	}
}

func (p *NotionProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewDatabaseDataSource,
		NewPageDataSource,
		NewPageMarkdownDataSource,
		NewUserDataSource,
		NewUsersDataSource,
		NewDatabaseEntriesDataSource,
		NewSearchDataSource,
		NewBlocksDataSource,
		NewMeetingNotesDataSource,
		NewViewQueryDataSource,
		NewPermissionGroupsDataSource,
		NewAdminUsersDataSource,
		NewWorkspacesDataSource,
		NewLegalHoldsDataSource,
		NewLegalHoldPagesDataSource,
	}
}
