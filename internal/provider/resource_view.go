package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// notion_view manages a Notion database view via the 2026-03-19 /v1/views
// endpoints. Filter, sorts, quick_filters, and configuration are accepted as
// raw JSON strings — they accept the same shape as the data source query API
// and modeling every nested field as TF attributes would balloon this resource
// without buying type safety beyond "is this valid JSON".

var (
	_ resource.Resource                = &ViewResource{}
	_ resource.ResourceWithImportState = &ViewResource{}
)

type ViewResource struct {
	client *notionapi.Client
}

type ViewResourceModel struct {
	ID            types.String `tfsdk:"id"`
	DatabaseID    types.String `tfsdk:"database_id"`
	ParentViewID  types.String `tfsdk:"parent_view_id"`
	DataSourceID  types.String `tfsdk:"data_source_id"`
	Name          types.String `tfsdk:"name"`
	Type          types.String `tfsdk:"type"`
	Filter        types.String `tfsdk:"filter"`
	Sorts         types.String `tfsdk:"sorts"`
	QuickFilters  types.String `tfsdk:"quick_filters"`
	Configuration types.String `tfsdk:"configuration"`
	URL           types.String `tfsdk:"url"`
}

func NewViewResource() resource.Resource {
	return &ViewResource{}
}

func (r *ViewResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_view"
}

func (r *ViewResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Notion database view via the 2026-03-19 `/v1/views` endpoints. " +
			"View types include `table`, `board`, `list`, `calendar`, `timeline`, `gallery`, " +
			"`form`, `chart`, `map`, and `dashboard`. Filter, sorts, quick_filters, and configuration " +
			"accept raw JSON strings (use `jsonencode`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The view ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"database_id": schema.StringAttribute{
				Description: "ID of the database to create the view in. Mutually exclusive with `parent_view_id`. " +
					"Changing this forces a new resource.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"parent_view_id": schema.StringAttribute{
				Description: "ID of a dashboard view to add this view to as a widget. Mutually exclusive with " +
					"`database_id`. Changing this forces a new resource.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"data_source_id": schema.StringAttribute{
				Description: "ID of the data source this view is scoped to (required by the create endpoint). " +
					"Under the v2025-09-03 multi-source databases model the data source is distinct from the database. " +
					"Changing this forces a new resource.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Display name of the view.",
				Required:    true,
			},
			"type": schema.StringAttribute{
				Description: "View type. One of: table, board, list, calendar, timeline, gallery, form, chart, map, dashboard. " +
					"Changing this forces a new resource.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					ViewTypeValidator(),
				},
			},
			"filter": schema.StringAttribute{
				Description: "Filter as a JSON object string. Uses the same shape as the data source query filter.",
				Optional:    true,
				Computed:    true,
			},
			"sorts": schema.StringAttribute{
				Description: "Sorts as a JSON array string. Uses the same shape as the data source query sorts (max 100 entries).",
				Optional:    true,
				Computed:    true,
			},
			"quick_filters": schema.StringAttribute{
				Description: "Quick filters pinned in the view's filter bar, as a JSON object string.",
				Optional:    true,
				Computed:    true,
			},
			"configuration": schema.StringAttribute{
				Description: "View presentation configuration as a JSON object string. The `type` field inside " +
					"must match the view's `type` attribute.",
				Optional: true,
				Computed: true,
			},
			"url": schema.StringAttribute{
				Description: "Deep link to the view in Notion.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ViewResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*notionapi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *notionapi.Client, got: %T.", req.ProviderData))
		return
	}
	r.client = client
}

func (r *ViewResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ViewResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.DatabaseID.IsNull() == plan.ParentViewID.IsNull() {
		resp.Diagnostics.AddError(
			"database_id and parent_view_id are mutually exclusive",
			"Exactly one of database_id or parent_view_id must be set. database_id creates a view directly "+
				"on a database; parent_view_id creates a widget inside an existing dashboard view.",
		)
		return
	}

	payload := viewCreate{
		DatabaseID:   plan.DatabaseID.ValueString(),
		ViewID:       plan.ParentViewID.ValueString(),
		DataSourceID: plan.DataSourceID.ValueString(),
		Name:         plan.Name.ValueString(),
		Type:         plan.Type.ValueString(),
	}
	if err := unpackViewJSON(&payload, &plan); err != nil {
		resp.Diagnostics.AddError("Invalid view JSON attribute", err.Error())
		return
	}

	token, err := tokenForClient(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error creating view", err.Error())
		return
	}

	v, err := createView(ctx, token, payload)
	if err != nil {
		resp.Diagnostics.AddError("Error creating view", err.Error())
		return
	}

	r.viewToState(v, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ViewResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ViewResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, err := tokenForClient(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error reading view", err.Error())
		return
	}

	v, err := getView(ctx, token, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading view", err.Error())
		return
	}
	if v == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	r.viewToState(v, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ViewResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ViewResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var prior ViewResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	payload := viewUpdate{Name: &name}
	if err := unpackViewJSONUpdate(&payload, &plan); err != nil {
		resp.Diagnostics.AddError("Invalid view JSON attribute", err.Error())
		return
	}
	qf, err := clearRemovedQuickFilters(payload.QuickFilters, prior.QuickFilters)
	if err != nil {
		resp.Diagnostics.AddError("Invalid view JSON attribute", err.Error())
		return
	}
	payload.QuickFilters = qf

	token, err := tokenForClient(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error updating view", err.Error())
		return
	}

	v, err := updateView(ctx, token, plan.ID.ValueString(), payload)
	if err != nil {
		resp.Diagnostics.AddError("Error updating view", err.Error())
		return
	}

	keepPriorWhereUnknown(&plan, prior)
	r.viewToState(v, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ViewResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ViewResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, err := tokenForClient(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting view", err.Error())
		return
	}

	if err := deleteView(ctx, token, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting view", err.Error())
		return
	}
}

func (r *ViewResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *ViewResource) viewToState(v *viewObject, state *ViewResourceModel) {
	state.ID = types.StringValue(normalizeID(v.ID))
	state.Name = types.StringValue(v.Name)
	state.Type = types.StringValue(v.Type)
	state.URL = types.StringValue(v.URL)
	// Import starts with only the view ID, so database_id comes from the
	// view's parent. Same hyphen handling as data_source_id below.
	if db := v.Parent.DatabaseID; db != "" && normalizeID(db) != normalizeID(state.DatabaseID.ValueString()) {
		state.DatabaseID = types.StringValue(normalizeID(db))
	}
	// Notion returns the ID with hyphens. Keep the form already in state
	// when it's the same ID, so a hyphenless config doesn't read as changed.
	if v.DataSourceID != "" && normalizeID(v.DataSourceID) != normalizeID(state.DataSourceID.ValueString()) {
		state.DataSourceID = types.StringValue(normalizeID(v.DataSourceID))
	}
	// Keep the configured JSON when Notion's copy means the same thing, so
	// key order, whitespace, and merged configuration fields don't fail the
	// apply or plan a diff. See viewJSONReadback.
	state.Filter = viewJSONReadback(state.Filter, v.Filter, false)
	state.Sorts = viewJSONReadback(state.Sorts, v.Sorts, false)
	state.QuickFilters = viewJSONReadback(state.QuickFilters, v.QuickFilters, false)
	state.Configuration = viewJSONReadback(state.Configuration, v.Configuration, true)
}

// keepPriorWhereUnknown fills JSON attributes the plan left unknown (not in
// the config, and the resource has other changes) with the prior state, so
// the readback compares against what's stored and keeps that string when
// Notion's value is unchanged.
func keepPriorWhereUnknown(plan *ViewResourceModel, prior ViewResourceModel) {
	for _, p := range []struct{ plan, prior *types.String }{
		{&plan.Filter, &prior.Filter},
		{&plan.Sorts, &prior.Sorts},
		{&plan.QuickFilters, &prior.QuickFilters},
		{&plan.Configuration, &prior.Configuration},
	} {
		if p.plan.IsUnknown() {
			*p.plan = *p.prior
		}
	}
}

// clearRemovedQuickFilters adds a null for every quick filter the prior
// state has and the plan dropped. The views guide says an update keeps
// quick filters it doesn't name and removes one set to null, so without
// this a removed quick filter would stay in Notion.
//
// Keys can be property names or IDs, and Notion returns IDs. A state that
// holds Notion's copy can name a property by ID while the config names it,
// so nulling the ID could remove the filter the config sets. To stay safe,
// nothing is nulled when the plan adds a key the prior state doesn't have.
func clearRemovedQuickFilters(planned json.RawMessage, prior types.String) (json.RawMessage, error) {
	if len(planned) == 0 || prior.IsNull() || prior.IsUnknown() || prior.ValueString() == "" {
		return planned, nil
	}
	var plan, old map[string]interface{}
	if json.Unmarshal(planned, &plan) != nil || json.Unmarshal([]byte(prior.ValueString()), &old) != nil {
		return planned, nil
	}
	for k := range plan {
		if _, ok := old[k]; !ok {
			return planned, nil
		}
	}
	changed := false
	for k := range old {
		if _, ok := plan[k]; !ok {
			plan[k] = nil
			changed = true
		}
	}
	if !changed {
		return planned, nil
	}
	return json.Marshal(plan)
}

func unpackViewJSON(payload *viewCreate, plan *ViewResourceModel) error {
	var err error
	if payload.Filter, err = readPlanJSON(plan.Filter, "filter"); err != nil {
		return err
	}
	if payload.Sorts, err = readPlanJSON(plan.Sorts, "sorts"); err != nil {
		return err
	}
	if payload.QuickFilters, err = readPlanJSON(plan.QuickFilters, "quick_filters"); err != nil {
		return err
	}
	if payload.Configuration, err = readPlanJSON(plan.Configuration, "configuration"); err != nil {
		return err
	}
	return nil
}

func unpackViewJSONUpdate(payload *viewUpdate, plan *ViewResourceModel) error {
	var err error
	if payload.Filter, err = readPlanJSON(plan.Filter, "filter"); err != nil {
		return err
	}
	if payload.Sorts, err = readPlanJSON(plan.Sorts, "sorts"); err != nil {
		return err
	}
	if payload.QuickFilters, err = readPlanJSON(plan.QuickFilters, "quick_filters"); err != nil {
		return err
	}
	if payload.Configuration, err = readPlanJSON(plan.Configuration, "configuration"); err != nil {
		return err
	}
	return nil
}

func readPlanJSON(v types.String, field string) (json.RawMessage, error) {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return nil, nil
	}
	raw := json.RawMessage(v.ValueString())
	var probe interface{}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", field, err)
	}
	return raw, nil
}

// viewJSONReadback returns the string to store for one of notion_view's
// JSON attributes once Notion has returned raw for it. Notion returns null
// for a filter, sorts, quick filters, or configuration that isn't set, which
// is stored as "" as before.
//
// Two values match when they decode to the same JSON, so key order and
// whitespace don't matter. With mergedKeys set (configuration), keys that
// only Notion's value has are ignored at the top level. The views guide says
// configuration updates are a shallow merge that preserves omitted fields,
// so Notion's configuration can carry top-level fields the config never
// set. Nested values, and every other attribute, must match exactly. Notion
// documents no other defaults it adds.
func viewJSONReadback(prior types.String, raw json.RawMessage, mergedKeys bool) types.String {
	if len(raw) == 0 || string(raw) == "null" {
		if !prior.IsNull() && !prior.IsUnknown() && isEmptyJSONValue(prior.ValueString()) {
			return prior
		}
		return types.StringValue("")
	}
	var got interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		return types.StringValue(string(raw))
	}
	if !prior.IsNull() && !prior.IsUnknown() && prior.ValueString() != "" {
		var want interface{}
		if err := json.Unmarshal([]byte(prior.ValueString()), &want); err == nil && viewJSONEqual(want, got, mergedKeys) {
			return prior
		}
	}
	b, err := json.Marshal(got)
	if err != nil {
		return types.StringValue(string(raw))
	}
	return types.StringValue(string(b))
}

// isEmptyJSONValue reports whether s is null, [] or {}. A config of
// jsonencode(null) clears the value in Notion, and an empty sorts array
// leaves nothing set. Notion returns null for both.
func isEmptyJSONValue(s string) bool {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return false
	}
	switch c := v.(type) {
	case nil:
		return true
	case []interface{}:
		return len(c) == 0
	case map[string]interface{}:
		return len(c) == 0
	}
	return false
}

func viewJSONEqual(want, got interface{}, mergedKeys bool) bool {
	w, wok := want.(map[string]interface{})
	g, gok := got.(map[string]interface{})
	if !mergedKeys || !wok || !gok {
		return reflect.DeepEqual(want, got)
	}
	for k, wv := range w {
		gv, present := g[k]
		if wv == nil {
			if present && gv != nil {
				return false
			}
			continue
		}
		if !present || !reflect.DeepEqual(wv, gv) {
			return false
		}
	}
	return true
}
