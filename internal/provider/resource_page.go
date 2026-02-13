package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var (
	_ resource.Resource                = &PageResource{}
	_ resource.ResourceWithImportState = &PageResource{}
)

type PageResource struct {
	client *notionapi.Client
}

type PageResourceModel struct {
	ID           types.String `tfsdk:"id"`
	ParentPageID types.String `tfsdk:"parent_page_id"`
	Title        types.String `tfsdk:"title"`
	URL          types.String `tfsdk:"url"`
	Icon         types.String `tfsdk:"icon"`
}

func NewPageResource() resource.Resource {
	return &PageResource{}
}

func (r *PageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_page"
}

func (r *PageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Notion page.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the page.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"parent_page_id": schema.StringAttribute{
				Description: "The ID of the parent page.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"title": schema.StringAttribute{
				Description: "The title of the page.",
				Required:    true,
			},
			"url": schema.StringAttribute{
				Description: "The URL of the page.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"icon": schema.StringAttribute{
				Description: "Emoji icon for the page.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
		},
	}
}

func (r *PageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	params := &notionapi.PageCreateRequest{
		Parent: notionapi.Parent{
			Type:   notionapi.ParentTypePageID,
			PageID: notionapi.PageID(plan.ParentPageID.ValueString()),
		},
		Properties: notionapi.Properties{
			"title": notionapi.TitleProperty{
				Type:  notionapi.PropertyTypeTitle,
				Title: plainToRichText(plan.Title.ValueString()),
			},
		},
	}

	if plan.Icon.ValueString() != "" {
		emoji := notionapi.Emoji(plan.Icon.ValueString())
		params.Icon = &notionapi.Icon{
			Type:  "emoji",
			Emoji: &emoji,
		}
	}

	page, err := r.client.Page.Create(ctx, params)
	if err != nil {
		resp.Diagnostics.AddError("Error creating page", err.Error())
		return
	}

	plan.ID = types.StringValue(normalizeID(string(page.ID)))
	plan.URL = types.StringValue(page.URL)
	if page.Icon != nil && page.Icon.Emoji != nil {
		plan.Icon = types.StringValue(string(*page.Icon.Emoji))
	} else {
		plan.Icon = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	page, err := r.client.Page.Get(ctx, notionapi.PageID(state.ID.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading page", err.Error())
		return
	}

	if page.Archived {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(normalizeID(string(page.ID)))
	state.URL = types.StringValue(page.URL)

	if page.Parent.Type == notionapi.ParentTypePageID {
		state.ParentPageID = types.StringValue(normalizeID(string(page.Parent.PageID)))
	}

	if titleProp, ok := page.Properties["title"]; ok {
		if tp, ok := titleProp.(*notionapi.TitleProperty); ok {
			state.Title = types.StringValue(richTextToPlain(tp.Title))
		}
	}

	if page.Icon != nil && page.Icon.Emoji != nil {
		state.Icon = types.StringValue(string(*page.Icon.Emoji))
	} else {
		state.Icon = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *PageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	params := &notionapi.PageUpdateRequest{
		Properties: notionapi.Properties{
			"title": notionapi.TitleProperty{
				Type:  notionapi.PropertyTypeTitle,
				Title: plainToRichText(plan.Title.ValueString()),
			},
		},
	}

	if plan.Icon.ValueString() != "" {
		emoji := notionapi.Emoji(plan.Icon.ValueString())
		params.Icon = &notionapi.Icon{
			Type:  "emoji",
			Emoji: &emoji,
		}
	}

	page, err := r.client.Page.Update(ctx, notionapi.PageID(plan.ID.ValueString()), params)
	if err != nil {
		resp.Diagnostics.AddError("Error updating page", err.Error())
		return
	}

	plan.URL = types.StringValue(page.URL)
	if page.Icon != nil && page.Icon.Emoji != nil {
		plan.Icon = types.StringValue(string(*page.Icon.Emoji))
	} else {
		plan.Icon = types.StringValue("")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.Page.Update(ctx, notionapi.PageID(state.ID.ValueString()), &notionapi.PageUpdateRequest{
		Archived: true,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error archiving page", err.Error())
		return
	}
}

func (r *PageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
