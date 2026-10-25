package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource           = &LegalHoldsDataSource{}
	_ datasource.DataSource           = &LegalHoldPagesDataSource{}
	_ action.ActionWithConfigure      = &LegalHoldExportAction{}
	_ action.ActionWithValidateConfig = &LegalHoldExportAction{}
)

// ---- notion_legal_holds ----

type LegalHoldsDataSource struct{ adminDataSourceBase }

type LegalHoldsDataSourceModel struct {
	UserID     types.String     `tfsdk:"user_id"`
	LegalHolds []LegalHoldEntry `tfsdk:"legal_holds"`
}

type LegalHoldEntry struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Description          types.String `tfsdk:"description"`
	Status               types.String `tfsdk:"status"`
	StartDate            types.Int64  `tfsdk:"start_date"`
	EndDate              types.Int64  `tfsdk:"end_date"`
	CreatedBy            types.String `tfsdk:"created_by"`
	CreatedTime          types.Int64  `tfsdk:"created_time"`
	UserInteractionTypes []string     `tfsdk:"user_interaction_types"`
	UserCount            types.Int64  `tfsdk:"user_count"`
	WorkspaceCount       types.Int64  `tfsdk:"workspace_count"`
}

func NewLegalHoldsDataSource() datasource.DataSource { return &LegalHoldsDataSource{} }

func (d *LegalHoldsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_legal_holds"
}

func (d *LegalHoldsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the organization's legal holds, active and released, through the Admin API. Requires admin_token with the legal-hold:read scope. " +
			"The API doesn't guarantee an order.",
		Attributes: map[string]schema.Attribute{
			"user_id": schema.StringAttribute{
				Description: "Only list holds that include this user.",
				Optional:    true,
			},
			"legal_holds": schema.ListNestedAttribute{
				Description: "Legal holds.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                     computedString("The legal hold ID."),
						"name":                   computedString("The hold's name."),
						"description":            computedString("The hold's description, or null."),
						"status":                 computedString("active or released."),
						"start_date":             schema.Int64Attribute{Description: "Start, as epoch milliseconds.", Computed: true},
						"end_date":               schema.Int64Attribute{Description: "End, as epoch milliseconds, or null.", Computed: true},
						"created_by":             computedString("ID of the creator."),
						"created_time":           schema.Int64Attribute{Description: "Creation time, as epoch milliseconds.", Computed: true},
						"user_interaction_types": schema.ListAttribute{Description: "Preserved activity types.", Computed: true, ElementType: types.StringType},
						"user_count":             schema.Int64Attribute{Description: "Number of users under the hold.", Computed: true},
						"workspace_count":        schema.Int64Attribute{Description: "Number of workspaces the hold covers.", Computed: true},
					},
				},
			},
		},
	}
}

func (d *LegalHoldsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state LegalHoldsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}
	holds, err := admin.ListLegalHolds(ctx, state.UserID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error listing legal holds", err.Error())
		return
	}
	state.LegalHolds = make([]LegalHoldEntry, 0, len(holds))
	for _, h := range holds {
		e := LegalHoldEntry{
			ID:                   types.StringValue(h.ID),
			Name:                 types.StringValue(h.Name),
			Description:          optionalString(types.StringNull(), h.Description),
			Status:               types.StringValue(h.Status),
			StartDate:            types.Int64Value(int64(h.StartDate)),
			EndDate:              types.Int64Null(),
			CreatedBy:            types.StringValue(h.CreatedBy),
			CreatedTime:          types.Int64Value(int64(h.CreatedTime)),
			UserInteractionTypes: append([]string{}, h.UserInteractionType...),
			UserCount:            types.Int64Value(int64(h.Users.Total)),
			WorkspaceCount:       types.Int64Value(int64(h.Workspaces.Total)),
		}
		if h.EndDate != nil {
			e.EndDate = types.Int64Value(int64(*h.EndDate))
		}
		state.LegalHolds = append(state.LegalHolds, e)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- notion_legal_hold_pages ----

type LegalHoldPagesDataSource struct{ adminDataSourceBase }

type LegalHoldPagesDataSourceModel struct {
	LegalHoldID types.String `tfsdk:"legal_hold_id"`
	WorkspaceID types.String `tfsdk:"workspace_id"`
	PageIDs     []string     `tfsdk:"page_ids"`
}

func NewLegalHoldPagesDataSource() datasource.DataSource { return &LegalHoldPagesDataSource{} }

func (d *LegalHoldPagesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_legal_hold_pages"
}

func (d *LegalHoldPagesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the pages a legal hold preserves in one workspace, through the Admin API. Requires admin_token with the legal-hold:read scope. " +
			"This can be a large list.",
		Attributes: map[string]schema.Attribute{
			"legal_hold_id": schema.StringAttribute{Description: "The legal hold.", Required: true},
			"workspace_id":  schema.StringAttribute{Description: "The workspace to list held pages in.", Required: true},
			"page_ids":      schema.ListAttribute{Description: "IDs of held pages.", Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *LegalHoldPagesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state LegalHoldPagesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}
	pages, err := admin.ListLegalHoldPages(ctx, state.LegalHoldID.ValueString(), state.WorkspaceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error listing legal hold pages", err.Error())
		return
	}
	state.PageIDs = append([]string{}, pages...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- action notion_legal_hold_export ----

type LegalHoldExportAction struct{ adminActionBase }

type LegalHoldExportModel struct {
	LegalHoldID       types.String `tfsdk:"legal_hold_id"`
	RequestingUserID  types.String `tfsdk:"requesting_user_id"`
	WorkspaceID       types.String `tfsdk:"workspace_id"`
	LegalHoldExportID types.String `tfsdk:"legal_hold_export_id"`
}

func NewLegalHoldExportAction() action.Action { return &LegalHoldExportAction{} }

func (a *LegalHoldExportAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_legal_hold_export"
}

func (a *LegalHoldExportAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		Description: "Starts an export of a legal hold's content for one workspace, or continues an existing export, through the Admin API. " +
			"Requires admin_token with the legal-hold:export scope. The export ID is reported as a progress message. " +
			"Set exactly one of workspace_id (start) or legal_hold_export_id (continue).",
		Attributes: map[string]actionschema.Attribute{
			"legal_hold_id":        actionschema.StringAttribute{Description: "The legal hold to export.", Required: true},
			"requesting_user_id":   actionschema.StringAttribute{Description: "The user the export is requested on behalf of.", Required: true},
			"workspace_id":         actionschema.StringAttribute{Description: "Workspace to export. Starts a new export.", Optional: true},
			"legal_hold_export_id": actionschema.StringAttribute{Description: "An export returned by an earlier run. Continues that export.", Optional: true},
		},
	}
}

func (a *LegalHoldExportAction) ValidateConfig(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var cfg LegalHoldExportModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.WorkspaceID.IsUnknown() || cfg.LegalHoldExportID.IsUnknown() {
		return
	}
	if cfg.WorkspaceID.IsNull() == cfg.LegalHoldExportID.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("workspace_id"), "Choose one export target",
			"Set exactly one of workspace_id (start a new export) or legal_hold_export_id (continue an existing one).")
	}
}

func (a *LegalHoldExportAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg LegalHoldExportModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin := a.admin(resp)
	if admin == nil {
		return
	}
	id, err := admin.ExportLegalHold(ctx, cfg.LegalHoldID.ValueString(), cfg.RequestingUserID.ValueString(),
		cfg.WorkspaceID.ValueString(), cfg.LegalHoldExportID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error exporting legal hold", err.Error())
		return
	}
	progress(resp, "Legal hold export %s requested for legal hold %s", id, cfg.LegalHoldID.ValueString())
}
