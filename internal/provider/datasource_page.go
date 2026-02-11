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

	result, err := d.searchPageRaw(ctx, config.Query.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error searching for page", err.Error())
		return
	}

	if len(result.Results) == 0 {
		resp.Diagnostics.AddError("Page not found",
			fmt.Sprintf("No page found matching query: %s", config.Query.ValueString()))
		return
	}

	page := result.Results[0]
	config.ID = types.StringValue(normalizeID(page.ID))
	config.URL = types.StringValue(page.URL)

	if page.Parent.Type == "page_id" && page.Parent.PageID != "" {
		config.ParentPageID = types.StringValue(normalizeID(page.Parent.PageID))
	} else {
		config.ParentPageID = types.StringValue("")
	}

	// Extract title from properties
	for _, prop := range page.Properties {
		if prop.Type == "title" {
			config.Title = types.StringValue(extractRichText(prop.Title))
			break
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

type rawPageSearchResponse struct {
	Results []rawPageResult `json:"results"`
}

type rawPageResult struct {
	ID         string                 `json:"id"`
	URL        string                 `json:"url"`
	Parent     rawParent              `json:"parent"`
	Properties map[string]rawProperty `json:"properties"`
}

type rawParent struct {
	Type   string `json:"type"`
	PageID string `json:"page_id,omitempty"`
}

func (d *PageDataSource) searchPageRaw(ctx context.Context, query string) (*rawPageSearchResponse, error) {
	body := map[string]interface{}{
		"query":     query,
		"page_size": 1,
		"filter": map[string]string{
			"value":    "page",
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

	var result rawPageSearchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}
