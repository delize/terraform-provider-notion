package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// notion_database_property_status manages a status property on a database.
// Status was read-only until the 2026-03-19 Notion API change made it
// creatable and updatable. Since 2026-06-22 an option can be placed in a
// group (To-do / In progress / Complete), and since 2026-10-22 the schema
// reports which option is the default. Reads and writes go through
// notion_status_property.go because the SDK models neither.

var (
	_ resource.Resource                   = &DatabasePropertyStatusResource{}
	_ resource.ResourceWithImportState    = &DatabasePropertyStatusResource{}
	_ resource.ResourceWithValidateConfig = &DatabasePropertyStatusResource{}
)

type DatabasePropertyStatusResource struct {
	client *notionapi.Client
}

type DatabasePropertyStatusModel struct {
	ID            types.String `tfsdk:"id"`
	Database      types.String `tfsdk:"database"`
	Name          types.String `tfsdk:"name"`
	Options       types.Map    `tfsdk:"options"`
	OptionGroups  types.Map    `tfsdk:"option_groups"`
	DefaultOption types.String `tfsdk:"default_option"`
}

func NewDatabasePropertyStatusResource() resource.Resource {
	return &DatabasePropertyStatusResource{}
}

func (r *DatabasePropertyStatusResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_property_status"
}

func (r *DatabasePropertyStatusResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a status property on a Notion database. " +
			"Status properties became writable via the API in the 2026-03-19 change; " +
			"prior to that they could only be created in the Notion UI.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the property.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"database": schema.StringAttribute{
				Description: "The ID of the parent database.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the property.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"options": schema.MapAttribute{
				Description: "Map of option label to color. Valid colors: default, gray, brown, orange, yellow, green, blue, purple, pink, red. " +
					"Changing an existing option's color updates it in place. Notion matches option names without regard to case.",
				Required:    true,
				ElementType: types.StringType,
			},
			"option_groups": schema.MapAttribute{
				Description: "Map of option label to the group it belongs in: To-do, In progress, or Complete. " +
					"Only the options listed here are managed. An option left out keeps its current group, and a new one goes to To-do.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"default_option": schema.StringAttribute{
				Description: "Name of the option a page gets when it has no explicit status, or null when none is configured. Set in Notion, read-only here.",
				Computed:    true,
			},
		},
	}
}

func (r *DatabasePropertyStatusResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg DatabasePropertyStatusModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.OptionGroups.IsNull() || cfg.OptionGroups.IsUnknown() {
		return
	}

	groups := map[string]types.String{}
	resp.Diagnostics.Append(cfg.OptionGroups.ElementsAs(ctx, &groups, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for label, g := range groups {
		if g.IsNull() || g.IsUnknown() {
			continue
		}
		if !slices.Contains(statusGroupNames, g.ValueString()) {
			resp.Diagnostics.AddAttributeError(path.Root("option_groups").AtMapKey(label),
				"Invalid status group",
				fmt.Sprintf("%q is not a status group. Use one of: %s.", g.ValueString(), strings.Join(statusGroupNames, ", ")))
		}
	}

	if cfg.Options.IsNull() || cfg.Options.IsUnknown() {
		return
	}
	options := map[string]types.String{}
	resp.Diagnostics.Append(cfg.Options.ElementsAs(ctx, &options, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for label := range groups {
		if _, ok := options[label]; !ok {
			resp.Diagnostics.AddAttributeError(path.Root("option_groups").AtMapKey(label),
				"Unknown status option",
				fmt.Sprintf("%q is in option_groups but not in options. Every grouped option must also be listed in options.", label))
		}
	}
}

func (r *DatabasePropertyStatusResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DatabasePropertyStatusResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DatabasePropertyStatusModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.write(ctx, &plan, "creating", &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabasePropertyStatusResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DatabasePropertyStatusModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.write(ctx, &plan, "updating", &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// write sends the full option set (with any configured groups) and fills
// the computed attributes from the response. options and option_groups keep
// their planned values, and Read reports any drift on the next refresh.
func (r *DatabasePropertyStatusResource) write(ctx context.Context, plan *DatabasePropertyStatusModel, verb string, diags *diag.Diagnostics) {
	token, err := tokenForClient(r.client)
	if err != nil {
		diags.AddError("Error "+verb+" status property", err.Error())
		return
	}

	options, d := buildStatusOptions(ctx, plan.Options, plan.OptionGroups)
	diags.Append(d...)
	if diags.HasError() {
		return
	}

	st, err := upsertStatusProperty(ctx, token, plan.Database.ValueString(), plan.Name.ValueString(), options)
	if err != nil {
		diags.AddError("Error "+verb+" status property", err.Error())
		return
	}

	plan.ID = types.StringValue(st.ID)
	plan.DefaultOption = types.StringPointerValue(st.DefaultOption)
}

func (r *DatabasePropertyStatusResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DatabasePropertyStatusModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, err := tokenForClient(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error reading database", err.Error())
		return
	}

	st, found, err := readStatusProperty(ctx, token, state.Database.ValueString(), state.ID.ValueString(), state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading database", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(st.ID)
	state.Name = types.StringValue(st.Name)

	if st.IsStatus {
		mapVal, diags := types.MapValueFrom(ctx, types.StringType, st.Options)
		resp.Diagnostics.Append(diags...)
		state.Options = mapVal
		state.DefaultOption = types.StringPointerValue(st.DefaultOption)

		// Only the options the config manages are tracked, so groups set in
		// the Notion UI for other options don't show up as drift.
		if !state.OptionGroups.IsNull() && !state.OptionGroups.IsUnknown() {
			prior := map[string]string{}
			resp.Diagnostics.Append(state.OptionGroups.ElementsAs(ctx, &prior, false)...)
			next := make(map[string]string, len(prior))
			for label := range prior {
				if g, ok := st.groupFor(label); ok {
					next[label] = g
				}
			}
			groupsVal, diags := types.MapValueFrom(ctx, types.StringType, next)
			resp.Diagnostics.Append(diags...)
			state.OptionGroups = groupsVal
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DatabasePropertyStatusResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DatabasePropertyStatusModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := deletePropertyFromDatabase(ctx, r.client, state.Database.ValueString(), state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error deleting status property", err.Error())
		return
	}
}

func (r *DatabasePropertyStatusResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	databaseID, propName, err := parseCompositeID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}

	resp.State.SetAttribute(ctx, path.Root("database"), types.StringValue(databaseID))
	resp.State.SetAttribute(ctx, path.Root("name"), types.StringValue(propName))
}

// buildStatusOptions turns the options and option_groups maps into the
// request list. Output is sorted by name so request bodies are stable.
func buildStatusOptions(ctx context.Context, optionsMap, groupsMap types.Map) ([]statusOptionRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	colors := map[string]string{}
	diags.Append(optionsMap.ElementsAs(ctx, &colors, false)...)
	groups := map[string]string{}
	if !groupsMap.IsNull() && !groupsMap.IsUnknown() {
		diags.Append(groupsMap.ElementsAs(ctx, &groups, false)...)
	}
	if diags.HasError() {
		return nil, diags
	}

	out := make([]statusOptionRequest, 0, len(colors))
	for name, color := range colors {
		out = append(out, statusOptionRequest{Name: name, Color: color, Group: groups[name]})
	}
	slices.SortFunc(out, func(a, b statusOptionRequest) int { return strings.Compare(a.Name, b.Name) })
	return out, diags
}
