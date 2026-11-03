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
var rawBlockTypes = map[string]bool{"heading_4": true, "tab": true, "callout": true, "column_list": true}

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

// columnAttrTypes is the object type of one columns element.
var columnAttrTypes = map[string]attr.Type{
	"id":          types.StringType,
	"width_ratio": types.Float64Type,
}

var columnObjectType = types.ObjectType{AttrTypes: columnAttrTypes}

type columnModel struct {
	ID         types.String  `tfsdk:"id"`
	WidthRatio types.Float64 `tfsdk:"width_ratio"`
}

func columnsFromList(ctx context.Context, l types.List) ([]columnModel, diag.Diagnostics) {
	var cols []columnModel
	if l.IsNull() || l.IsUnknown() {
		return nil, nil
	}
	diags := l.ElementsAs(ctx, &cols, false)
	return cols, diags
}

func columnsToList(ctx context.Context, cols []columnModel) (types.List, diag.Diagnostics) {
	return types.ListValueFrom(ctx, columnObjectType, cols)
}

func (m columnModel) spec() columnSpec {
	if m.WidthRatio.IsNull() || m.WidthRatio.IsUnknown() {
		return columnSpec{}
	}
	v := m.WidthRatio.ValueFloat64()
	return columnSpec{WidthRatio: &v}
}

// widthRatioValue is a width_ratio from Notion as a state value.
func widthRatioValue(c *rawColumn) types.Float64 {
	if c == nil || c.WidthRatio == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*c.WidthRatio)
}

// sameRatio reports whether two ratios match closely enough that a float
// round trip through Notion isn't drift.
func sameRatio(a, b float64) bool {
	d := a - b
	return d < 1e-6 && d > -1e-6
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
	var cols []columnModel
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
			"callout": calloutBody(rt, plan.Color.ValueString(), plan.iconFields().request()),
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
	case "column_list":
		cols, diags = columnsFromList(ctx, plan.Columns)
		if diags.HasError() {
			return diags
		}
		specs := make([]columnSpec, len(cols))
		for i, c := range cols {
			specs[i] = c.spec()
		}
		block = columnListBlock(specs)
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

	if plan.Type.ValueString() == "column_list" {
		children, err := listBlockChildrenRaw(ctx, token, created.ID)
		found := columnChildren(children)
		switch {
		case err != nil:
			diags.AddError("Error reading new columns", err.Error())
		case len(found) != len(cols):
			diags.AddError("Error reading new columns",
				fmt.Sprintf("created %d columns but Notion returned %d", len(cols), len(found)))
		default:
			for i := range cols {
				cols[i].ID = types.StringValue(normalizeID(found[i].ID))
				if cols[i].WidthRatio.IsUnknown() {
					cols[i].WidthRatio = widthRatioValue(found[i].Column)
				}
			}
		}
		// Same as tabs, a failed read leaves nulls so the block is saved
		// and tainted instead of orphaned.
		for i := range cols {
			if cols[i].ID.IsUnknown() {
				cols[i].ID = types.StringNull()
			}
			if cols[i].WidthRatio.IsUnknown() {
				cols[i].WidthRatio = types.Float64Null()
			}
		}
		list, d := columnsToList(ctx, cols)
		diags.Append(d...)
		plan.Columns = list
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

	if b.Type == "column_list" {
		children, err := listBlockChildrenRaw(ctx, token, b.ID)
		if err != nil {
			diags.AddError("Error reading columns", err.Error())
			return true, diags
		}
		prior, d := columnsFromList(ctx, state.Columns)
		diags.Append(d...)
		list, d := columnsToList(ctx, columnsFromChildren(children, prior))
		diags.Append(d...)
		state.Columns = list
	}
	return true, diags
}

// columnChildren keeps the live column blocks among a column list's
// children, in Notion's order.
func columnChildren(children []rawBlock) []rawBlock {
	out := make([]rawBlock, 0, len(children))
	for _, c := range children {
		if c.Type == "column" && !c.InTrash && !c.Archived {
			out = append(out, c)
		}
	}
	return out
}

// columnsFromChildren turns a column list's children into columns. Only
// the columns are read, not what's inside them, so the placeholder
// paragraph each new column starts with never shows up as drift. A ratio
// within float noise of the prior state keeps the prior value.
func columnsFromChildren(children []rawBlock, prior []columnModel) []columnModel {
	found := columnChildren(children)
	cols := make([]columnModel, 0, len(found))
	for i, c := range found {
		ratio := widthRatioValue(c.Column)
		if i < len(prior) && !ratio.IsNull() && !prior[i].WidthRatio.IsNull() && !prior[i].WidthRatio.IsUnknown() &&
			sameRatio(prior[i].WidthRatio.ValueFloat64(), ratio.ValueFloat64()) {
			ratio = prior[i].WidthRatio
		}
		cols = append(cols, columnModel{ID: types.StringValue(normalizeID(c.ID)), WidthRatio: ratio})
	}
	return cols
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
		if attr := calloutIconRemoval(prior.iconFields(), icon); attr != "" {
			diags.AddAttributeError(path.Root(attr), "Notion can't remove a callout icon through the API",
				"The callout has an icon but the config sets none. Notion's API can set or change a callout icon but not remove it. "+
					"Remove it in the Notion app and then from the config, or configure an icon.")
			return diags
		}
		updated, err := updateBlockRaw(ctx, token, plan.ID.ValueString(), map[string]interface{}{
			"callout": calloutBody(rt, plan.Color.ValueString(), icon),
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

	case "column_list":
		// columns is optional on an existing list (an import from an
		// earlier version may not set it), and then it isn't managed.
		if plan.Columns.IsNull() || plan.Columns.IsUnknown() {
			plan.Columns = prior.Columns
			return diags
		}
		priorCols, d := columnsFromList(ctx, prior.Columns)
		diags.Append(d...)
		planCols, d := columnsFromList(ctx, plan.Columns)
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		got, err := reconcileColumns(ctx, token, plan.ID.ValueString(), priorCols, planCols)
		if err != nil {
			diags.AddError("Error updating columns", err.Error())
			return diags
		}
		for i := range planCols {
			planCols[i].ID = got[i].ID
			if planCols[i].WidthRatio.IsUnknown() {
				planCols[i].WidthRatio = got[i].WidthRatio
			}
		}
		fillUnknownRatios(ctx, token, plan.ID.ValueString(), planCols)
		list, d := columnsToList(ctx, planCols)
		diags.Append(d...)
		plan.Columns = list
		plan.HasChildren = types.BoolValue(true)
	}
	return diags
}

// columnOps is the set of API calls that turns prior columns into the
// planned ones, matched by position like tab items. A configured
// width_ratio that changed is updated in place, extra planned columns are
// appended, and leftover prior columns are deleted with their content.
type columnOps struct {
	Update []int // indexes into plan whose width_ratio changed
	Append []int // indexes into plan to create
	Delete []int // indexes into prior to delete
}

func planColumnOps(prior, plan []columnModel) columnOps {
	var ops columnOps
	for i := range plan {
		if i >= len(prior) {
			ops.Append = append(ops.Append, i)
			continue
		}
		want := plan[i].WidthRatio
		if want.IsNull() || want.IsUnknown() {
			continue
		}
		have := prior[i].WidthRatio
		if have.IsNull() || have.IsUnknown() || !sameRatio(have.ValueFloat64(), want.ValueFloat64()) {
			ops.Update = append(ops.Update, i)
		}
	}
	for i := len(plan); i < len(prior); i++ {
		ops.Delete = append(ops.Delete, i)
	}
	return ops
}

// reconcileColumns runs planColumnOps and returns every planned column with
// its ID. width_ratio is what Notion returned for an updated or appended
// column and unknown for the rest, and the caller only uses it where the
// plan left the ratio unknown.
func reconcileColumns(ctx context.Context, token, listID string, prior, plan []columnModel) ([]columnModel, error) {
	ops := planColumnOps(prior, plan)
	out := make([]columnModel, len(plan))
	for i := range plan {
		out[i] = columnModel{ID: types.StringNull(), WidthRatio: types.Float64Unknown()}
		if i < len(prior) {
			out[i].ID = prior[i].ID
		}
	}

	for _, i := range ops.Update {
		updated, err := updateBlockRaw(ctx, token, out[i].ID.ValueString(), map[string]interface{}{
			"column": map[string]interface{}{"width_ratio": plan[i].WidthRatio.ValueFloat64()},
		})
		if err != nil {
			return nil, err
		}
		out[i].WidthRatio = widthRatioValue(updated.Column)
	}

	if len(ops.Append) > 0 {
		children := make([]map[string]interface{}, 0, len(ops.Append))
		for _, i := range ops.Append {
			children = append(children, columnBlock(plan[i].spec()))
		}
		// Appending has no idempotency key, so a resend could add the
		// columns twice.
		created, err := appendBlocksRaw(withNoReplay(ctx), token, listID, children, "")
		if err != nil {
			return nil, err
		}
		if len(created) != len(ops.Append) {
			return nil, fmt.Errorf("appended %d columns but Notion returned %d", len(ops.Append), len(created))
		}
		for k, i := range ops.Append {
			out[i].ID = types.StringValue(normalizeID(created[k].ID))
			out[i].WidthRatio = widthRatioValue(created[k].Column)
		}
	}

	for _, i := range ops.Delete {
		if err := deleteBlockRaw(ctx, token, prior[i].ID.ValueString()); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// fillUnknownRatios resolves width_ratio values the plan left unknown,
// which happens for unconfigured ratios when something else changed. Notion
// may rebalance widths when columns are added or removed, so it reads them
// back. It's best effort. On a failed read it saves null and the next
// refresh picks up the real value, which an unconfigured ratio accepts
// without planning a change.
func fillUnknownRatios(ctx context.Context, token, listID string, cols []columnModel) {
	unknown := false
	for _, c := range cols {
		if c.WidthRatio.IsUnknown() {
			unknown = true
		}
	}
	if !unknown {
		return
	}
	byID := map[string]*rawColumn{}
	if children, err := listBlockChildrenRaw(ctx, token, listID); err == nil {
		for _, f := range columnChildren(children) {
			byID[normalizeID(f.ID)] = f.Column
		}
	}
	for i := range cols {
		if cols[i].WidthRatio.IsUnknown() {
			cols[i].WidthRatio = widthRatioValue(byID[cols[i].ID.ValueString()])
		}
	}
}

// calloutIconRemoval returns the icon attribute the prior state had set
// when the plan configures no icon, or "" when nothing would be removed.
// Update a block takes only an icon object, so this is checked before any
// request. The prior state only holds an icon that was configured or
// imported, since reconcileCalloutIcon leaves an unconfigured one out.
func calloutIconRemoval(prior iconFields, planned map[string]interface{}) string {
	if planned != nil {
		return ""
	}
	switch {
	case setString(prior.Emoji):
		return "icon"
	case setString(prior.Name):
		return "icon_name"
	case setString(prior.URL):
		return "icon_url"
	case setString(prior.CustomEmojiID):
		return "icon_custom_emoji_id"
	}
	return ""
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
	priorIcons := state.iconFields()
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
	if b.Type != "column_list" && state.Columns.IsUnknown() {
		state.Columns = types.ListNull(columnObjectType)
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
		state.setIconFields(reconcileCalloutIcon(priorIcons, b.Callout.icon()))
	case "tab", "column_list":
		state.RichText = types.StringValue("")
	}
}

// reconcileCalloutIcon maps a callout's icon onto the prior state. Each icon
// kind is tracked only once it's configured, so a callout with no icon in
// the config never plans a change because of its icon. That matters because
// the Notion app gives new callouts a 💡, and the API can't remove an icon,
// so tracking it would plan an update that can only be refused. Once `icon`
// holds an emoji, a changed or removed emoji shows as drift, as on pages.
// An import has no prior state (`icon` is null), so it takes Notion's emoji
// and the user writes that in the config.
func reconcileCalloutIcon(prior iconFields, p parsedIcon) iconFields {
	out := prior.reconcile(p)
	untracked := !prior.Emoji.IsNull() && !prior.Emoji.IsUnknown() && prior.Emoji.ValueString() == "" && !prior.managed()
	if untracked {
		out.Emoji = types.StringValue("")
	}
	return out
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
