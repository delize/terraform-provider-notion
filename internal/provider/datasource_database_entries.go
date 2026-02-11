package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var _ datasource.DataSource = &DatabaseEntriesDataSource{}

type DatabaseEntriesDataSource struct {
	client *notionapi.Client
}

type DatabaseEntriesDataSourceModel struct {
	Database types.String             `tfsdk:"database"`
	Entries  []DatabaseEntryDataModel `tfsdk:"entries"`
}

type DatabaseEntryDataModel struct {
	ID    types.String `tfsdk:"id"`
	Title types.String `tfsdk:"title"`
	URL   types.String `tfsdk:"url"`
}

func NewDatabaseEntriesDataSource() datasource.DataSource {
	return &DatabaseEntriesDataSource{}
}

func (d *DatabaseEntriesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_entries"
}

func (d *DatabaseEntriesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Query all entries in a Notion database.",
		Attributes: map[string]schema.Attribute{
			"database": schema.StringAttribute{
				Description: "The ID of the database to query.",
				Required:    true,
			},
			"entries": schema.ListNestedAttribute{
				Description: "List of database entries.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The ID of the entry.",
							Computed:    true,
						},
						"title": schema.StringAttribute{
							Description: "The title of the entry.",
							Computed:    true,
						},
						"url": schema.StringAttribute{
							Description: "The URL of the entry.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *DatabaseEntriesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *DatabaseEntriesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DatabaseEntriesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var entries []DatabaseEntryDataModel
	var cursor notionapi.Cursor

	for {
		result, err := d.client.Database.Query(ctx, notionapi.DatabaseID(config.Database.ValueString()), &notionapi.DatabaseQueryRequest{
			StartCursor: cursor,
			PageSize:    100,
		})
		if err != nil {
			resp.Diagnostics.AddError("Error querying database", err.Error())
			return
		}

		for i := range result.Results {
			page := &result.Results[i]
			entry := DatabaseEntryDataModel{
				ID:  types.StringValue(normalizeID(string(page.ID))),
				URL: types.StringValue(page.URL),
			}

			// Extract title
			for _, prop := range page.Properties {
				if tp, ok := prop.(*notionapi.TitleProperty); ok {
					entry.Title = types.StringValue(richTextToPlain(tp.Title))
					break
				}
			}

			if entry.Title.IsNull() {
				entry.Title = types.StringValue("")
			}

			entries = append(entries, entry)
		}

		if !result.HasMore {
			break
		}
		cursor = result.NextCursor
	}

	config.Entries = entries
	if config.Entries == nil {
		config.Entries = []DatabaseEntryDataModel{}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
