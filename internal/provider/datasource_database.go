package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var _ datasource.DataSource = &DatabaseDataSource{}

type DatabaseDataSource struct {
	client *notionapi.Client
}

type DatabaseDataSourceModel struct {
	Query types.String `tfsdk:"query"`
	ID    types.String `tfsdk:"id"`
	Title types.String `tfsdk:"title"`
	URL   types.String `tfsdk:"url"`
}

func NewDatabaseDataSource() datasource.DataSource {
	return &DatabaseDataSource{}
}

func (d *DatabaseDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (d *DatabaseDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Search for a Notion database by title.",
		Attributes: map[string]schema.Attribute{
			"query": schema.StringAttribute{
				Description: "Search query to find the database by title.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The ID of the database.",
				Computed:    true,
			},
			"title": schema.StringAttribute{
				Description: "The title of the database.",
				Computed:    true,
			},
			"url": schema.StringAttribute{
				Description: "The URL of the database.",
				Computed:    true,
			},
		},
	}
}

func (d *DatabaseDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *DatabaseDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DatabaseDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.Search.Do(ctx, &notionapi.SearchRequest{
		Query: config.Query.ValueString(),
		Filter: notionapi.SearchFilter{
			Value:    "database",
			Property: "object",
		},
		PageSize: 1,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error searching for database", err.Error())
		return
	}

	if len(result.Results) == 0 {
		resp.Diagnostics.AddError("Database not found",
			fmt.Sprintf("No database found matching query: %s", config.Query.ValueString()))
		return
	}

	obj := result.Results[0]
	db, ok := obj.(*notionapi.Database)
	if !ok {
		resp.Diagnostics.AddError("Unexpected search result type", "Expected a database object from search results.")
		return
	}

	config.ID = types.StringValue(normalizeID(string(db.ID)))
	config.Title = types.StringValue(richTextToPlain(db.Title))
	config.URL = types.StringValue(db.URL)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
