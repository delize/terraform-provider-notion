package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// resolveRichText returns RichText from rich_text_json if set, otherwise from rich_text with markdown link parsing.
func resolveRichText(plan BlockResourceModel) ([]notionapi.RichText, error) {
	if !plan.RichTextJSON.IsNull() && !plan.RichTextJSON.IsUnknown() {
		return jsonToRichText(plan.RichTextJSON.ValueString())
	}
	return plainToRichText(plan.RichText.ValueString()), nil
}

// buildBlockForCreate constructs a concrete SDK block from the flat schema model.
func buildBlockForCreate(plan BlockResourceModel) (notionapi.Block, error) {
	blockType := plan.Type.ValueString()

	rt, err := resolveRichText(plan)
	if err != nil {
		return nil, err
	}

	switch blockType {
	case "paragraph":
		return &notionapi.ParagraphBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeParagraph},
			Paragraph: notionapi.Paragraph{
				RichText: rt,
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "heading_1":
		return &notionapi.Heading1Block{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeHeading1},
			Heading1: notionapi.Heading{
				RichText:     rt,
				Color:        plan.Color.ValueString(),
				IsToggleable: plan.IsToggleable.ValueBool(),
			},
		}, nil

	case "heading_2":
		return &notionapi.Heading2Block{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeHeading2},
			Heading2: notionapi.Heading{
				RichText:     rt,
				Color:        plan.Color.ValueString(),
				IsToggleable: plan.IsToggleable.ValueBool(),
			},
		}, nil

	case "heading_3":
		return &notionapi.Heading3Block{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeHeading3},
			Heading3: notionapi.Heading{
				RichText:     rt,
				Color:        plan.Color.ValueString(),
				IsToggleable: plan.IsToggleable.ValueBool(),
			},
		}, nil

	case "bulleted_list_item":
		return &notionapi.BulletedListItemBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeBulletedListItem},
			BulletedListItem: notionapi.ListItem{
				RichText: rt,
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "numbered_list_item":
		return &notionapi.NumberedListItemBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeNumberedListItem},
			NumberedListItem: notionapi.ListItem{
				RichText: rt,
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "to_do":
		return &notionapi.ToDoBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeToDo},
			ToDo: notionapi.ToDo{
				RichText: rt,
				Checked:  plan.Checked.ValueBool(),
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "toggle":
		return &notionapi.ToggleBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeToggle},
			Toggle: notionapi.Toggle{
				RichText: rt,
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "quote":
		return &notionapi.QuoteBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockQuote},
			Quote: notionapi.Quote{
				RichText: rt,
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "code":
		block := &notionapi.CodeBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeCode},
			Code: notionapi.Code{
				RichText: rt,
				Language: plan.Language.ValueString(),
			},
		}
		if !plan.Caption.IsNull() && !plan.Caption.IsUnknown() && plan.Caption.ValueString() != "" {
			block.Code.Caption = plainToRichText(plan.Caption.ValueString())
		}
		return block, nil

	case "equation":
		return &notionapi.EquationBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeEquation},
			Equation: notionapi.Equation{
				Expression: plan.Expression.ValueString(),
			},
		}, nil

	case "divider":
		return &notionapi.DividerBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeDivider},
			Divider:    notionapi.Divider{},
		}, nil

	case "table_of_contents":
		return &notionapi.TableOfContentsBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeTableOfContents},
			TableOfContents: notionapi.TableOfContents{
				Color: plan.Color.ValueString(),
			},
		}, nil

	case "bookmark":
		block := &notionapi.BookmarkBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeBookmark},
			Bookmark: notionapi.Bookmark{
				URL: plan.URL.ValueString(),
			},
		}
		if !plan.Caption.IsNull() && !plan.Caption.IsUnknown() && plan.Caption.ValueString() != "" {
			block.Bookmark.Caption = plainToRichText(plan.Caption.ValueString())
		}
		return block, nil

	case "embed":
		return &notionapi.EmbedBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeEmbed},
			Embed: notionapi.Embed{
				URL: plan.URL.ValueString(),
			},
		}, nil

	case "image":
		block := &notionapi.ImageBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeImage},
			Image: notionapi.Image{
				Type:     notionapi.FileTypeExternal,
				External: &notionapi.FileObject{URL: plan.URL.ValueString()},
			},
		}
		if !plan.Caption.IsNull() && !plan.Caption.IsUnknown() && plan.Caption.ValueString() != "" {
			block.Image.Caption = plainToRichText(plan.Caption.ValueString())
		}
		return block, nil

	case "synced_block":
		synced := notionapi.Synced{}
		if !plan.SyncedFrom.IsNull() && !plan.SyncedFrom.IsUnknown() && plan.SyncedFrom.ValueString() != "" {
			synced.SyncedFrom = &notionapi.SyncedFrom{
				BlockID: notionapi.BlockID(plan.SyncedFrom.ValueString()),
			}
		}
		return &notionapi.SyncedBlock{
			BasicBlock:  notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeSyncedBlock},
			SyncedBlock: synced,
		}, nil

	case "column":
		// Notion rejects a column with no children, and ModifyPlan refuses
		// this at plan time, so reaching it is a provider bug.
		return nil, fmt.Errorf("a column can't be created on its own, use a column_list with columns")

	case "heading_4", "tab", "callout", "column_list":
		// The SDK doesn't model these (or, for callout, its icon union, and
		// for column_list, the nested columns Notion requires on create).
		// resource_block_raw.go creates them over direct HTTP, so reaching
		// this is a provider bug.
		return nil, fmt.Errorf("block type %q is built by the direct HTTP path, not the SDK builder", blockType)

	default:
		return nil, fmt.Errorf("unsupported block type: %s", blockType)
	}
}

// buildBlockUpdateRequest constructs a BlockUpdateRequest from the flat schema model.
func buildBlockUpdateRequest(plan BlockResourceModel) (*notionapi.BlockUpdateRequest, error) {
	blockType := plan.Type.ValueString()

	rt, err := resolveRichText(plan)
	if err != nil {
		return nil, err
	}

	switch blockType {
	case "paragraph":
		return &notionapi.BlockUpdateRequest{
			Paragraph: &notionapi.Paragraph{
				RichText: rt,
				Color:    blockColorForUpdate(plan.Color),
			},
		}, nil

	case "heading_1":
		return &notionapi.BlockUpdateRequest{
			Heading1: &notionapi.Heading{
				RichText:     rt,
				Color:        blockColorForUpdate(plan.Color),
				IsToggleable: plan.IsToggleable.ValueBool(),
			},
		}, nil

	case "heading_2":
		return &notionapi.BlockUpdateRequest{
			Heading2: &notionapi.Heading{
				RichText:     rt,
				Color:        blockColorForUpdate(plan.Color),
				IsToggleable: plan.IsToggleable.ValueBool(),
			},
		}, nil

	case "heading_3":
		return &notionapi.BlockUpdateRequest{
			Heading3: &notionapi.Heading{
				RichText:     rt,
				Color:        blockColorForUpdate(plan.Color),
				IsToggleable: plan.IsToggleable.ValueBool(),
			},
		}, nil

	case "bulleted_list_item":
		return &notionapi.BlockUpdateRequest{
			BulletedListItem: &notionapi.ListItem{
				RichText: rt,
				Color:    blockColorForUpdate(plan.Color),
			},
		}, nil

	case "numbered_list_item":
		return &notionapi.BlockUpdateRequest{
			NumberedListItem: &notionapi.ListItem{
				RichText: rt,
				Color:    blockColorForUpdate(plan.Color),
			},
		}, nil

	case "to_do":
		return &notionapi.BlockUpdateRequest{
			ToDo: &notionapi.ToDo{
				RichText: rt,
				Checked:  plan.Checked.ValueBool(),
				Color:    blockColorForUpdate(plan.Color),
			},
		}, nil

	case "toggle":
		return &notionapi.BlockUpdateRequest{
			Toggle: &notionapi.Toggle{
				RichText: rt,
				Color:    blockColorForUpdate(plan.Color),
			},
		}, nil

	case "quote":
		return &notionapi.BlockUpdateRequest{
			Quote: &notionapi.Quote{
				RichText: rt,
				Color:    blockColorForUpdate(plan.Color),
			},
		}, nil

	case "code":
		code := &notionapi.Code{
			RichText: rt,
			Language: plan.Language.ValueString(),
		}
		if !plan.Caption.IsNull() && !plan.Caption.IsUnknown() {
			code.Caption = plainToRichText(plan.Caption.ValueString())
		}
		return &notionapi.BlockUpdateRequest{Code: code}, nil

	case "equation":
		return &notionapi.BlockUpdateRequest{
			Equation: &notionapi.Equation{
				Expression: plan.Expression.ValueString(),
			},
		}, nil

	case "bookmark":
		bm := &notionapi.Bookmark{
			URL: plan.URL.ValueString(),
		}
		if !plan.Caption.IsNull() && !plan.Caption.IsUnknown() {
			bm.Caption = plainToRichText(plan.Caption.ValueString())
		}
		return &notionapi.BlockUpdateRequest{Bookmark: bm}, nil

	case "embed":
		return &notionapi.BlockUpdateRequest{
			Embed: &notionapi.Embed{
				URL: plan.URL.ValueString(),
			},
		}, nil

	case "image":
		img := &notionapi.Image{
			Type:     notionapi.FileTypeExternal,
			External: &notionapi.FileObject{URL: plan.URL.ValueString()},
		}
		if !plan.Caption.IsNull() && !plan.Caption.IsUnknown() {
			img.Caption = plainToRichText(plan.Caption.ValueString())
		}
		return &notionapi.BlockUpdateRequest{Image: img}, nil

	case "divider", "table_of_contents", "synced_block", "column_list", "column":
		return nil, fmt.Errorf("block type %q does not support updates", blockType)

	default:
		return nil, fmt.Errorf("unsupported block type: %s", blockType)
	}
}

// setRichTextState copies a block's rich text into the state. Without
// rich_text_json, rich_text gets the markdown-aware plain text.
//
// With rich_text_json set, that attribute owns the text. It keeps its prior
// string when Notion's rich text matches it (see richTextJSONReadback), and
// rich_text, which is ignored then, keeps its prior value (the "" default
// unless the config sets it). Writing the plain text into rich_text would
// differ from the plan and fail the apply.
func setRichTextState(rt []notionapi.RichText, state *BlockResourceModel) {
	if state.RichTextJSON.IsNull() || state.RichTextJSON.IsUnknown() {
		state.RichTextJSON = types.StringNull()
		state.RichText = types.StringValue(richTextToPlain(rt))
		return
	}
	if state.RichText.IsNull() || state.RichText.IsUnknown() {
		state.RichText = types.StringValue("")
	}
	state.RichTextJSON = richTextJSONReadback(state.RichTextJSON, rt)
}

// readBlockIntoState extracts fields from a concrete SDK block into the flat schema model.
// Notion reports an unset color as "default". normalizeBlockColor keeps that
// as "" when the prior value was empty, so a block with no color configured
// doesn't plan "default" -> "" forever.
func readBlockIntoState(block notionapi.Block, state *BlockResourceModel) {
	priorColor := state.Color
	state.ID = types.StringValue(normalizeID(string(block.GetID())))
	state.HasChildren = types.BoolValue(block.GetHasChildren())

	blockType := string(block.GetType())
	state.Type = types.StringValue(blockType)

	// Set parent_id from block's parent
	if parent := block.GetParent(); parent != nil {
		switch parent.Type {
		case notionapi.ParentTypePageID:
			state.ParentID = types.StringValue(normalizeID(string(parent.PageID)))
		case notionapi.ParentTypeBlockID:
			state.ParentID = types.StringValue(normalizeID(string(parent.BlockID)))
		}
	}

	switch b := block.(type) {
	case *notionapi.ParagraphBlock:
		setRichTextState(b.Paragraph.RichText, state)
		state.Color = types.StringValue(normalizeBlockColor(b.Paragraph.Color, priorColor))

	case *notionapi.Heading1Block:
		setRichTextState(b.Heading1.RichText, state)
		state.Color = types.StringValue(normalizeBlockColor(b.Heading1.Color, priorColor))
		state.IsToggleable = types.BoolValue(b.Heading1.IsToggleable)

	case *notionapi.Heading2Block:
		setRichTextState(b.Heading2.RichText, state)
		state.Color = types.StringValue(normalizeBlockColor(b.Heading2.Color, priorColor))
		state.IsToggleable = types.BoolValue(b.Heading2.IsToggleable)

	case *notionapi.Heading3Block:
		setRichTextState(b.Heading3.RichText, state)
		state.Color = types.StringValue(normalizeBlockColor(b.Heading3.Color, priorColor))
		state.IsToggleable = types.BoolValue(b.Heading3.IsToggleable)

	case *notionapi.BulletedListItemBlock:
		setRichTextState(b.BulletedListItem.RichText, state)
		state.Color = types.StringValue(normalizeBlockColor(b.BulletedListItem.Color, priorColor))

	case *notionapi.NumberedListItemBlock:
		setRichTextState(b.NumberedListItem.RichText, state)
		state.Color = types.StringValue(normalizeBlockColor(b.NumberedListItem.Color, priorColor))

	case *notionapi.ToDoBlock:
		setRichTextState(b.ToDo.RichText, state)
		state.Checked = types.BoolValue(b.ToDo.Checked)
		state.Color = types.StringValue(normalizeBlockColor(b.ToDo.Color, priorColor))

	case *notionapi.ToggleBlock:
		setRichTextState(b.Toggle.RichText, state)
		state.Color = types.StringValue(normalizeBlockColor(b.Toggle.Color, priorColor))

	case *notionapi.QuoteBlock:
		setRichTextState(b.Quote.RichText, state)
		state.Color = types.StringValue(normalizeBlockColor(b.Quote.Color, priorColor))

	case *notionapi.CodeBlock:
		setRichTextState(b.Code.RichText, state)
		state.Language = types.StringValue(b.Code.Language)
		state.Caption = types.StringValue(richTextToPlain(b.Code.Caption))

	case *notionapi.EquationBlock:
		state.Expression = types.StringValue(b.Equation.Expression)

	case *notionapi.DividerBlock:
		// No additional fields

	case *notionapi.TableOfContentsBlock:
		state.Color = types.StringValue(normalizeBlockColor(b.TableOfContents.Color, priorColor))

	case *notionapi.BookmarkBlock:
		state.URL = types.StringValue(b.Bookmark.URL)
		state.Caption = types.StringValue(richTextToPlain(b.Bookmark.Caption))

	case *notionapi.EmbedBlock:
		state.URL = types.StringValue(b.Embed.URL)

	case *notionapi.ImageBlock:
		state.URL = types.StringValue(b.Image.GetURL())
		state.Caption = types.StringValue(richTextToPlain(b.Image.Caption))

	case *notionapi.SyncedBlock:
		if b.SyncedBlock.SyncedFrom != nil {
			state.SyncedFrom = types.StringValue(normalizeID(string(b.SyncedBlock.SyncedFrom.BlockID)))
		}

	case *notionapi.ColumnBlock:
		// No additional fields
	}
	// Attributes this block type doesn't use get their schema defaults, so an
	// import reads back the same values a create stores. readRawBlockIntoState
	// does the same for the raw block types.
	for _, s := range []*types.String{&state.RichText, &state.Color, &state.Icon, &state.Language, &state.Caption, &state.URL, &state.Expression} {
		setStringDefault(s)
	}
	if state.IsToggleable.IsNull() || state.IsToggleable.IsUnknown() {
		state.IsToggleable = types.BoolValue(false)
	}
	if state.Checked.IsNull() || state.Checked.IsUnknown() {
		state.Checked = types.BoolValue(false)
	}
	if state.Columns.IsUnknown() {
		state.Columns = types.ListNull(columnObjectType)
	}
}
