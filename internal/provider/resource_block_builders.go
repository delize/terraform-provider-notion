package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

func emojiPtr(s string) *notionapi.Emoji {
	e := notionapi.Emoji(s)
	return &e
}

// buildBlockForCreate constructs a concrete SDK block from the flat schema model.
func buildBlockForCreate(plan BlockResourceModel) (notionapi.Block, error) {
	blockType := plan.Type.ValueString()

	switch blockType {
	case "paragraph":
		return &notionapi.ParagraphBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeParagraph},
			Paragraph: notionapi.Paragraph{
				RichText: plainToRichText(plan.RichText.ValueString()),
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "heading_1":
		return &notionapi.Heading1Block{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeHeading1},
			Heading1: notionapi.Heading{
				RichText:     plainToRichText(plan.RichText.ValueString()),
				Color:        plan.Color.ValueString(),
				IsToggleable: plan.IsToggleable.ValueBool(),
			},
		}, nil

	case "heading_2":
		return &notionapi.Heading2Block{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeHeading2},
			Heading2: notionapi.Heading{
				RichText:     plainToRichText(plan.RichText.ValueString()),
				Color:        plan.Color.ValueString(),
				IsToggleable: plan.IsToggleable.ValueBool(),
			},
		}, nil

	case "heading_3":
		return &notionapi.Heading3Block{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeHeading3},
			Heading3: notionapi.Heading{
				RichText:     plainToRichText(plan.RichText.ValueString()),
				Color:        plan.Color.ValueString(),
				IsToggleable: plan.IsToggleable.ValueBool(),
			},
		}, nil

	case "bulleted_list_item":
		return &notionapi.BulletedListItemBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeBulletedListItem},
			BulletedListItem: notionapi.ListItem{
				RichText: plainToRichText(plan.RichText.ValueString()),
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "numbered_list_item":
		return &notionapi.NumberedListItemBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeNumberedListItem},
			NumberedListItem: notionapi.ListItem{
				RichText: plainToRichText(plan.RichText.ValueString()),
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "to_do":
		return &notionapi.ToDoBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeToDo},
			ToDo: notionapi.ToDo{
				RichText: plainToRichText(plan.RichText.ValueString()),
				Checked:  plan.Checked.ValueBool(),
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "toggle":
		return &notionapi.ToggleBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeToggle},
			Toggle: notionapi.Toggle{
				RichText: plainToRichText(plan.RichText.ValueString()),
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "quote":
		return &notionapi.QuoteBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockQuote},
			Quote: notionapi.Quote{
				RichText: plainToRichText(plan.RichText.ValueString()),
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "callout":
		block := &notionapi.CalloutBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockCallout},
			Callout: notionapi.Callout{
				RichText: plainToRichText(plan.RichText.ValueString()),
				Color:    plan.Color.ValueString(),
			},
		}
		if !plan.Icon.IsNull() && !plan.Icon.IsUnknown() && plan.Icon.ValueString() != "" {
			block.Callout.Icon = &notionapi.Icon{
				Type:  "emoji",
				Emoji: emojiPtr(plan.Icon.ValueString()),
			}
		}
		return block, nil

	case "code":
		block := &notionapi.CodeBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeCode},
			Code: notionapi.Code{
				RichText: plainToRichText(plan.RichText.ValueString()),
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

	case "column_list":
		return &notionapi.ColumnListBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeColumnList},
			ColumnList: notionapi.ColumnList{},
		}, nil

	case "column":
		return &notionapi.ColumnBlock{
			BasicBlock: notionapi.BasicBlock{Object: notionapi.ObjectTypeBlock, Type: notionapi.BlockTypeColumn},
			Column:     notionapi.Column{},
		}, nil

	default:
		return nil, fmt.Errorf("unsupported block type: %s", blockType)
	}
}

// buildBlockUpdateRequest constructs a BlockUpdateRequest from the flat schema model.
func buildBlockUpdateRequest(plan BlockResourceModel) (*notionapi.BlockUpdateRequest, error) {
	blockType := plan.Type.ValueString()

	switch blockType {
	case "paragraph":
		return &notionapi.BlockUpdateRequest{
			Paragraph: &notionapi.Paragraph{
				RichText: plainToRichText(plan.RichText.ValueString()),
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "heading_1":
		return &notionapi.BlockUpdateRequest{
			Heading1: &notionapi.Heading{
				RichText:     plainToRichText(plan.RichText.ValueString()),
				Color:        plan.Color.ValueString(),
				IsToggleable: plan.IsToggleable.ValueBool(),
			},
		}, nil

	case "heading_2":
		return &notionapi.BlockUpdateRequest{
			Heading2: &notionapi.Heading{
				RichText:     plainToRichText(plan.RichText.ValueString()),
				Color:        plan.Color.ValueString(),
				IsToggleable: plan.IsToggleable.ValueBool(),
			},
		}, nil

	case "heading_3":
		return &notionapi.BlockUpdateRequest{
			Heading3: &notionapi.Heading{
				RichText:     plainToRichText(plan.RichText.ValueString()),
				Color:        plan.Color.ValueString(),
				IsToggleable: plan.IsToggleable.ValueBool(),
			},
		}, nil

	case "bulleted_list_item":
		return &notionapi.BlockUpdateRequest{
			BulletedListItem: &notionapi.ListItem{
				RichText: plainToRichText(plan.RichText.ValueString()),
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "numbered_list_item":
		return &notionapi.BlockUpdateRequest{
			NumberedListItem: &notionapi.ListItem{
				RichText: plainToRichText(plan.RichText.ValueString()),
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "to_do":
		return &notionapi.BlockUpdateRequest{
			ToDo: &notionapi.ToDo{
				RichText: plainToRichText(plan.RichText.ValueString()),
				Checked:  plan.Checked.ValueBool(),
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "toggle":
		return &notionapi.BlockUpdateRequest{
			Toggle: &notionapi.Toggle{
				RichText: plainToRichText(plan.RichText.ValueString()),
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "quote":
		return &notionapi.BlockUpdateRequest{
			Quote: &notionapi.Quote{
				RichText: plainToRichText(plan.RichText.ValueString()),
				Color:    plan.Color.ValueString(),
			},
		}, nil

	case "callout":
		callout := &notionapi.Callout{
			RichText: plainToRichText(plan.RichText.ValueString()),
			Color:    plan.Color.ValueString(),
		}
		if !plan.Icon.IsNull() && !plan.Icon.IsUnknown() && plan.Icon.ValueString() != "" {
			callout.Icon = &notionapi.Icon{
				Type:  "emoji",
				Emoji: emojiPtr(plan.Icon.ValueString()),
			}
		}
		return &notionapi.BlockUpdateRequest{Callout: callout}, nil

	case "code":
		code := &notionapi.Code{
			RichText: plainToRichText(plan.RichText.ValueString()),
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

// readBlockIntoState extracts fields from a concrete SDK block into the flat schema model.
func readBlockIntoState(block notionapi.Block, state *BlockResourceModel) {
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
		state.RichText = types.StringValue(richTextToPlain(b.Paragraph.RichText))
		state.Color = types.StringValue(b.Paragraph.Color)

	case *notionapi.Heading1Block:
		state.RichText = types.StringValue(richTextToPlain(b.Heading1.RichText))
		state.Color = types.StringValue(b.Heading1.Color)
		state.IsToggleable = types.BoolValue(b.Heading1.IsToggleable)

	case *notionapi.Heading2Block:
		state.RichText = types.StringValue(richTextToPlain(b.Heading2.RichText))
		state.Color = types.StringValue(b.Heading2.Color)
		state.IsToggleable = types.BoolValue(b.Heading2.IsToggleable)

	case *notionapi.Heading3Block:
		state.RichText = types.StringValue(richTextToPlain(b.Heading3.RichText))
		state.Color = types.StringValue(b.Heading3.Color)
		state.IsToggleable = types.BoolValue(b.Heading3.IsToggleable)

	case *notionapi.BulletedListItemBlock:
		state.RichText = types.StringValue(richTextToPlain(b.BulletedListItem.RichText))
		state.Color = types.StringValue(b.BulletedListItem.Color)

	case *notionapi.NumberedListItemBlock:
		state.RichText = types.StringValue(richTextToPlain(b.NumberedListItem.RichText))
		state.Color = types.StringValue(b.NumberedListItem.Color)

	case *notionapi.ToDoBlock:
		state.RichText = types.StringValue(richTextToPlain(b.ToDo.RichText))
		state.Checked = types.BoolValue(b.ToDo.Checked)
		state.Color = types.StringValue(b.ToDo.Color)

	case *notionapi.ToggleBlock:
		state.RichText = types.StringValue(richTextToPlain(b.Toggle.RichText))
		state.Color = types.StringValue(b.Toggle.Color)

	case *notionapi.QuoteBlock:
		state.RichText = types.StringValue(richTextToPlain(b.Quote.RichText))
		state.Color = types.StringValue(b.Quote.Color)

	case *notionapi.CalloutBlock:
		state.RichText = types.StringValue(richTextToPlain(b.Callout.RichText))
		state.Color = types.StringValue(b.Callout.Color)
		if b.Callout.Icon != nil && b.Callout.Icon.Emoji != nil {
			state.Icon = types.StringValue(string(*b.Callout.Icon.Emoji))
		}

	case *notionapi.CodeBlock:
		state.RichText = types.StringValue(richTextToPlain(b.Code.RichText))
		state.Language = types.StringValue(b.Code.Language)
		state.Caption = types.StringValue(richTextToPlain(b.Code.Caption))

	case *notionapi.EquationBlock:
		state.Expression = types.StringValue(b.Equation.Expression)

	case *notionapi.DividerBlock:
		// No additional fields

	case *notionapi.TableOfContentsBlock:
		state.Color = types.StringValue(b.TableOfContents.Color)

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

	case *notionapi.ColumnListBlock:
		// No additional fields

	case *notionapi.ColumnBlock:
		// No additional fields
	}
}
