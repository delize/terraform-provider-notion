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

// notion_database_property_status manages a status property on a database.
// Status was read-only until the 2026-03-19 Notion API change made it
// creatable and updatable. Groups (To-do / In progress / Complete buckets) are
// managed server-side based on the option set; the schema only exposes the
// label-to-color map so users don't have to model group membership.

var (
	_ resource.Resource                = &DatabasePropertyStatusResource{}
	_ resource.ResourceWithImportState = &DatabasePropertyStatusResource{}
)

type DatabasePropertyStatusResource struct {
	client *notionapi.Client
}

type DatabasePropertyStatusModel struct {
	ID       types.String `tfsdk:"id"`
	Database types.String `tfsdk:"database"`
	Name     types.String `tfsdk:"name"`
	Options  types.Map    `tfsdk:"options"`
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
					"Notion assigns options to the To-do / In progress / Complete groups server-side; group membership is not modeled here.",
				Required:    true,
				ElementType: types.StringType,
			},
		},
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

	options, diags := buildSelectOptions(ctx, plan.Options)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	db, err := r.client.Database.Update(ctx, notionapi.DatabaseID(plan.Database.ValueString()), &notionapi.DatabaseUpdateRequest{
		Properties: notionapi.PropertyConfigs{
			plan.Name.ValueString(): notionapi.StatusPropertyConfig{
				Type:   notionapi.PropertyConfigStatus,
				Status: notionapi.StatusConfig{Options: options},
			},
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating status property", err.Error())
		return
	}

	if prop, ok := db.Properties[plan.Name.ValueString()]; ok {
		plan.ID = types.StringValue(string(prop.GetID()))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabasePropertyStatusResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DatabasePropertyStatusModel
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

			if statusProp, ok := prop.(*notionapi.StatusPropertyConfig); ok {
				optionsMap := make(map[string]string)
				for _, opt := range statusProp.Status.Options {
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

func (r *DatabasePropertyStatusResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DatabasePropertyStatusModel
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
			plan.Name.ValueString(): notionapi.StatusPropertyConfig{
				Type:   notionapi.PropertyConfigStatus,
				Status: notionapi.StatusConfig{Options: options},
			},
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating status property", err.Error())
		return
	}

	if prop, ok := db.Properties[plan.Name.ValueString()]; ok {
		plan.ID = types.StringValue(string(prop.GetID()))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
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
