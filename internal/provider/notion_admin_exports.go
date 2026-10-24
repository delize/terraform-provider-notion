package provider

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Workspace export endpoints of the Admin API. Scope: workspace:export.

type workspaceExportRequest struct {
	ExportType               string   `json:"export_type"`
	OnBehalfOfUserEmail      string   `json:"on_behalf_of_user_email"`
	CollectionViewExportType string   `json:"collection_view_export_type,omitempty"`
	FlattenExportFiletree    *bool    `json:"flatten_export_filetree,omitempty"`
	IncludeComments          *bool    `json:"include_comments,omitempty"`
	IncludeContents          string   `json:"include_contents,omitempty"`
	Locale                   string   `json:"locale,omitempty"`
	TeamspaceIDs             []string `json:"teamspace_ids,omitempty"`
	TimeZone                 string   `json:"time_zone,omitempty"`
}

type adminWorkspaceExport struct {
	ExportJobID    string   `json:"export_job_id"`
	Status         string   `json:"status"`
	ExportFormat   string   `json:"export_format"`
	ExportType     string   `json:"export_type"`
	PagesExported  float64  `json:"pages_exported"`
	CreatedTime    *float64 `json:"created_time"`
	LastEditedTime *float64 `json:"last_edited_time"`
	DownloadURLs   []string `json:"download_urls"`
	URLExpiresTime *float64 `json:"url_expires_time"`
	ErrorType      string   `json:"error_type"`
	ErrorMessage   string   `json:"error_message"`
}

func (c *adminClient) EnqueueWorkspaceExport(ctx context.Context, spaceID string, req workspaceExportRequest) (*adminWorkspaceExport, error) {
	var out adminWorkspaceExport
	if err := c.do(ctx, http.MethodPost, adminSpacePath(spaceID, "exports"), nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *adminClient) GetWorkspaceExport(ctx context.Context, spaceID, jobID string) (*adminWorkspaceExport, error) {
	var out adminWorkspaceExport
	if err := c.do(ctx, http.MethodGet, adminSpacePath(spaceID, "exports", jobID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

var (
	_ action.ActionWithConfigure      = &WorkspaceExportAction{}
	_ action.ActionWithValidateConfig = &WorkspaceExportAction{}
	_ datasource.DataSource           = &WorkspaceExportStatusDataSource{}
)

// ---- action notion_enqueue_workspace_export ----

type WorkspaceExportAction struct{ adminActionBase }

type WorkspaceExportModel struct {
	WorkspaceID              types.String `tfsdk:"workspace_id"`
	ExportType               types.String `tfsdk:"export_type"`
	OnBehalfOfUserEmail      types.String `tfsdk:"on_behalf_of_user_email"`
	CollectionViewExportType types.String `tfsdk:"collection_view_export_type"`
	FlattenExportFiletree    types.Bool   `tfsdk:"flatten_export_filetree"`
	IncludeComments          types.Bool   `tfsdk:"include_comments"`
	IncludeContents          types.String `tfsdk:"include_contents"`
	Locale                   types.String `tfsdk:"locale"`
	TeamspaceIDs             []string     `tfsdk:"teamspace_ids"`
	TimeZone                 types.String `tfsdk:"time_zone"`
}

func NewWorkspaceExportAction() action.Action { return &WorkspaceExportAction{} }

func (a *WorkspaceExportAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_enqueue_workspace_export"
}

func (a *WorkspaceExportAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	opt := func(desc string) actionschema.StringAttribute { return actionschema.StringAttribute{Description: desc, Optional: true} }
	resp.Schema = actionschema.Schema{
		Description: "Starts an export of a workspace through the Admin API. The export job ID is reported as a progress message. " +
			"Read its progress and download URLs with the notion_workspace_export_status data source. Requires admin_token with the workspace:export scope.",
		Attributes: map[string]actionschema.Attribute{
			"workspace_id":                actionschema.StringAttribute{Description: "The workspace to export.", Required: true},
			"export_type":                 actionschema.StringAttribute{Description: "html or markdown.", Required: true},
			"on_behalf_of_user_email":     actionschema.StringAttribute{Description: "Email of the user the export runs as.", Required: true},
			"collection_view_export_type": opt("all or currentView: which database views to export."),
			"flatten_export_filetree":     actionschema.BoolAttribute{Description: "Flatten the exported file tree.", Optional: true},
			"include_comments":            actionschema.BoolAttribute{Description: "Include comments.", Optional: true},
			"include_contents":            opt("everything or no_files."),
			"locale":                      opt("Locale for the export."),
			"teamspace_ids":               actionschema.ListAttribute{Description: "Only export these teamspaces.", Optional: true, ElementType: types.StringType},
			"time_zone":                   opt("Time zone for dates in the export."),
		},
	}
}

func (a *WorkspaceExportAction) ValidateConfig(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var cfg WorkspaceExportModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	check := func(attr string, v types.String, allowed ...string) {
		if v.IsNull() || v.IsUnknown() || slices.Contains(allowed, v.ValueString()) {
			return
		}
		resp.Diagnostics.AddAttributeError(path.Root(attr), "Invalid value", fmt.Sprintf("%q is not valid for %s. Use one of %v.", v.ValueString(), attr, allowed))
	}
	check("export_type", cfg.ExportType, "html", "markdown")
	check("collection_view_export_type", cfg.CollectionViewExportType, "all", "currentView")
	check("include_contents", cfg.IncludeContents, "everything", "no_files")
}

func boolPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

func (a *WorkspaceExportAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg WorkspaceExportModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin := a.admin(resp)
	if admin == nil {
		return
	}
	job, err := admin.EnqueueWorkspaceExport(ctx, cfg.WorkspaceID.ValueString(), workspaceExportRequest{
		ExportType:               cfg.ExportType.ValueString(),
		OnBehalfOfUserEmail:      cfg.OnBehalfOfUserEmail.ValueString(),
		CollectionViewExportType: cfg.CollectionViewExportType.ValueString(),
		FlattenExportFiletree:    boolPtr(cfg.FlattenExportFiletree),
		IncludeComments:          boolPtr(cfg.IncludeComments),
		IncludeContents:          cfg.IncludeContents.ValueString(),
		Locale:                   cfg.Locale.ValueString(),
		TeamspaceIDs:             cfg.TeamspaceIDs,
		TimeZone:                 cfg.TimeZone.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error enqueueing workspace export", err.Error())
		return
	}
	progress(resp, "Workspace export %s enqueued (status %s). Read it with data.notion_workspace_export_status.", job.ExportJobID, job.Status)
}

// ---- notion_workspace_export_status ----

type WorkspaceExportStatusDataSource struct{ adminDataSourceBase }

type WorkspaceExportStatusModel struct {
	WorkspaceID    types.String `tfsdk:"workspace_id"`
	ExportJobID    types.String `tfsdk:"export_job_id"`
	Status         types.String `tfsdk:"status"`
	ExportFormat   types.String `tfsdk:"export_format"`
	ExportType     types.String `tfsdk:"export_type"`
	PagesExported  types.Int64  `tfsdk:"pages_exported"`
	CreatedTime    types.Int64  `tfsdk:"created_time"`
	LastEditedTime types.Int64  `tfsdk:"last_edited_time"`
	DownloadURLs   types.List   `tfsdk:"download_urls"`
	URLExpiresTime types.Int64  `tfsdk:"url_expires_time"`
	ErrorType      types.String `tfsdk:"error_type"`
	ErrorMessage   types.String `tfsdk:"error_message"`
}

func NewWorkspaceExportStatusDataSource() datasource.DataSource {
	return &WorkspaceExportStatusDataSource{}
}

func (d *WorkspaceExportStatusDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workspace_export_status"
}

func (d *WorkspaceExportStatusDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	str := func(desc string) dsschema.StringAttribute { return dsschema.StringAttribute{Description: desc, Computed: true} }
	num := func(desc string) dsschema.Int64Attribute { return dsschema.Int64Attribute{Description: desc, Computed: true} }
	resp.Schema = dsschema.Schema{
		Description: "Reads the status of a workspace export started with notion_enqueue_workspace_export, through the Admin API. Requires admin_token with the workspace:export scope.",
		Attributes: map[string]dsschema.Attribute{
			"workspace_id":     dsschema.StringAttribute{Description: "The exported workspace.", Required: true},
			"export_job_id":    dsschema.StringAttribute{Description: "The job ID the enqueue action reported.", Required: true},
			"status":           str("in_progress, completed, url_generation_failed, or failed."),
			"export_format":    str("html, markdown, or pdf."),
			"export_type":      str("space, teamspace, or page."),
			"pages_exported":   num("Pages exported so far."),
			"created_time":     num("Job creation, epoch milliseconds."),
			"last_edited_time": num("Last job update, epoch milliseconds."),
			"download_urls": dsschema.ListAttribute{
				Description: "Short-lived, bot-scoped download URLs, present only when status is completed. Marked sensitive because anyone with a URL can download the export until it expires.",
				Computed:    true, Sensitive: true, ElementType: types.StringType,
			},
			"url_expires_time": num("When the download URLs expire, epoch milliseconds."),
			"error_type":       str("Why the export failed, or null."),
			"error_message":    str("Failure details, or null."),
		},
	}
}

func (d *WorkspaceExportStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state WorkspaceExportStatusModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	admin, err := adminClientFor(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return
	}
	job, err := admin.GetWorkspaceExport(ctx, state.WorkspaceID.ValueString(), state.ExportJobID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading workspace export", err.Error())
		return
	}
	state.Status = types.StringValue(job.Status)
	state.ExportFormat = types.StringValue(job.ExportFormat)
	state.ExportType = types.StringValue(job.ExportType)
	state.PagesExported = types.Int64Value(int64(job.PagesExported))
	state.CreatedTime = int64FromPtr(job.CreatedTime)
	state.LastEditedTime = int64FromPtr(job.LastEditedTime)
	urls, diags := types.ListValueFrom(ctx, types.StringType, append([]string{}, job.DownloadURLs...))
	resp.Diagnostics.Append(diags...)
	state.DownloadURLs = urls
	state.URLExpiresTime = int64FromPtr(job.URLExpiresTime)
	state.ErrorType = nullableString(job.ErrorType)
	state.ErrorMessage = nullableString(job.ErrorMessage)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
