package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// notion_custom_emojis wraps GET /v1/custom_emojis (2026-03-25). Use it to
// resolve a custom emoji name to its ID. The SDK has no client for it.

var _ datasource.DataSource = &CustomEmojisDataSource{}

type CustomEmojisDataSource struct {
	client *notionapi.Client
}

type CustomEmojisDataSourceModel struct {
	Name   types.String       `tfsdk:"name"`
	Emojis []CustomEmojiModel `tfsdk:"emojis"`
}

type CustomEmojiModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	URL  types.String `tfsdk:"url"`
}

func NewCustomEmojisDataSource() datasource.DataSource {
	return &CustomEmojisDataSource{}
}

func (d *CustomEmojisDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_emojis"
}

func (d *CustomEmojisDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List the workspace's custom emojis. Wraps GET /v1/custom_emojis.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "Optional exact name to match, for resolving one custom emoji name to its ID.",
				Optional:    true,
			},
			"emojis": schema.ListNestedAttribute{
				Description: "Matching custom emojis.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The custom emoji ID.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The custom emoji name.",
							Computed:    true,
						},
						"url": schema.StringAttribute{
							Description: "The URL of the custom emoji image.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *CustomEmojisDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *CustomEmojisDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config CustomEmojisDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, err := tokenForClient(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Error listing custom emojis", err.Error())
		return
	}

	emojis, err := listCustomEmojis(ctx, notionAPIBaseURL, token, config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error listing custom emojis", err.Error())
		return
	}

	config.Emojis = make([]CustomEmojiModel, 0, len(emojis))
	for _, e := range emojis {
		config.Emojis = append(config.Emojis, CustomEmojiModel{
			ID:   types.StringValue(e.ID),
			Name: types.StringValue(e.Name),
			URL:  types.StringValue(e.URL),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

type rawCustomEmoji struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// listCustomEmojis pages through GET /v1/custom_emojis on 2026-03-11.
func listCustomEmojis(ctx context.Context, baseURL, token, name string) ([]rawCustomEmoji, error) {
	var out []rawCustomEmoji
	cursor := ""
	for {
		q := url.Values{"page_size": {"100"}}
		if name != "" {
			q.Set("name", name)
		}
		if cursor != "" {
			q.Set("start_cursor", cursor)
		}
		body, err := doNotionGet(ctx, baseURL+"/custom_emojis?"+q.Encode(), token, notionTrashAPIVersion)
		if err != nil {
			return nil, err
		}
		var page struct {
			Results    []rawCustomEmoji `json:"results"`
			HasMore    bool             `json:"has_more"`
			NextCursor *string          `json:"next_cursor"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("failed to parse custom emojis response: %w", err)
		}
		out = append(out, page.Results...)
		if !page.HasMore || page.NextCursor == nil || *page.NextCursor == "" {
			return out, nil
		}
		cursor = *page.NextCursor
	}
}
