package provider

import (
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

var _ datasource.DataSource = &SearchDataSource{}

type SearchDataSource struct {
	client *notionapi.Client
}

type SearchDataSourceModel struct {
	Query        types.String        `tfsdk:"query"`
	FilterObject types.String        `tfsdk:"filter_object"`
	InTrash      types.Bool          `tfsdk:"in_trash"`
	Results      []SearchResultModel `tfsdk:"results"`
}

type SearchResultModel struct {
	ID         types.String `tfsdk:"id"`
	Object     types.String `tfsdk:"object"`
	Title      types.String `tfsdk:"title"`
	URL        types.String `tfsdk:"url"`
	ParentType types.String `tfsdk:"parent_type"`
	ParentID   types.String `tfsdk:"parent_id"`
	Archived   types.Bool   `tfsdk:"archived"`
}

func NewSearchDataSource() datasource.DataSource {
	return &SearchDataSource{}
}

func (d *SearchDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_search"
}

func (d *SearchDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Search the Notion workspace for pages and databases the integration has access to. Wraps the /v1/search endpoint.",
		Attributes: map[string]schema.Attribute{
			"query": schema.StringAttribute{
				Description: "Optional substring to match against page/database titles. Omit to list everything accessible.",
				Optional:    true,
			},
			"filter_object": schema.StringAttribute{
				Description: `Optionally restrict results to "page" or "database". Omit for both.`,
				Optional:    true,
			},
			"in_trash": schema.BoolAttribute{
				Description: "When true, list pages and databases in the trash instead of live ones. Combines with query and filter_object. Defaults to false.",
				Optional:    true,
			},
			"results": schema.ListNestedAttribute{
				Description: "All matching pages and databases.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The Notion ID of the page or database.",
							Computed:    true,
						},
						"object": schema.StringAttribute{
							Description: `Either "page" or "database".`,
							Computed:    true,
						},
						"title": schema.StringAttribute{
							Description: "The plain-text title of the page or database.",
							Computed:    true,
						},
						"url": schema.StringAttribute{
							Description: "The Notion URL of the page or database.",
							Computed:    true,
						},
						"parent_type": schema.StringAttribute{
							Description: `The parent kind ("workspace", "page_id", "database_id", or "block_id").`,
							Computed:    true,
						},
						"parent_id": schema.StringAttribute{
							Description: "The parent ID, if any. Empty when parent_type is workspace.",
							Computed:    true,
						},
						"archived": schema.BoolAttribute{
							Description: "Whether the result is archived.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *SearchDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SearchDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SearchDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, err := tokenForClient(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Error searching Notion", err.Error())
		return
	}

	cursor := ""
	for {
		body := buildSearchBody(config.Query.ValueString(), config.FilterObject.ValueString(), config.InTrash.ValueBool(), cursor)
		page, err := doSearch(ctx, notionAPIBaseURL, token, body)
		if err != nil {
			resp.Diagnostics.AddError("Error searching Notion", err.Error())
			return
		}

		for _, obj := range page.Results {
			config.Results = append(config.Results, searchResultFor(obj))
		}

		if !page.HasMore || page.NextCursor == "" {
			break
		}
		cursor = string(page.NextCursor)
	}

	if config.Results == nil {
		config.Results = []SearchResultModel{}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// buildSearchBody builds a POST /v1/search body. The SDK's SearchFilter has
// no in_trash field (2026-07-15), so search goes over direct HTTP. in_trash
// can stand alone or sit next to the object filter.
func buildSearchBody(query, filterObject string, inTrash bool, cursor string) map[string]interface{} {
	body := map[string]interface{}{"page_size": 100}
	if query != "" {
		body["query"] = query
	}
	if cursor != "" {
		body["start_cursor"] = cursor
	}
	filter := map[string]interface{}{}
	if filterObject != "" {
		filter["property"] = "object"
		filter["value"] = filterObject
	}
	if inTrash {
		filter["in_trash"] = true
	}
	if len(filter) > 0 {
		body["filter"] = filter
	}
	return body
}

// doSearch posts one search page on Notion-Version 2022-06-28, where results
// are pages and databases, and decodes it with the SDK's response type so
// searchResultFor keeps working unchanged.
func doSearch(ctx context.Context, baseURL, token string, body map[string]interface{}) (*notionapi.SearchResponse, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := doNotionRequestWithVersion(ctx, http.MethodPost, baseURL+"/search", token, legacySchemaAPIVersion, b)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("notion API %d on search: %s", resp.StatusCode, string(respBody))
	}
	var page notionapi.SearchResponse
	if err := json.Unmarshal(respBody, &page); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}
	return &page, nil
}

// searchResultFor converts a Notion search result (Page or Database) into the
// flat representation we surface to Terraform.
func searchResultFor(obj notionapi.Object) SearchResultModel {
	switch v := obj.(type) {
	case *notionapi.Page:
		return SearchResultModel{
			ID:         types.StringValue(normalizeID(string(v.ID))),
			Object:     types.StringValue(string(v.Object)),
			Title:      types.StringValue(pageTitle(v)),
			URL:        types.StringValue(v.URL),
			ParentType: types.StringValue(string(v.Parent.Type)),
			ParentID:   types.StringValue(parentID(v.Parent)),
			Archived:   types.BoolValue(v.Archived),
		}
	case *notionapi.Database:
		return SearchResultModel{
			ID:         types.StringValue(normalizeID(string(v.ID))),
			Object:     types.StringValue(string(v.Object)),
			Title:      types.StringValue(richTextPlain(v.Title)),
			URL:        types.StringValue(v.URL),
			ParentType: types.StringValue(string(v.Parent.Type)),
			ParentID:   types.StringValue(parentID(v.Parent)),
			Archived:   types.BoolValue(v.Archived),
		}
	default:
		return SearchResultModel{
			ID:     types.StringValue(""),
			Object: types.StringValue("unknown"),
		}
	}
}

// pageTitle returns the plain-text title of a Page by finding its title
// property. Pages have one title-typed property whose name varies.
func pageTitle(p *notionapi.Page) string {
	for _, prop := range p.Properties {
		if tp, ok := prop.(*notionapi.TitleProperty); ok {
			return richTextPlain(tp.Title)
		}
	}
	return ""
}

func richTextPlain(rts []notionapi.RichText) string {
	out := ""
	for _, rt := range rts {
		out += rt.PlainText
	}
	return out
}

func parentID(p notionapi.Parent) string {
	switch p.Type {
	case notionapi.ParentTypePageID:
		return normalizeID(string(p.PageID))
	case notionapi.ParentTypeDatabaseID:
		return normalizeID(string(p.DatabaseID))
	case notionapi.ParentTypeBlockID:
		return normalizeID(string(p.BlockID))
	default:
		return ""
	}
}
