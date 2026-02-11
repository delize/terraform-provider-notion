package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var (
	_ resource.Resource                = &DatabasePropertyBasicResource{}
	_ resource.ResourceWithImportState = &DatabasePropertyBasicResource{}
)

// DatabasePropertyBasicResource handles the 10 simple property types that have no extra attributes.
type DatabasePropertyBasicResource struct {
	client       *notionapi.Client
	typeName     string
	propertyType notionapi.PropertyConfigType
}

// newDatabasePropertyBasicResource creates a factory function for a basic property resource.
func newDatabasePropertyBasicResource(typeName string, propertyType notionapi.PropertyConfigType) func() resource.Resource {
	return func() resource.Resource {
		return &DatabasePropertyBasicResource{
			typeName:     typeName,
			propertyType: propertyType,
		}
	}
}

func (r *DatabasePropertyBasicResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_property_" + r.typeName
}

func (r *DatabasePropertyBasicResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: fmt.Sprintf("Manages a %s property on a Notion database.", r.typeName),
		Attributes:  databasePropertyBaseSchema(),
	}
}

func (r *DatabasePropertyBasicResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DatabasePropertyBasicResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databasePropertyBaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	propConfig := r.buildPropertyConfig()

	db, err := r.client.Database.Update(ctx, notionapi.DatabaseID(plan.Database.ValueString()), &notionapi.DatabaseUpdateRequest{
		Properties: notionapi.PropertyConfigs{
			plan.Name.ValueString(): propConfig,
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating property", err.Error())
		return
	}

	if prop, ok := db.Properties[plan.Name.ValueString()]; ok {
		plan.ID = types.StringValue(string(prop.GetID()))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabasePropertyBasicResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databasePropertyBaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	propID, propName, err := readPropertyFromDatabase(ctx, r.client, state.Database.ValueString(), state.Name.ValueString(), state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(propID)
	state.Name = types.StringValue(propName)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DatabasePropertyBasicResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Name has RequiresReplace, database has RequiresReplace. Only property-specific
	// attributes can trigger Update. Basic properties have none, so this is a no-op.
	var plan databasePropertyBaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabasePropertyBasicResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databasePropertyBaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := deletePropertyFromDatabase(ctx, r.client, state.Database.ValueString(), state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error deleting property", err.Error())
		return
	}
}

func (r *DatabasePropertyBasicResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	databaseID, propName, err := parseCompositeID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}

	resp.State.SetAttribute(ctx, path.Root("database"), types.StringValue(databaseID))
	resp.State.SetAttribute(ctx, path.Root("name"), types.StringValue(propName))
}

func (r *DatabasePropertyBasicResource) buildPropertyConfig() notionapi.PropertyConfig {
	switch r.propertyType {
	case notionapi.PropertyConfigTypeRichText:
		return notionapi.RichTextPropertyConfig{Type: r.propertyType, RichText: struct{}{}}
	case notionapi.PropertyConfigTypeDate:
		return notionapi.DatePropertyConfig{Type: r.propertyType, Date: struct{}{}}
	case notionapi.PropertyConfigTypePeople:
		return notionapi.PeoplePropertyConfig{Type: r.propertyType, People: struct{}{}}
	case notionapi.PropertyConfigTypeCheckbox:
		return notionapi.CheckboxPropertyConfig{Type: r.propertyType, Checkbox: struct{}{}}
	case notionapi.PropertyConfigTypeURL:
		return notionapi.URLPropertyConfig{Type: r.propertyType, URL: struct{}{}}
	case notionapi.PropertyConfigTypeEmail:
		return notionapi.EmailPropertyConfig{Type: r.propertyType, Email: struct{}{}}
	case notionapi.PropertyConfigCreatedTime:
		return notionapi.CreatedTimePropertyConfig{Type: r.propertyType, CreatedTime: struct{}{}}
	case notionapi.PropertyConfigCreatedBy:
		return notionapi.CreatedByPropertyConfig{Type: r.propertyType, CreatedBy: struct{}{}}
	case notionapi.PropertyConfigLastEditedTime:
		return notionapi.LastEditedTimePropertyConfig{Type: r.propertyType, LastEditedTime: struct{}{}}
	case notionapi.PropertyConfigLastEditedBy:
		return notionapi.LastEditedByPropertyConfig{Type: r.propertyType, LastEditedBy: struct{}{}}
	default:
		return nil
	}
}
