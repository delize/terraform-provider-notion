package provider

import (
	"context"
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

var (
	_ resource.Resource                = &DatabasePropertyNumberResource{}
	_ resource.ResourceWithImportState = &DatabasePropertyNumberResource{}
)

type DatabasePropertyNumberResource struct {
	client *notionapi.Client
}

type DatabasePropertyNumberModel struct {
	ID       types.String `tfsdk:"id"`
	Database types.String `tfsdk:"database"`
	Name     types.String `tfsdk:"name"`
	Format   types.String `tfsdk:"format"`
}

func NewDatabasePropertyNumberResource() resource.Resource {
	return &DatabasePropertyNumberResource{}
}

func (r *DatabasePropertyNumberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_property_number"
}

func (r *DatabasePropertyNumberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a number property on a Notion database.",
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
			"format": schema.StringAttribute{
				Description: "The number format (e.g., number, percent, dollar, euro).",
				Required:    true,
				Validators: []validator.String{
					NumberFormatValidator(),
				},
			},
		},
	}
}

func (r *DatabasePropertyNumberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DatabasePropertyNumberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DatabasePropertyNumberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	db, err := r.client.Database.Update(ctx, notionapi.DatabaseID(plan.Database.ValueString()), &notionapi.DatabaseUpdateRequest{
		Properties: notionapi.PropertyConfigs{
			plan.Name.ValueString(): notionapi.NumberPropertyConfig{
				Type: notionapi.PropertyConfigTypeNumber,
				Number: notionapi.NumberFormat{
					Format: notionapi.FormatType(plan.Format.ValueString()),
				},
			},
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating number property", err.Error())
		return
	}

	if prop, ok := db.Properties[plan.Name.ValueString()]; ok {
		plan.ID = types.StringValue(string(prop.GetID()))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabasePropertyNumberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DatabasePropertyNumberModel
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

			if numProp, ok := prop.(*notionapi.NumberPropertyConfig); ok {
				state.Format = types.StringValue(string(numProp.Number.Format))
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

func (r *DatabasePropertyNumberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DatabasePropertyNumberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	db, err := r.client.Database.Update(ctx, notionapi.DatabaseID(plan.Database.ValueString()), &notionapi.DatabaseUpdateRequest{
		Properties: notionapi.PropertyConfigs{
			plan.Name.ValueString(): notionapi.NumberPropertyConfig{
				Type: notionapi.PropertyConfigTypeNumber,
				Number: notionapi.NumberFormat{
					Format: notionapi.FormatType(plan.Format.ValueString()),
				},
			},
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating number property", err.Error())
		return
	}

	if prop, ok := db.Properties[plan.Name.ValueString()]; ok {
		plan.ID = types.StringValue(string(prop.GetID()))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabasePropertyNumberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DatabasePropertyNumberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := deletePropertyFromDatabase(ctx, r.client, state.Database.ValueString(), state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error deleting number property", err.Error())
		return
	}
}

func (r *DatabasePropertyNumberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	databaseID, propName, err := parseCompositeID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}

	resp.State.SetAttribute(ctx, path.Root("database"), types.StringValue(databaseID))
	resp.State.SetAttribute(ctx, path.Root("name"), types.StringValue(propName))
}
