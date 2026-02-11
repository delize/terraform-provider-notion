package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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

	result, err := d.searchRaw(ctx, config.Query.ValueString(), "database")
	if err != nil {
		resp.Diagnostics.AddError("Error searching for database", err.Error())
		return
	}

	if len(result.Results) == 0 {
		resp.Diagnostics.AddError("Database not found",
			fmt.Sprintf("No database found matching query: %s", config.Query.ValueString()))
		return
	}

	db := result.Results[0]
	config.ID = types.StringValue(normalizeID(db.ID))
	config.Title = types.StringValue(extractRawTitle(db.Title))
	config.URL = types.StringValue(db.URL)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

type rawSearchResponse struct {
	Results []rawSearchResult `json:"results"`
}

type rawSearchResult struct {
	ID     string          `json:"id"`
	URL    string          `json:"url"`
	Title  json.RawMessage `json:"title"`
	Object string          `json:"object"`
}

func extractRawTitle(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var items []rawRichText
	if err := json.Unmarshal(raw, &items); err != nil {
		return ""
	}
	var result string
	for _, item := range items {
		result += item.PlainText
	}
	return result
}

// searchRaw queries the Notion search API directly, bypassing the SDK's
// strict property type checking.
func (d *DatabaseDataSource) searchRaw(ctx context.Context, query string, objectType string) (*rawSearchResponse, error) {
	body := map[string]interface{}{
		"query":     query,
		"page_size": 1,
		"filter": map[string]string{
			"value":    objectType,
			"property": "object",
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.notion.com/v1/search", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", d.client.Token.String()))
	httpReq.Header.Set("Notion-Version", "2022-06-28")
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Notion API error (status %d): %s", httpResp.StatusCode, string(respBody))
	}

	var result rawSearchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}
