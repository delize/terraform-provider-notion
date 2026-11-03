package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var _ datasource.DataSource = &PageMarkdownDataSource{}

type PageMarkdownDataSource struct {
	mdClient *markdownClient
}

type PageMarkdownDataSourceModel struct {
	PageID            types.String `tfsdk:"page_id"`
	IncludeTranscript types.Bool   `tfsdk:"include_transcript"`
	MarkdownVersion   types.String `tfsdk:"markdown_version"`
	Markdown          types.String `tfsdk:"markdown"`
}

func NewPageMarkdownDataSource() datasource.DataSource {
	return &PageMarkdownDataSource{}
}

func (d *PageMarkdownDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_page_markdown"
}

func (d *PageMarkdownDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieve a Notion page's content as markdown.",
		Attributes: map[string]schema.Attribute{
			"page_id": schema.StringAttribute{
				Description: "The ID of the page to retrieve markdown for.",
				Required:    true,
			},
			"include_transcript": schema.BoolAttribute{
				Description: "Include full AI meeting note transcripts in the markdown. Without it, each meeting note renders as a placeholder with its URL. Defaults to false.",
				Optional:    true,
			},
			"markdown_version": schema.StringAttribute{
				Description: "Markdown syntax to read the page in: `v2` (Notion-flavored Markdown) or `v1` (enhanced markdown, deprecated by Notion). " +
					"Overrides the provider's markdown_version. When neither is set, Notion returns v1.",
				Optional: true,
				Validators: []validator.String{
					MarkdownVersionValidator(),
				},
			},
			"markdown": schema.StringAttribute{
				Description: "The page content rendered as markdown in the requested markdown_version.",
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

	mdResp, err := d.mdClient.GetPageMarkdownWithTranscript(ctx, config.PageID.ValueString(), config.IncludeTranscript.ValueBool(),
		d.mdClient.resolveMarkdownVersion(config.MarkdownVersion))
	if err != nil {
		resp.Diagnostics.AddError("Error reading page markdown", err.Error())
		return
	}

	config.Markdown = types.StringValue(mdResp.Markdown)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
