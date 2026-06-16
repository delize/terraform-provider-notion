package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// notion_view_query issues POST /v1/views/{id}/query and returns the raw JSON
// response. The view's saved filter/sort are applied server-side; callers can
// optionally override page_size and start_cursor.

var _ datasource.DataSource = &ViewQueryDataSource{}

type ViewQueryDataSource struct {
	client *notionapi.Client
}

type ViewQueryDataSourceModel struct {
	ViewID      types.String `tfsdk:"view_id"`
	PageSize    types.Int64  `tfsdk:"page_size"`
	StartCursor types.String `tfsdk:"start_cursor"`
	NextCursor  types.String `tfsdk:"next_cursor"`
	HasMore     types.Bool   `tfsdk:"has_more"`
	RawJSON     types.String `tfsdk:"raw_json"`
}

func NewViewQueryDataSource() datasource.DataSource {
	return &ViewQueryDataSource{}
}

func (d *ViewQueryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_view_query"
}

func (d *ViewQueryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Query a Notion view (2026-03-19 Views API). Applies the view's saved filter/sort " +
			"server-side and returns one page of results. Returns the raw JSON response — use `jsondecode` " +
			"to extract entries. Iterate via `next_cursor`/`has_more`. Cursors are opaque strings " +
			"per the 2026-04-22 pagination change.",
		Attributes: map[string]schema.Attribute{
			"view_id": schema.StringAttribute{
				Description: "ID of the view to query.",
				Required:    true,
			},
			"page_size": schema.Int64Attribute{
				Description: "Maximum number of results per page (1-100).",
				Optional:    true,
			},
			"start_cursor": schema.StringAttribute{
				Description: "Opaque cursor returned in a prior response's `next_cursor` to fetch the next page.",
				Optional:    true,
			},
			"next_cursor": schema.StringAttribute{
				Description: "Cursor to fetch the next page, or empty when `has_more` is false.",
				Computed:    true,
			},
			"has_more": schema.BoolAttribute{
				Description: "Whether more results are available beyond this page.",
				Computed:    true,
			},
			"raw_json": schema.StringAttribute{
				Description: "Raw JSON response from POST /v1/views/{id}/query. Use `jsondecode` to access results.",
				Computed:    true,
			},
		},
	}
}

func (d *ViewQueryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ViewQueryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ViewQueryDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{}
	if !config.PageSize.IsNull() {
		body["page_size"] = config.PageSize.ValueInt64()
	}
	if !config.StartCursor.IsNull() && config.StartCursor.ValueString() != "" {
		body["start_cursor"] = config.StartCursor.ValueString()
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		resp.Diagnostics.AddError("Error encoding view query request", err.Error())
		return
	}

	token, err := tokenForClient(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Error querying view", err.Error())
		return
	}

	respBody, err := queryView(ctx, token, config.ViewID.ValueString(), bodyJSON)
	if err != nil {
		resp.Diagnostics.AddError("Error querying view", err.Error())
		return
	}

	var parsed struct {
		HasMore    bool   `json:"has_more"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		resp.Diagnostics.AddError("Error parsing view query response", err.Error())
		return
	}

	config.NextCursor = types.StringValue(parsed.NextCursor)
	config.HasMore = types.BoolValue(parsed.HasMore)
	config.RawJSON = types.StringValue(string(respBody))

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
