package provider

import (
	"context"
	"encoding/json"
	"fmt"

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

	name := plan.Name.ValueString()
	payload := viewUpdate{Name: &name}
	if err := unpackViewJSONUpdate(&payload, &plan); err != nil {
		resp.Diagnostics.AddError("Invalid view JSON attribute", err.Error())
		return
	}

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
	if v.DataSourceID != "" {
		state.DataSourceID = types.StringValue(v.DataSourceID)
	}
	state.Filter = jsonRawToTFString(v.Filter)
	state.Sorts = jsonRawToTFString(v.Sorts)
	state.QuickFilters = jsonRawToTFString(v.QuickFilters)
	state.Configuration = jsonRawToTFString(v.Configuration)
}

func jsonRawToTFString(raw json.RawMessage) types.String {
	if len(raw) == 0 || string(raw) == "null" {
		return types.StringValue("")
	}
	return types.StringValue(string(raw))
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
