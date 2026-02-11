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
	_ resource.Resource                = &DatabaseEntryResource{}
	_ resource.ResourceWithImportState = &DatabaseEntryResource{}
)

type DatabaseEntryResource struct {
	client *notionapi.Client
}

type DatabaseEntryResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Database types.String `tfsdk:"database"`
	Title    types.String `tfsdk:"title"`
	URL      types.String `tfsdk:"url"`
}

func NewDatabaseEntryResource() resource.Resource {
	return &DatabaseEntryResource{}
}

func (r *DatabaseEntryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_entry"
}

func (r *DatabaseEntryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an entry (page) in a Notion database.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the database entry.",
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
			"title": schema.StringAttribute{
				Description: "The title of the entry.",
				Required:    true,
			},
			"url": schema.StringAttribute{
				Description: "The URL of the entry.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *DatabaseEntryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// findTitlePropertyName retrieves the database and returns the name of the title property.
func (r *DatabaseEntryResource) findTitlePropertyName(ctx context.Context, databaseID string) (string, error) {
	db, err := r.client.Database.Get(ctx, notionapi.DatabaseID(databaseID))
	if err != nil {
		return "", err
	}
	for name, prop := range db.Properties {
		if prop.GetType() == notionapi.PropertyConfigTypeTitle {
			return name, nil
		}
	}
	return "Name", nil
}

func (r *DatabaseEntryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DatabaseEntryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	titlePropName, err := r.findTitlePropertyName(ctx, plan.Database.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading database", err.Error())
		return
	}

	params := &notionapi.PageCreateRequest{
		Parent: notionapi.Parent{
			Type:       notionapi.ParentTypeDatabaseID,
			DatabaseID: notionapi.DatabaseID(plan.Database.ValueString()),
		},
		Properties: notionapi.Properties{
			titlePropName: notionapi.TitleProperty{
				Type:  notionapi.PropertyTypeTitle,
				Title: plainToRichText(plan.Title.ValueString()),
			},
		},
	}

	page, err := r.client.Page.Create(ctx, params)
	if err != nil {
		resp.Diagnostics.AddError("Error creating database entry", err.Error())
		return
	}

	plan.ID = types.StringValue(normalizeID(string(page.ID)))
	plan.URL = types.StringValue(page.URL)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabaseEntryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DatabaseEntryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	page, err := r.client.Page.Get(ctx, notionapi.PageID(state.ID.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading database entry", err.Error())
		return
	}

	if page.Archived {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(normalizeID(string(page.ID)))
	state.URL = types.StringValue(page.URL)

	if page.Parent.Type == notionapi.ParentTypeDatabaseID {
		state.Database = types.StringValue(normalizeID(string(page.Parent.DatabaseID)))
	}

	for _, prop := range page.Properties {
		if tp, ok := prop.(*notionapi.TitleProperty); ok {
			state.Title = types.StringValue(richTextToPlain(tp.Title))
			break
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DatabaseEntryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DatabaseEntryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	titlePropName, err := r.findTitlePropertyName(ctx, plan.Database.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading database", err.Error())
		return
	}

	params := &notionapi.PageUpdateRequest{
		Properties: notionapi.Properties{
			titlePropName: notionapi.TitleProperty{
				Type:  notionapi.PropertyTypeTitle,
				Title: plainToRichText(plan.Title.ValueString()),
			},
		},
	}

	page, err := r.client.Page.Update(ctx, notionapi.PageID(plan.ID.ValueString()), params)
	if err != nil {
		resp.Diagnostics.AddError("Error updating database entry", err.Error())
		return
	}

	plan.URL = types.StringValue(page.URL)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabaseEntryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DatabaseEntryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.Page.Update(ctx, notionapi.PageID(state.ID.ValueString()), &notionapi.PageUpdateRequest{
		Archived: true,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error archiving database entry", err.Error())
		return
	}
}

func (r *DatabaseEntryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
