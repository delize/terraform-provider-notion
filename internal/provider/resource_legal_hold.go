package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// notion_legal_hold manages an organization legal hold and its users.
// Destroying the resource leaves the hold in place unless release_on_destroy
// is true, because releasing a hold is a compliance decision that shouldn't
// follow from removing a block of Terraform.

const (
	// 2001-09-09 in epoch milliseconds. Smaller values are almost certainly
	// epoch seconds.
	minPlausibleEpochMillis = 1_000_000_000_000
	// 2022-05-01T00:00:00Z, the earliest activity a hold can preserve
	// according to the Help Center.
	legalHoldEarliestLookback = 1_651_363_200_000
)

var (
	_ resource.Resource                   = &LegalHoldResource{}
	_ resource.ResourceWithImportState    = &LegalHoldResource{}
	_ resource.ResourceWithValidateConfig = &LegalHoldResource{}
)

type LegalHoldResource struct {
	client *notionapi.Client
}

type LegalHoldModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Description          types.String `tfsdk:"description"`
	Icon                 types.String `tfsdk:"icon"`
	StartDate            types.Int64  `tfsdk:"start_date"`
	EndDate              types.Int64  `tfsdk:"end_date"`
	UserInteractionTypes types.Set    `tfsdk:"user_interaction_types"`
	UserIDs              types.Set    `tfsdk:"user_ids"`
	ReleaseOnDestroy     types.Bool   `tfsdk:"release_on_destroy"`
	Status               types.String `tfsdk:"status"`
	CreatedBy            types.String `tfsdk:"created_by"`
	CreatedTime          types.Int64  `tfsdk:"created_time"`
	WorkspaceIDs         types.Set    `tfsdk:"workspace_ids"`
}

func NewLegalHoldResource() resource.Resource {
	return &LegalHoldResource{}
}

func (r *LegalHoldResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_legal_hold"
}

func (r *LegalHoldResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "Manages an organization legal hold through the Notion Admin API. " +
			"Requires admin_token (Enterprise plan) with the legal-hold:read and legal-hold:write scopes, plus legal-hold:write-high-impact when release_on_destroy is true. " +
			"Destroying this resource leaves the hold active in Notion unless release_on_destroy is true.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the legal hold.", Computed: true, PlanModifiers: keep,
			},
			"name": schema.StringAttribute{
				Description: "The name of the legal hold. Changes in place.", Required: true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the legal hold. Changes in place, and removing it clears the description.", Optional: true,
			},
			"icon": schema.StringAttribute{
				Description: "The hold's icon, as a string the Admin API accepts. Changes in place.", Optional: true,
			},
			"start_date": schema.Int64Attribute{
				Description:   "When the hold starts, as epoch milliseconds. The API has no way to change it, so changing it forces a new hold.",
				Required:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"end_date": schema.Int64Attribute{
				Description:   "When the hold ends, as epoch milliseconds. Changing it forces a new hold.",
				Optional:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"user_interaction_types": schema.SetAttribute{
				Description:   "Which page activity the hold preserves: page.created, page.edited, page.viewed. Changing it forces a new hold.",
				Required:      true,
				ElementType:   types.StringType,
				PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"user_ids": schema.SetAttribute{
				Description: "Users under the hold. Adding or removing IDs updates the hold in place.",
				Required:    true,
				ElementType: types.StringType,
			},
			"release_on_destroy": schema.BoolAttribute{
				Description: "Release the hold in Notion when this resource is destroyed or replaced. Defaults to false, which only removes it from Terraform state. " +
					"Releasing needs the legal-hold:write-high-impact scope.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"status": schema.StringAttribute{
				Description: "active or released.", Computed: true,
			},
			"created_by": schema.StringAttribute{
				Description: "ID of the user or bot that created the hold.", Computed: true, PlanModifiers: keep,
			},
			"created_time": schema.Int64Attribute{
				Description:   "When the hold was created, as epoch milliseconds.",
				Computed:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"workspace_ids": schema.SetAttribute{
				Description: "Workspaces the hold covers, derived by Notion from its users.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *LegalHoldResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg LegalHoldModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !cfg.UserInteractionTypes.IsNull() && !cfg.UserInteractionTypes.IsUnknown() {
		var types_ []types.String
		resp.Diagnostics.Append(cfg.UserInteractionTypes.ElementsAs(ctx, &types_, false)...)
		for _, t := range types_ {
			if t.IsUnknown() || t.IsNull() {
				continue
			}
			if !slices.Contains(legalHoldInteractionTypes, t.ValueString()) {
				resp.Diagnostics.AddAttributeError(path.Root("user_interaction_types"), "Invalid interaction type",
					fmt.Sprintf("%q is not a legal hold interaction type. Use: %s.", t.ValueString(), strings.Join(legalHoldInteractionTypes, ", ")))
			}
		}
	}
	for _, d := range []struct {
		attr string
		v    types.Int64
	}{{"start_date", cfg.StartDate}, {"end_date", cfg.EndDate}} {
		if d.v.IsNull() || d.v.IsUnknown() {
			continue
		}
		if d.v.ValueInt64() < minPlausibleEpochMillis {
			resp.Diagnostics.AddAttributeError(path.Root(d.attr), "Date looks like seconds",
				fmt.Sprintf("%s = %d is before 2001 when read as epoch milliseconds. Multiply epoch seconds by 1000.", d.attr, d.v.ValueInt64()))
		}
	}
	if !cfg.StartDate.IsNull() && !cfg.StartDate.IsUnknown() && cfg.StartDate.ValueInt64() >= minPlausibleEpochMillis &&
		cfg.StartDate.ValueInt64() < legalHoldEarliestLookback {
		resp.Diagnostics.AddAttributeWarning(path.Root("start_date"), "Start date is before what a hold can preserve",
			"Notion's Help Center says a hold reaches back no further than the workspace's Enterprise upgrade date or May 2022, "+
				"so activity before that isn't preserved.")
	}
	if !cfg.StartDate.IsNull() && !cfg.StartDate.IsUnknown() && !cfg.EndDate.IsNull() && !cfg.EndDate.IsUnknown() &&
		cfg.EndDate.ValueInt64() < cfg.StartDate.ValueInt64() {
		resp.Diagnostics.AddAttributeError(path.Root("end_date"), "Invalid end date", "end_date is before start_date.")
	}
}

func (r *LegalHoldResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := providerDataFrom(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", err.Error())
		return
	}
	r.client = client
}

func (r *LegalHoldResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan LegalHoldModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}

	body := legalHoldCreate{
		Name:                plan.Name.ValueString(),
		Description:         plan.Description.ValueString(),
		Icon:                plan.Icon.ValueString(),
		StartDate:           plan.StartDate.ValueInt64(),
		UserIDs:             setStrings(ctx, plan.UserIDs, &resp.Diagnostics),
		UserInteractionType: setStrings(ctx, plan.UserInteractionTypes, &resp.Diagnostics),
	}
	if !plan.EndDate.IsNull() {
		v := plan.EndDate.ValueInt64()
		body.EndDate = &v
	}
	if resp.Diagnostics.HasError() {
		return
	}

	h, err := admin.CreateLegalHold(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Error creating legal hold", err.Error())
		return
	}
	plan.ID = types.StringValue(h.ID)
	r.refresh(ctx, admin, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// keepAppliedInt64 returns server unless an applied value exists and
// differs, in which case it keeps applied and warns. See refresh.
func keepAppliedInt64(applied, server types.Int64, attr string, diags *diag.Diagnostics) types.Int64 {
	if applied.IsNull() || applied.IsUnknown() || applied.Equal(server) {
		return server
	}
	diags.AddAttributeWarning(path.Root(attr), "Legal hold date differs in Notion",
		fmt.Sprintf("Terraform applied %s = %s but Notion reports %s. State keeps the applied value so no replacement hold is planned. "+
			"If Notion's value is about 1000 times smaller, the API expects seconds rather than milliseconds, so please report it.",
			attr, applied, server))
	return applied
}

// refresh reads the hold back into m. Config-owned fields keep the form the
// user wrote (for example hyphenless IDs). Server-owned fields are replaced.
// It returns false when the hold no longer exists.
func (r *LegalHoldResource) refresh(ctx context.Context, admin *adminClient, m *LegalHoldModel, diags *diag.Diagnostics) bool {
	h, err := admin.GetLegalHold(ctx, m.ID.ValueString())
	if isAdminNotFound(err) {
		return false
	}
	if err != nil {
		diags.AddError("Error reading legal hold", err.Error())
		return true
	}
	users, err := admin.ListLegalHoldUsers(ctx, h.ID)
	if err != nil {
		diags.AddError("Error listing legal hold users", err.Error())
		return true
	}
	workspaces, err := admin.ListLegalHoldWorkspaces(ctx, h.ID)
	if err != nil {
		diags.AddError("Error listing legal hold workspaces", err.Error())
		return true
	}

	m.Name = types.StringValue(h.Name)
	m.Description = optionalString(m.Description, h.Description)
	m.Icon = optionalString(m.Icon, h.Icon)
	// start_date, end_date, and user_interaction_types force a new hold
	// when they change. Notion doesn't document the date unit or whether it
	// normalizes these values, so a value that comes back different from
	// what was applied is kept as configured and reported as a warning,
	// instead of failing the apply or planning a second hold. Import (no
	// prior value) takes Notion's values.
	serverEnd := types.Int64Null()
	if h.EndDate != nil {
		serverEnd = types.Int64Value(int64(*h.EndDate))
	}
	m.StartDate = keepAppliedInt64(m.StartDate, types.Int64Value(int64(h.StartDate)), "start_date", diags)
	m.EndDate = keepAppliedInt64(m.EndDate, serverEnd, "end_date", diags)
	serverTypes := stringsSet(h.UserInteractionType, diags)
	if m.UserInteractionTypes.IsNull() || m.UserInteractionTypes.IsUnknown() {
		m.UserInteractionTypes = serverTypes
	} else if !m.UserInteractionTypes.Equal(serverTypes) {
		diags.AddAttributeWarning(path.Root("user_interaction_types"), "Legal hold activity types differ in Notion",
			fmt.Sprintf("Terraform applied %s but Notion reports %s. State keeps the applied value so no replacement hold is planned. "+
				"Check the hold in the organization console.", m.UserInteractionTypes, serverTypes))
	}
	prior := setStrings(ctx, m.UserIDs, diags)
	m.UserIDs = stringsSet(alignIDs(users, prior), diags)
	m.WorkspaceIDs = stringsSet(workspaces, diags)
	m.Status = types.StringValue(h.Status)
	m.CreatedBy = types.StringValue(h.CreatedBy)
	m.CreatedTime = types.Int64Value(int64(h.CreatedTime))
	return true
}

func (r *LegalHoldResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state LegalHoldModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}
	if !r.refresh(ctx, admin, &state, &resp.Diagnostics) {
		resp.State.RemoveResource(ctx)
		return
	}
	if state.ReleaseOnDestroy.IsNull() {
		state.ReleaseOnDestroy = types.BoolValue(false)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *LegalHoldResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state LegalHoldModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}
	id := state.ID.ValueString()
	plan.ID = state.ID

	var upd legalHoldUpdate
	changed := false
	if plan.Name.ValueString() != state.Name.ValueString() {
		v := plan.Name.ValueString()
		upd.Name, changed = &v, true
	}
	if !plan.Description.Equal(state.Description) {
		v := plan.Description.ValueString() // "" clears it
		upd.Description, changed = &v, true
	}
	if !plan.Icon.Equal(state.Icon) {
		v := plan.Icon.ValueString()
		upd.Icon, changed = &v, true
	}
	if changed {
		if _, err := admin.UpdateLegalHold(ctx, id, upd); err != nil {
			resp.Diagnostics.AddError("Error updating legal hold", err.Error())
			return
		}
	}

	want := setStrings(ctx, plan.UserIDs, &resp.Diagnostics)
	have := setStrings(ctx, state.UserIDs, &resp.Diagnostics)
	add, remove := diffIDs(want, have)
	if len(add) > 0 {
		if err := admin.AddLegalHoldUsers(ctx, id, add); err != nil {
			resp.Diagnostics.AddError("Error adding users to legal hold", err.Error())
			return
		}
	}
	for _, u := range remove {
		if err := admin.RemoveLegalHoldUser(ctx, id, u); err != nil && !isAdminNotFound(err) {
			resp.Diagnostics.AddError("Error removing user from legal hold", err.Error())
			return
		}
	}

	if !r.refresh(ctx, admin, &plan, &resp.Diagnostics) {
		resp.Diagnostics.AddError("Legal hold disappeared", "The legal hold was not found after updating it.")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *LegalHoldResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state LegalHoldModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !state.ReleaseOnDestroy.ValueBool() {
		resp.Diagnostics.AddWarning("Legal hold left in place",
			fmt.Sprintf("Legal hold %s was removed from Terraform state but is still %s in Notion. "+
				"Set release_on_destroy = true and apply before destroying to release it, or release it in the Notion organization console.",
				state.ID.ValueString(), state.Status.ValueString()))
		return
	}
	if state.Status.ValueString() == "released" {
		return
	}
	admin, err := adminClientFor(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}
	if _, err := admin.ReleaseLegalHold(ctx, state.ID.ValueString()); err != nil && !isAdminNotFound(err) {
		resp.Diagnostics.AddError("Error releasing legal hold", err.Error())
	}
}

func (r *LegalHoldResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---- shared helpers for Admin API resources ----

// setStrings returns a set's string elements, or nil for a null or unknown set.
func setStrings(ctx context.Context, s types.Set, diags *diag.Diagnostics) []string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var out []string
	diags.Append(s.ElementsAs(ctx, &out, false)...)
	return out
}

func stringsSet(values []string, diags *diag.Diagnostics) types.Set {
	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.StringValue(v))
	}
	s, d := types.SetValue(types.StringType, elems)
	diags.Append(d...)
	return s
}

// alignIDs returns server IDs, but in the form the user wrote them when an
// ID matches one in prior without regard to hyphens or case.
func alignIDs(server, prior []string) []string {
	byNorm := make(map[string]string, len(prior))
	for _, p := range prior {
		byNorm[strings.ToLower(normalizeID(p))] = p
	}
	out := make([]string, 0, len(server))
	for _, s := range server {
		if p, ok := byNorm[strings.ToLower(normalizeID(s))]; ok {
			out = append(out, p)
		} else {
			out = append(out, s)
		}
	}
	return out
}

// diffIDs compares two ID lists without regard to hyphens or case.
func diffIDs(want, have []string) (add, remove []string) {
	norm := func(s string) string { return strings.ToLower(normalizeID(s)) }
	haveSet := map[string]bool{}
	for _, h := range have {
		haveSet[norm(h)] = true
	}
	wantSet := map[string]bool{}
	for _, w := range want {
		wantSet[norm(w)] = true
		if !haveSet[norm(w)] {
			add = append(add, w)
		}
	}
	for _, h := range have {
		if !wantSet[norm(h)] {
			remove = append(remove, h)
		}
	}
	return add, remove
}

// optionalString maps an empty API value back to null when the prior value
// was null, so an unset optional attribute doesn't show a diff.
func optionalString(prior types.String, api string) types.String {
	if api == "" && prior.IsNull() {
		return types.StringNull()
	}
	return types.StringValue(api)
}
