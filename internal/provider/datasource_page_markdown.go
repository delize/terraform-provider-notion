package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var _ datasource.DataSource = &PageMarkdownDataSource{}

type PageMarkdownDataSource struct {
	mdClient *markdownClient
}

type PageMarkdownDataSourceModel struct {
	PageID   types.String `tfsdk:"page_id"`
	Markdown types.String `tfsdk:"markdown"`
}

func NewPageMarkdownDataSource() datasource.DataSource {
	return &PageMarkdownDataSource{}
}

func (d *PageMarkdownDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_page_markdown"
}

func (d *PageMarkdownDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieve a Notion page's content as enhanced markdown.",
		Attributes: map[string]schema.Attribute{
			"page_id": schema.StringAttribute{
				Description: "The ID of the page to retrieve markdown for.",
				Required:    true,
			},
			"markdown": schema.StringAttribute{
				Description: "The page content rendered as enhanced markdown.",
				Computed:    true,
			},
		},
	}
}

func (d *PageMarkdownDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*notionapi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected DataSource Configure Type",
			fmt.Sprintf("Expected *notionapi.Client, got: %T.", req.ProviderData))
		return
	}
	d.mdClient = newMarkdownClient(client)
}

func (d *PageMarkdownDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PageMarkdownDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mdResp, err := d.mdClient.GetPageMarkdown(ctx, config.PageID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading page markdown", err.Error())
		return
	}

	config.Markdown = types.StringValue(mdResp.Markdown)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
