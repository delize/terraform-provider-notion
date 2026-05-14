package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var _ datasource.DataSource = &BlocksDataSource{}

type BlocksDataSource struct {
	client *notionapi.Client
}

type BlocksDataSourceModel struct {
	ParentID types.String     `tfsdk:"parent_id"`
	Blocks   []BlockDataModel `tfsdk:"blocks"`
}

type BlockDataModel struct {
	ID          types.String `tfsdk:"id"`
	Type        types.String `tfsdk:"type"`
	HasChildren types.Bool   `tfsdk:"has_children"`
	PlainText   types.String `tfsdk:"plain_text"`
	Archived    types.Bool   `tfsdk:"archived"`
}

func NewBlocksDataSource() datasource.DataSource {
	return &BlocksDataSource{}
}

func (d *BlocksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_blocks"
}

func (d *BlocksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List the immediate child blocks of a Notion page or block. Wraps /v1/blocks/{id}/children.",
		Attributes: map[string]schema.Attribute{
			"parent_id": schema.StringAttribute{
				Description: "The ID of the page or block whose children should be listed.",
				Required:    true,
			},
			"blocks": schema.ListNestedAttribute{
				Description: "Immediate children of parent_id, in document order.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The block ID.",
							Computed:    true,
						},
						"type": schema.StringAttribute{
							Description: "The block type (e.g. paragraph, heading_1, code, image).",
							Computed:    true,
						},
						"has_children": schema.BoolAttribute{
							Description: "Whether this block has nested children. Use a separate notion_blocks data source with parent_id set to this block's ID to fetch them.",
							Computed:    true,
						},
						"plain_text": schema.StringAttribute{
							Description: "Best-effort plain-text representation of the block's content. Empty for blocks without textual content (dividers, images, etc.).",
							Computed:    true,
						},
						"archived": schema.BoolAttribute{
							Description: "Whether the block is archived.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *BlocksDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *BlocksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config BlocksDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	parentID := normalizeID(config.ParentID.ValueString())
	var cursor notionapi.Cursor
	for {
		page, err := d.client.Block.GetChildren(ctx, notionapi.BlockID(parentID), &notionapi.Pagination{
			StartCursor: cursor,
			PageSize:    100,
		})
		if err != nil {
			resp.Diagnostics.AddError("Error listing block children", err.Error())
			return
		}

		for _, b := range page.Results {
			config.Blocks = append(config.Blocks, blockDataModel(b))
		}

		if !page.HasMore {
			break
		}
		cursor = notionapi.Cursor(page.NextCursor)
	}

	if config.Blocks == nil {
		config.Blocks = []BlockDataModel{}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// blockDataModel converts an SDK Block into the flat representation we expose
// to Terraform. plain_text extraction is best-effort: for block types whose
// textual content is exposed via well-known fields we surface it; otherwise
// the field is empty.
func blockDataModel(b notionapi.Block) BlockDataModel {
	model := BlockDataModel{
		ID:          types.StringValue(normalizeID(string(b.GetID()))),
		Type:        types.StringValue(string(b.GetType())),
		HasChildren: types.BoolValue(b.GetHasChildren()),
		Archived:    types.BoolValue(b.GetArchived()),
		PlainText:   types.StringValue(blockPlainText(b)),
	}
	return model
}

func blockPlainText(b notionapi.Block) string {
	switch v := b.(type) {
	case *notionapi.ParagraphBlock:
		return richTextPlain(v.Paragraph.RichText)
	case *notionapi.Heading1Block:
		return richTextPlain(v.Heading1.RichText)
	case *notionapi.Heading2Block:
		return richTextPlain(v.Heading2.RichText)
	case *notionapi.Heading3Block:
		return richTextPlain(v.Heading3.RichText)
	case *notionapi.BulletedListItemBlock:
		return richTextPlain(v.BulletedListItem.RichText)
	case *notionapi.NumberedListItemBlock:
		return richTextPlain(v.NumberedListItem.RichText)
	case *notionapi.ToDoBlock:
		return richTextPlain(v.ToDo.RichText)
	case *notionapi.ToggleBlock:
		return richTextPlain(v.Toggle.RichText)
	case *notionapi.QuoteBlock:
		return richTextPlain(v.Quote.RichText)
	case *notionapi.CalloutBlock:
		return richTextPlain(v.Callout.RichText)
	case *notionapi.CodeBlock:
		return richTextPlain(v.Code.RichText)
	}
	return ""
}
