package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Resource-side handling for the block types that go over direct HTTP
// (see notion_blocks_raw.go). Everything else in notion_block stays on the
// SDK path in resource_block.go.

// rawBlockTypes lists the notion_block types handled here.
var rawBlockTypes = map[string]bool{"heading_4": true, "tab": true, "callout": true}

func isRawBlockType(t string) bool { return rawBlockTypes[t] }

// tabItemAttrTypes is the object type of one tab_items element.
var tabItemAttrTypes = map[string]attr.Type{
	"id":    types.StringType,
	"label": types.StringType,
	"icon":  types.StringType,
}

type tabItemModel struct {
	ID    types.String `tfsdk:"id"`
	Label types.String `tfsdk:"label"`
	Icon  types.String `tfsdk:"icon"`
}

func tabItemsFromList(ctx context.Context, l types.List) ([]tabItemModel, diag.Diagnostics) {
	var items []tabItemModel
	if l.IsNull() || l.IsUnknown() {
		return nil, nil
	}
	diags := l.ElementsAs(ctx, &items, false)
	return items, diags
}

func tabItemsToList(ctx context.Context, items []tabItemModel) (types.List, diag.Diagnostics) {
	return types.ListValueFrom(ctx, types.ObjectType{AttrTypes: tabItemAttrTypes}, items)
}

func (m tabItemModel) spec() tabItemSpec {
	return tabItemSpec{Label: m.Label.ValueString(), Icon: m.Icon.ValueString()}
}

// createRawBlock appends a heading_4, tab, or callout block and fills plan
// from the result. For a tab it then lists the new tab items to learn their
// IDs.
func (r *BlockResource) createRawBlock(ctx context.Context, plan *BlockResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	token, err := tokenForClient(r.client)
	if err != nil {
		diags.AddError("Error creating block", err.Error())
		return diags
	}

	var block map[string]interface{}
	var items []tabItemModel
	switch plan.Type.ValueString() {
	case "heading_4":
		rt, err := resolveRichText(*plan)
		if err != nil {
			diags.AddError("Error building block", err.Error())
			return diags
		}
		block = map[string]interface{}{
			"object":    "block",
			"type":      "heading_4",
			"heading_4": heading4Body(rt, plan.Color.ValueString(), plan.IsToggleable.ValueBool()),
		}
	case "callout":
		rt, err := resolveRichText(*plan)
		if err != nil {
			diags.AddError("Error building block", err.Error())
			return diags
		}
		block = map[string]interface{}{
			"object":  "block",
			"type":    "callout",
			"callout": calloutBody(rt, plan.Color.ValueString(), plan.iconFields().request(), false),
		}
	case "tab":
		items, diags = tabItemsFromList(ctx, plan.TabItems)
		if diags.HasError() {
			return diags
		}
		specs := make([]tabItemSpec, len(items))
		for i, it := range items {
			specs[i] = it.spec()
		}
		block = tabBlock(specs)
	}

	after := ""
	if !plan.After.IsNull() && !plan.After.IsUnknown() {
		after = plan.After.ValueString()
	}
	// Appending has no idempotency key, so a resend after Notion answered
	// could create the block twice.
	results, err := appendBlocksRaw(withNoReplay(ctx), token, plan.ParentID.ValueString(), []map[string]interface{}{block}, after)
	if err != nil {
		diags.AddError("Error creating block", err.Error())
		return diags
	}
	if len(results) == 0 {
		diags.AddError("Error creating block", "No block returned from Notion API")
		return diags
	}

	created := results[0]
	planned := plan.iconFields()
	readRawBlockIntoState(&created, plan)
	if plan.Type.ValueString() == "callout" {
		plan.setIconFields(keepPlannedIcon(planned, plan.iconFields()))
	}

	if plan.Type.ValueString() == "tab" {
		children, err := listBlockChildrenRaw(ctx, token, created.ID)
		switch {
		case err != nil:
			diags.AddError("Error reading new tab items", err.Error())
		case len(children) != len(items):
			diags.AddError("Error reading new tab items",
				fmt.Sprintf("created %d tab items but Notion returned %d", len(items), len(children)))
		default:
			for i := range items {
				items[i].ID = types.StringValue(normalizeID(children[i].ID))
			}
		}
		// On error the item IDs stay null so the block can still be saved
		// (and tainted) instead of orphaned.
		for i := range items {
			if items[i].ID.IsUnknown() {
				items[i].ID = types.StringNull()
			}
		}
		list, d := tabItemsToList(ctx, items)
		diags.Append(d...)
		plan.TabItems = list
	}
	return diags
}

// readRawBlock refreshes state for a heading_4, tab, or callout block. It
// reports found=false when the block is gone or in the trash.
func (r *BlockResource) readRawBlock(ctx context.Context, token string, b *rawBlock, state *BlockResourceModel) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	if b.InTrash || b.Archived {
		return false, diags
	}
	readRawBlockIntoState(b, state)

	if b.Type == "tab" {
		children, err := listBlockChildrenRaw(ctx, token, b.ID)
		if err != nil {
			diags.AddError("Error reading tab items", err.Error())
			return true, diags
		}
		list, d := tabItemsToList(ctx, tabItemsFromChildren(children))
		diags.Append(d...)
		state.TabItems = list
	}
	return true, diags
}

// tabItemsFromChildren turns a tab block's paragraph children into
// tab_items, in Notion's order.
func tabItemsFromChildren(children []rawBlock) []tabItemModel {
	items := make([]tabItemModel, 0, len(children))
	for _, c := range children {
		if c.Type != "paragraph" || c.Paragraph == nil || c.InTrash || c.Archived {
			continue
		}
		items = append(items, tabItemModel{
			ID:    types.StringValue(normalizeID(c.ID)),
			Label: types.StringValue(richTextToPlain(c.Paragraph.RichText)),
			Icon:  types.StringValue(c.Paragraph.emoji()),
		})
	}
	return items
}

// updateRawBlock applies a plan to an existing heading_4, tab, or callout
// block.
func (r *BlockResource) updateRawBlock(ctx context.Context, prior, plan *BlockResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	token, err := tokenForClient(r.client)
	if err != nil {
		diags.AddError("Error updating block", err.Error())
		return diags
	}

	switch plan.Type.ValueString() {
	case "heading_4":
		rt, err := resolveRichText(*plan)
		if err != nil {
			diags.AddError("Error building block update", err.Error())
			return diags
		}
		updated, err := updateBlockRaw(ctx, token, plan.ID.ValueString(), map[string]interface{}{
			"heading_4": heading4Body(rt, blockColorForUpdate(plan.Color), plan.IsToggleable.ValueBool()),
		})
		if err != nil {
			diags.AddError("Error updating block", err.Error())
			return diags
		}
		readRawBlockIntoState(updated, plan)

	case "callout":
		rt, err := resolveRichText(*plan)
		if err != nil {
			diags.AddError("Error building block update", err.Error())
			return diags
		}
		planned := plan.iconFields()
		icon := planned.request()
		// Every icon attribute was removed from the config, so clear the
		// icon the prior state tracked.
		clearIcon := icon == nil && (prior.Icon.ValueString() != "" || prior.iconFields().managed())
		updated, err := updateBlockRaw(ctx, token, plan.ID.ValueString(), map[string]interface{}{
			"callout": calloutBody(rt, plan.Color.ValueString(), icon, clearIcon),
		})
		if err != nil {
			diags.AddError("Error updating block", err.Error())
			return diags
		}
		readRawBlockIntoState(updated, plan)
		plan.setIconFields(keepPlannedIcon(planned, plan.iconFields()))

	case "tab":
		priorItems, d := tabItemsFromList(ctx, prior.TabItems)
		diags.Append(d...)
		planItems, d := tabItemsFromList(ctx, plan.TabItems)
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		if removed := tabIconRemovals(priorItems, planItems); len(removed) > 0 {
			diags.AddAttributeError(path.Root("tab_items"), "Notion can't remove a tab icon through the API",
				fmt.Sprintf("The config removes the icon from tab %s. Notion's API can set or change a tab icon but not remove it. "+
					"Remove it in the Notion app and then from the config, or set a different emoji.", strings.Join(removed, ", ")))
			return diags
		}
		ids, err := reconcileTabItems(ctx, token, plan.ID.ValueString(), priorItems, planItems)
		if err != nil {
			diags.AddError("Error updating tab items", err.Error())
			return diags
		}
		for i := range planItems {
			planItems[i].ID = types.StringValue(ids[i])
		}
		list, d := tabItemsToList(ctx, planItems)
		diags.Append(d...)
		plan.TabItems = list
		plan.HasChildren = types.BoolValue(true)
	}
	return diags
}

// tabIconRemovals returns the labels of tabs whose icon the plan removes.
// Update a block takes only an icon object, so this is checked before any
// request instead of failing halfway through the update.
func tabIconRemovals(prior, plan []tabItemModel) []string {
	var out []string
	for i := range plan {
		if i < len(prior) && prior[i].Icon.ValueString() != "" && plan[i].Icon.ValueString() == "" {
			out = append(out, fmt.Sprintf("%q", plan[i].Label.ValueString()))
		}
	}
	return out
}

// tabItemOps is the set of API calls that turns prior tab items into the
// planned ones. Items are matched by position: changed labels or icons are
// updated in place, extra planned items are appended, and leftover prior
// items are deleted along with their content.
type tabItemOps struct {
	Update []int // indexes into plan whose label or icon changed
	Append []int // indexes into plan to create
	Delete []int // indexes into prior to delete
}

func planTabItemOps(prior, plan []tabItemModel) tabItemOps {
	var ops tabItemOps
	for i := range plan {
		if i >= len(prior) {
			ops.Append = append(ops.Append, i)
			continue
		}
		if plan[i].Label.ValueString() != prior[i].Label.ValueString() ||
			plan[i].Icon.ValueString() != prior[i].Icon.ValueString() {
			ops.Update = append(ops.Update, i)
		}
	}
	for i := len(plan); i < len(prior); i++ {
		ops.Delete = append(ops.Delete, i)
	}
	return ops
}

// reconcileTabItems runs planTabItemOps and returns the block ID of every
// planned item, in order.
func reconcileTabItems(ctx context.Context, token, tabID string, prior, plan []tabItemModel) ([]string, error) {
	ops := planTabItemOps(prior, plan)
	ids := make([]string, len(plan))
	for i := range plan {
		if i < len(prior) {
			ids[i] = prior[i].ID.ValueString()
		}
	}

	for _, i := range ops.Update {
		if _, err := updateBlockRaw(ctx, token, ids[i], map[string]interface{}{
			"paragraph": tabItemBody(plan[i].spec()),
		}); err != nil {
			return nil, err
		}
	}

	if len(ops.Append) > 0 {
		children := make([]map[string]interface{}, 0, len(ops.Append))
		for _, i := range ops.Append {
			children = append(children, tabItemBlock(plan[i].spec()))
		}
		created, err := appendBlocksRaw(ctx, token, tabID, children, "")
		if err != nil {
			return nil, err
		}
		if len(created) != len(ops.Append) {
			return nil, fmt.Errorf("appended %d tab items but Notion returned %d", len(ops.Append), len(created))
		}
		for k, i := range ops.Append {
			ids[i] = normalizeID(created[k].ID)
		}
	}

	for _, i := range ops.Delete {
		if err := deleteBlockRaw(ctx, token, prior[i].ID.ValueString()); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func deleteBlockRaw(ctx context.Context, token, blockID string) error {
	_, err := doRawBlockRequest(ctx, http.MethodDelete, fmt.Sprintf("%s/blocks/%s", notionAPIBaseURL, blockID), token, nil)
	return err
}

// readRawBlockIntoState copies a raw block into the flat model. Attributes
// these types don't use are set to their schema defaults so an import plans
// cleanly. Notion reports an unset color as "default", which is kept as ""
// when that's what the config had, so the default doesn't read as drift.
func readRawBlockIntoState(b *rawBlock, state *BlockResourceModel) {
	priorColor := state.Color
	state.ID = types.StringValue(normalizeID(b.ID))
	state.Type = types.StringValue(b.Type)
	state.HasChildren = types.BoolValue(b.HasChildren)
	if p := b.parentID(); p != "" {
		state.ParentID = types.StringValue(p)
	}

	setStringDefault(&state.RichText)
	setStringDefault(&state.Color)
	setStringDefault(&state.Icon)
	setStringDefault(&state.Language)
	setStringDefault(&state.Caption)
	setStringDefault(&state.URL)
	setStringDefault(&state.Expression)
	if state.IsToggleable.IsNull() || state.IsToggleable.IsUnknown() {
		state.IsToggleable = types.BoolValue(false)
	}
	if state.Checked.IsNull() || state.Checked.IsUnknown() {
		state.Checked = types.BoolValue(false)
	}
	if b.Type != "tab" && (state.TabItems.IsUnknown()) {
		state.TabItems = types.ListNull(types.ObjectType{AttrTypes: tabItemAttrTypes})
	}

	switch b.Type {
	case "heading_4":
		if b.Heading4 == nil {
			return
		}
		setRichTextState(b.Heading4.RichText, state)
		state.Color = types.StringValue(normalizeBlockColor(b.Heading4.Color, priorColor))
		state.IsToggleable = types.BoolValue(b.Heading4.IsToggleable)
	case "callout":
		if b.Callout == nil {
			return
		}
		setRichTextState(b.Callout.RichText, state)
		state.Color = types.StringValue(normalizeBlockColor(b.Callout.Color, priorColor))
		// Same rule as pages: the emoji `icon` always reflects Notion, so a
		// changed or removed emoji shows as drift, and the other icon kinds
		// are only tracked when configured.
		state.setIconFields(state.iconFields().reconcile(b.Callout.icon()))
	case "tab":
		state.RichText = types.StringValue("")
	}
}

// keepPlannedIcon returns the icon attributes to save after a create or
// update. Terraform requires a known planned value to be saved unchanged,
// so the plan wins and the next refresh reports any difference. A value
// that was unknown at plan time takes what Notion returned.
func keepPlannedIcon(planned, returned iconFields) iconFields {
	pick := func(p, r types.String) types.String {
		if p.IsUnknown() {
			return r
		}
		return p
	}
	return iconFields{
		Emoji:         pick(planned.Emoji, returned.Emoji),
		Name:          pick(planned.Name, returned.Name),
		Color:         pick(planned.Color, returned.Color),
		URL:           pick(planned.URL, returned.URL),
		CustomEmojiID: pick(planned.CustomEmojiID, returned.CustomEmojiID),
	}
}

func setStringDefault(s *types.String) {
	if s.IsNull() || s.IsUnknown() {
		*s = types.StringValue("")
	}
}

// normalizeBlockColor keeps "" in state when Notion answers "default" for
// a color the config left empty.
func normalizeBlockColor(apiColor string, prior types.String) string {
	if apiColor == "default" && (prior.IsNull() || prior.IsUnknown() || prior.ValueString() == "") {
		return ""
	}
	return apiColor
}

// blockColorForUpdate is the color sent on an update. An empty color is
// sent as "default", since the SDK omits an empty color and Notion would
// then keep a color the config no longer sets.
func blockColorForUpdate(c types.String) string {
	if c.IsNull() || c.IsUnknown() || c.ValueString() == "" {
		return "default"
	}
	return c.ValueString()
}
