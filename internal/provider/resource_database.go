package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var (
	_ resource.Resource                   = &DatabaseResource{}
	_ resource.ResourceWithImportState    = &DatabaseResource{}
	_ resource.ResourceWithValidateConfig = &DatabaseResource{}
)

type DatabaseResource struct {
	client *notionapi.Client
}

type DatabaseResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Parent           types.String `tfsdk:"parent"`
	Title            types.String `tfsdk:"title"`
	TitleColumnTitle types.String `tfsdk:"title_column_title"`
	TitleColumnID    types.String `tfsdk:"title_column_id"`
	URL              types.String `tfsdk:"url"`
	IsInline         types.Bool   `tfsdk:"is_inline"`
	Description      types.String `tfsdk:"description"`
	Icon             types.String `tfsdk:"icon"`
	IconName         types.String `tfsdk:"icon_name"`
	IconColor        types.String `tfsdk:"icon_color"`
	IconURL          types.String `tfsdk:"icon_url"`
	IconCustomEmoji  types.String `tfsdk:"icon_custom_emoji_id"`
	IsLocked         types.Bool   `tfsdk:"is_locked"`
	ShowDSTitle      types.Bool   `tfsdk:"show_data_source_title"`
	DatabaseType     types.String `tfsdk:"database_type"`
	DataSourceIDs    types.List   `tfsdk:"data_source_ids"`
}

func (m *DatabaseResourceModel) iconFields() iconFields {
	return iconFields{Emoji: m.Icon, Name: m.IconName, Color: m.IconColor, URL: m.IconURL, CustomEmojiID: m.IconCustomEmoji}
}

// setManagedIcon copies the non-emoji icon attributes. The emoji `icon`
// attribute is Computed here and handled separately.
func (m *DatabaseResourceModel) setManagedIcon(f iconFields) {
	m.IconName, m.IconColor, m.IconURL, m.IconCustomEmoji = f.Name, f.Color, f.URL, f.CustomEmojiID
}

// titlePropertyConfigWithName wraps the Notion title property config with a
// "name" field for renaming. The SDK's TitlePropertyConfig lacks this field.
type titlePropertyConfigWithName struct {
	ID    notionapi.PropertyID         `json:"id,omitempty"`
	Type  notionapi.PropertyConfigType `json:"type"`
	Title struct{}                     `json:"title"`
	Name  string                       `json:"name"`
}

func (p titlePropertyConfigWithName) GetType() notionapi.PropertyConfigType {
	return p.Type
}

func (p titlePropertyConfigWithName) GetID() notionapi.PropertyID {
	return p.ID
}

func NewDatabaseResource() resource.Resource {
	return &DatabaseResource{}
}

func (r *DatabaseResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (r *DatabaseResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Notion database.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the database.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"parent": schema.StringAttribute{
				Description: "The ID of the parent page.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"title": schema.StringAttribute{
				Description: "The title of the database.",
				Required:    true,
			},
			"title_column_title": schema.StringAttribute{
				Description: "The name of the title column.",
				Required:    true,
			},
			"title_column_id": schema.StringAttribute{
				Description: "The ID of the title column.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"url": schema.StringAttribute{
				Description: "The URL of the database.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"is_inline": schema.BoolAttribute{
				Description: "Whether the database appears inline on the parent page. If false, it appears as a child page.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Description: "The description of the database (read-only, set in Notion UI).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"icon": schema.StringAttribute{
				Description: "Emoji icon of the database. When unset, the current emoji is reported and left alone. " +
					"Conflicts with icon_name, icon_url, and icon_custom_emoji_id.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"icon_name": schema.StringAttribute{
				Description: "Native Notion icon name, such as `pizza` (2026-03-25). Notion returns its internal icon name, " +
					"so use that rather than a picker alias to avoid a diff.",
				Optional: true,
			},
			"icon_color": schema.StringAttribute{
				Description: "Color of the native icon set with icon_name: gray, lightgray, brown, yellow, orange, green, blue, purple, pink, or red. Notion defaults to gray.",
				Optional:    true,
			},
			"icon_url": schema.StringAttribute{
				Description: "External image URL for the database icon. URLs from Notion's own icon set are rejected, use icon_name for those.",
				Optional:    true,
			},
			"icon_custom_emoji_id": schema.StringAttribute{
				Description: "ID of a custom emoji in the integration's workspace to use as the database icon.",
				Optional:    true,
			},
			"is_locked": schema.BoolAttribute{
				Description: "Whether the database is locked from editing in the Notion app. It does not block API updates. " +
					"When unset, the current value is reported and left alone.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"show_data_source_title": schema.BoolAttribute{
				Description: "Whether the data source title is shown above the database's views (the Show data source title setting). " +
					"Notion defaults it to true on create. Full-page databases with one data source never show the title, whatever this says. " +
					"When unset, the current value is reported and left alone.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"database_type": schema.StringAttribute{
				Description: "The typed database kind Notion reports, such as `tasks`, `projects`, or `skills`. Null for a regular database.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"data_source_ids": schema.ListAttribute{
				Description: "IDs of the database's data sources, without hyphens. Use one as `data_source_id` on `notion_view`.",
				ElementType: types.StringType,
				Computed:    true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *DatabaseResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg DatabaseResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateIconFields(cfg.iconFields(), &resp.Diagnostics)
}

// applyExtras writes icon, is_locked, and show_data_source_title on the
// database container over Notion-Version 2026-03-11, where Update a database
// takes them without touching the schema. The SDK's 2022-06-28 request can't
// send native icons, custom emoji, or the newer flags. clearIcon removes an
// icon that the config no longer manages. Each flag is sent only when
// configured, so an unset one is left as it is in Notion.
func (r *DatabaseResource) applyExtras(ctx context.Context, id string, cfg *DatabaseResourceModel, clearIcon bool) (containerExtras, error) {
	token, err := tokenForClient(r.client)
	if err != nil {
		return containerExtras{}, err
	}
	return patchContainerExtras(ctx, token, "databases", id, cfg.iconFields().request(), clearIcon, cfg.IsLocked, cfg.extraFields())
}

// extraFields returns the database-only fields for the extras PATCH.
func (m *DatabaseResourceModel) extraFields() map[string]interface{} {
	if m.ShowDSTitle.IsNull() || m.ShowDSTitle.IsUnknown() {
		return nil
	}
	return map[string]interface{}{"show_data_source_title": m.ShowDSTitle.ValueBool()}
}

// finishExtras fills the computed emoji `icon`, is_locked, and
// show_data_source_title after a write. A known planned emoji is kept as
// planned, since Terraform rejects a result that differs from a known plan
// value. Read refreshes it.
//
// show_data_source_title is taken from the response. The docs describe it as
// the stored setting, which full-page databases with one data source ignore,
// so a configured value is expected to come back as sent.
func finishExtras(plan *DatabaseResourceModel, extras containerExtras) {
	if plan.Icon.IsUnknown() {
		if extras.Icon.Type == "emoji" {
			plan.Icon = types.StringValue(extras.Icon.Emoji)
		} else {
			plan.Icon = types.StringValue("")
		}
	}
	plan.IsLocked = types.BoolValue(extras.IsLocked)
	switch {
	case extras.ShowDataSourceTitle != nil:
		plan.ShowDSTitle = types.BoolValue(*extras.ShowDataSourceTitle)
	case plan.ShowDSTitle.IsUnknown():
		plan.ShowDSTitle = types.BoolNull()
	}
	plan.setReadOnly(extras)
}

// setReadOnly copies database_type and data_source_ids from a 2026-03-11
// database response. database_type isn't validated, so a new kind Notion
// adds reads through as is.
func (m *DatabaseResourceModel) setReadOnly(extras containerExtras) {
	if extras.DatabaseType == nil {
		m.DatabaseType = types.StringNull()
	} else {
		m.DatabaseType = types.StringValue(*extras.DatabaseType)
	}
	if extras.DataSourceIDs == nil {
		m.DataSourceIDs = types.ListNull(types.StringType)
		return
	}
	ids := make([]attr.Value, len(extras.DataSourceIDs))
	for i, id := range extras.DataSourceIDs {
		ids[i] = types.StringValue(id)
	}
	m.DataSourceIDs = types.ListValueMust(types.StringType, ids)
}

// showDSTitleValue maps the response field to state, null when absent.
func showDSTitleValue(extras containerExtras) types.Bool {
	if extras.ShowDataSourceTitle == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*extras.ShowDataSourceTitle)
}

func (r *DatabaseResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DatabaseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DatabaseResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	params := &notionapi.DatabaseCreateRequest{
		Parent: notionapi.Parent{
			Type:   notionapi.ParentTypePageID,
			PageID: notionapi.PageID(plan.Parent.ValueString()),
		},
		Title: plainToRichText(plan.Title.ValueString()),
		Properties: notionapi.PropertyConfigs{
			plan.TitleColumnTitle.ValueString(): notionapi.TitlePropertyConfig{
				Type:  notionapi.PropertyConfigTypeTitle,
				Title: struct{}{},
			},
		},
		IsInline: plan.IsInline.ValueBool(),
	}

	db, err := r.client.Database.Create(ctx, params)
	if err != nil {
		resp.Diagnostics.AddError("Error creating database", err.Error())
		return
	}

	plan.ID = types.StringValue(normalizeID(string(db.ID)))
	plan.URL = types.StringValue(db.URL)
	plan.IsInline = types.BoolValue(db.IsInline)
	plan.Description = types.StringValue(richTextToPlain(db.Description))

	for name, prop := range db.Properties {
		if name == plan.TitleColumnTitle.ValueString() {
			plan.TitleColumnID = types.StringValue(string(prop.GetID()))
			break
		}
	}

	// On create the plan holds exactly what was configured (unset Computed
	// values are unknown), so it doubles as the config here.
	extras, err := r.applyExtras(ctx, plan.ID.ValueString(), &plan, false)
	if err != nil {
		// The database exists, so keep it in state rather than orphan it.
		resp.Diagnostics.AddError("Error setting database icon or lock", err.Error())
		if plan.Icon.IsUnknown() {
			plan.Icon = types.StringValue("")
		}
		if plan.IsLocked.IsUnknown() {
			plan.IsLocked = types.BoolValue(false)
		}
		if plan.ShowDSTitle.IsUnknown() {
			plan.ShowDSTitle = types.BoolNull()
		}
		if plan.DatabaseType.IsUnknown() {
			plan.DatabaseType = types.StringNull()
		}
		if plan.DataSourceIDs.IsUnknown() {
			plan.DataSourceIDs = types.ListNull(types.StringType)
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}
	finishExtras(&plan, extras)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabaseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DatabaseResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	db, err := r.client.Database.Get(ctx, notionapi.DatabaseID(state.ID.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading database", err.Error())
		return
	}

	if db.Archived {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(normalizeID(string(db.ID)))
	state.Title = types.StringValue(richTextToPlain(db.Title))
	state.URL = types.StringValue(db.URL)
	state.IsInline = types.BoolValue(db.IsInline)
	state.Description = types.StringValue(richTextToPlain(db.Description))

	// The schema read above stays on the SDK (2022-06-28, where properties
	// live on the database). Icon, is_locked, show_data_source_title,
	// database_type, and data_source_ids come from a second GET on
	// 2026-03-11, which reports native icons, custom emoji, and data sources.
	token, err := tokenForClient(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error reading database", err.Error())
		return
	}
	extrasBody, err := containerRequest(ctx, token, http.MethodGet, "databases", state.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError("Error reading database icon and lock", err.Error())
		return
	}
	extras, err := decodeContainerExtras(extrasBody)
	if err != nil {
		resp.Diagnostics.AddError("Error reading database icon and lock", err.Error())
		return
	}
	reconciled := state.iconFields().reconcile(extras.Icon)
	state.Icon = reconciled.Emoji
	state.setManagedIcon(reconciled)
	state.IsLocked = types.BoolValue(extras.IsLocked)
	state.ShowDSTitle = showDSTitleValue(extras)
	state.setReadOnly(extras)

	if db.Parent.Type == notionapi.ParentTypePageID {
		state.Parent = types.StringValue(normalizeID(string(db.Parent.PageID)))
	}

	for name, prop := range db.Properties {
		if prop.GetType() == notionapi.PropertyConfigTypeTitle {
			state.TitleColumnTitle = types.StringValue(name)
			state.TitleColumnID = types.StringValue(string(prop.GetID()))
			break
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DatabaseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config DatabaseResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	params := &notionapi.DatabaseUpdateRequest{
		Title: plainToRichText(plan.Title.ValueString()),
	}

	// If the title column name changed, rename it via the Notion API.
	// We send the current (state) name as the key with a "name" field set to
	// the desired (plan) name. The SDK's TitlePropertyConfig doesn't support
	// the "name" field, so we use titlePropertyConfigWithName.
	if plan.TitleColumnTitle.ValueString() != state.TitleColumnTitle.ValueString() {
		params.Properties = notionapi.PropertyConfigs{
			state.TitleColumnTitle.ValueString(): titlePropertyConfigWithName{
				Type:  notionapi.PropertyConfigTypeTitle,
				Title: struct{}{},
				Name:  plan.TitleColumnTitle.ValueString(),
			},
		}
	}

	db, err := r.client.Database.Update(ctx, notionapi.DatabaseID(plan.ID.ValueString()), params)
	if err != nil {
		resp.Diagnostics.AddError("Error updating database", err.Error())
		return
	}

	plan.URL = types.StringValue(db.URL)
	plan.IsInline = types.BoolValue(db.IsInline)
	plan.Description = types.StringValue(richTextToPlain(db.Description))

	for name, prop := range db.Properties {
		if prop.GetType() == notionapi.PropertyConfigTypeTitle {
			plan.TitleColumnTitle = types.StringValue(name)
			plan.TitleColumnID = types.StringValue(string(prop.GetID()))
			break
		}
	}

	// Use the config, not the plan. Computed icon, is_locked, and
	// show_data_source_title carry the prior state when unset, and resending
	// those would undo UI changes. Clear the icon only when a managed
	// non-emoji icon was removed.
	clear := config.iconFields().request() == nil && state.iconFields().managed()
	extras, err := r.applyExtras(ctx, plan.ID.ValueString(), &config, clear)
	if err != nil {
		resp.Diagnostics.AddError("Error setting database icon or lock", err.Error())
		return
	}
	finishExtras(&plan, extras)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabaseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DatabaseResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, err := tokenForClient(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error trashing database", err.Error())
		return
	}
	if err := trashObject(ctx, token, "databases", state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error trashing database", err.Error())
		return
	}
}

func (r *DatabaseResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
