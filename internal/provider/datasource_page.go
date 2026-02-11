package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var _ datasource.DataSource = &PageDataSource{}

type PageDataSource struct {
	client *notionapi.Client
}

type PageDataSourceModel struct {
	Query        types.String `tfsdk:"query"`
	ID           types.String `tfsdk:"id"`
	ParentPageID types.String `tfsdk:"parent_page_id"`
	Title        types.String `tfsdk:"title"`
	URL          types.String `tfsdk:"url"`
}

func NewPageDataSource() datasource.DataSource {
	return &PageDataSource{}
}

func (d *PageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_page"
}

func (d *PageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Search for a Notion page by title.",
		Attributes: map[string]schema.Attribute{
			"query": schema.StringAttribute{
				Description: "Search query to find the page by title.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The ID of the page.",
				Computed:    true,
			},
			"parent_page_id": schema.StringAttribute{
				Description: "The ID of the parent page, if applicable.",
				Computed:    true,
			},
			"title": schema.StringAttribute{
				Description: "The title of the page.",
				Computed:    true,
			},
			"url": schema.StringAttribute{
				Description: "The URL of the page.",
				Computed:    true,
			},
		},
	}
}

func (d *PageDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*notionapi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected DataSource Configure Type",
			fmt.Sprintf("Expected *notionapi.Client, got: %T.", req.ProviderData))
		return
	}
	d.client = client
}

func (d *PageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PageDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.Search.Do(ctx, &notionapi.SearchRequest{
		Query: config.Query.ValueString(),
		Filter: notionapi.SearchFilter{
			Value:    "page",
			Property: "object",
		},
		PageSize: 1,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error searching for page", err.Error())
		return
	}

	if len(result.Results) == 0 {
		resp.Diagnostics.AddError("Page not found",
			fmt.Sprintf("No page found matching query: %s", config.Query.ValueString()))
		return
	}

	obj := result.Results[0]
	page, ok := obj.(*notionapi.Page)
	if !ok {
		resp.Diagnostics.AddError("Unexpected search result type", "Expected a page object from search results.")
		return
	}

	config.ID = types.StringValue(normalizeID(string(page.ID)))
	config.URL = types.StringValue(page.URL)

	if page.Parent.Type == notionapi.ParentTypePageID {
		config.ParentPageID = types.StringValue(normalizeID(string(page.Parent.PageID)))
	} else {
		config.ParentPageID = types.StringValue("")
	}

	for _, prop := range page.Properties {
		if tp, ok := prop.(*notionapi.TitleProperty); ok {
			config.Title = types.StringValue(richTextToPlain(tp.Title))
			break
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
