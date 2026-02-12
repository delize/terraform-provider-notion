package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var _ provider.Provider = &NotionProvider{}

type NotionProvider struct {
	version string
}

type NotionProviderModel struct {
	Token types.String `tfsdk:"token"`
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

	if token == "" {
		resp.Diagnostics.AddError(
			"Missing Notion API Token",
			"The provider cannot create the Notion API client as there is a missing or empty value for the Notion API token. "+
				"Set the token value in the configuration or use the NOTION_TOKEN environment variable.",
		)
		return
	}

	client := notionapi.NewClient(notionapi.Token(token))

	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *NotionProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewPageResource,
		NewBlockResource,
		NewDatabaseResource,
		NewDatabaseEntryResource,
		NewDatabasePropertySelectResource,
		NewDatabasePropertyMultiSelectResource,
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
	}
}

func (p *NotionProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewDatabaseDataSource,
		NewPageDataSource,
		NewUserDataSource,
		NewDatabaseEntriesDataSource,
	}
}
