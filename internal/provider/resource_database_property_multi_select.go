package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var (
	_ resource.Resource                = &DatabasePropertyMultiSelectResource{}
	_ resource.ResourceWithImportState = &DatabasePropertyMultiSelectResource{}
)

type DatabasePropertyMultiSelectResource struct {
	client *notionapi.Client
}

type DatabasePropertyMultiSelectModel struct {
	ID       types.String `tfsdk:"id"`
	Database types.String `tfsdk:"database"`
	Name     types.String `tfsdk:"name"`
	Options  types.Map    `tfsdk:"options"`
}

func NewDatabasePropertyMultiSelectResource() resource.Resource {
	return &DatabasePropertyMultiSelectResource{}
}

func (r *DatabasePropertyMultiSelectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_property_multi_select"
}

func (r *DatabasePropertyMultiSelectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a multi-select property on a Notion database.",
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
				Description: "Map of option label to color. Valid colors: default, gray, brown, orange, yellow, green, blue, purple, pink, red.",
				Required:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *DatabasePropertyMultiSelectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DatabasePropertyMultiSelectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DatabasePropertyMultiSelectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	options, diags := buildSelectOptions(ctx, plan.Options)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	db, err := r.client.Database.Update(ctx, notionapi.DatabaseID(plan.Database.ValueString()), &notionapi.DatabaseUpdateRequest{
		Properties: notionapi.PropertyConfigs{
			plan.Name.ValueString(): notionapi.MultiSelectPropertyConfig{
				Type:        notionapi.PropertyConfigTypeMultiSelect,
				MultiSelect: notionapi.Select{Options: options},
			},
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating multi-select property", err.Error())
		return
	}

	if prop, ok := db.Properties[plan.Name.ValueString()]; ok {
		plan.ID = types.StringValue(string(prop.GetID()))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabasePropertyMultiSelectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DatabasePropertyMultiSelectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	db, err := r.client.Database.Get(ctx, notionapi.DatabaseID(state.Database.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading database", err.Error())
		return
	}

	found := false
	for name, prop := range db.Properties {
		if string(prop.GetID()) == state.ID.ValueString() || name == state.Name.ValueString() {
			state.ID = types.StringValue(string(prop.GetID()))
			state.Name = types.StringValue(name)

			if msProp, ok := prop.(*notionapi.MultiSelectPropertyConfig); ok {
				optionsMap := make(map[string]string)
				for _, opt := range msProp.MultiSelect.Options {
					optionsMap[opt.Name] = string(opt.Color)
				}
				mapVal, diags := types.MapValueFrom(ctx, types.StringType, optionsMap)
				resp.Diagnostics.Append(diags...)
				state.Options = mapVal
			}
			found = true
			break
		}
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DatabasePropertyMultiSelectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DatabasePropertyMultiSelectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	options, diags := buildSelectOptions(ctx, plan.Options)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	db, err := r.client.Database.Update(ctx, notionapi.DatabaseID(plan.Database.ValueString()), &notionapi.DatabaseUpdateRequest{
		Properties: notionapi.PropertyConfigs{
			plan.Name.ValueString(): notionapi.MultiSelectPropertyConfig{
				Type:        notionapi.PropertyConfigTypeMultiSelect,
				MultiSelect: notionapi.Select{Options: options},
			},
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating multi-select property", err.Error())
		return
	}

	if prop, ok := db.Properties[plan.Name.ValueString()]; ok {
		plan.ID = types.StringValue(string(prop.GetID()))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabasePropertyMultiSelectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DatabasePropertyMultiSelectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := deletePropertyFromDatabase(ctx, r.client, state.Database.ValueString(), state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error deleting multi-select property", err.Error())
		return
	}
}

func (r *DatabasePropertyMultiSelectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	databaseID, propName, err := parseCompositeID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}

	resp.State.SetAttribute(ctx, path.Root("database"), types.StringValue(databaseID))
	resp.State.SetAttribute(ctx, path.Root("name"), types.StringValue(propName))
}
