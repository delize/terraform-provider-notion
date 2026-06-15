package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var (
	_ resource.Resource                = &PageResource{}
	_ resource.ResourceWithImportState = &PageResource{}
)

type PageResource struct {
	client   *notionapi.Client
	mdClient *markdownClient
}

type PageResourceModel struct {
	ID             types.String `tfsdk:"id"`
	ParentPageID   types.String `tfsdk:"parent_page_id"`
	Title          types.String `tfsdk:"title"`
	URL            types.String `tfsdk:"url"`
	Icon           types.String `tfsdk:"icon"`
	Markdown       types.String `tfsdk:"markdown"`
	MarkdownInsert *MarkdownInsertModel `tfsdk:"markdown_insert"`
}

// MarkdownInsertModel represents a one-shot markdown insertion at the start or
// end of the page using the 2026-05-15 insert_content.position API. Each
// change to either field triggers another insert — this is a trigger, not a
// declarative state, so removing content requires manual cleanup.
type MarkdownInsertModel struct {
	Content  types.String `tfsdk:"content"`
	Position types.String `tfsdk:"position"`
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
			"markdown": schema.StringAttribute{
				Description: "Page content as enhanced markdown. Mutually exclusive with managing content via notion_block resources. " +
					"Note: Notion may normalize the markdown content, so the stored value may differ slightly from what was submitted.",
				Optional: true,
			},
			"markdown_insert": schema.SingleNestedAttribute{
				Description: "Append or prepend markdown to the page without rewriting the existing content. " +
					"Each change to `content` or `position` triggers another insert via the Notion insert_content endpoint; " +
					"this is an imperative trigger, not declarative state. Removing the block does not remove the previously inserted content.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"content": schema.StringAttribute{
						Description: "Markdown to insert into the page.",
						Required:    true,
					},
					"position": schema.StringAttribute{
						Description: `Where to insert the content. Must be "start" (prepend) or "end" (append).`,
						Required:    true,
						Validators: []validator.String{
							MarkdownInsertPositionValidator(),
						},
					},
				},
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
	r.mdClient = newMarkdownClient(client)
}

func (r *PageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Markdown.IsNull() && !plan.Markdown.IsUnknown() {
		r.createWithMarkdown(ctx, &plan, resp)
	} else {
		r.createWithoutMarkdown(ctx, &plan, resp)
	}
}

func (r *PageResource) createWithMarkdown(ctx context.Context, plan *PageResourceModel, resp *resource.CreateResponse) {
	pageID, pageURL, err := r.mdClient.CreatePageWithMarkdownAndTitle(
		ctx,
		plan.ParentPageID.ValueString(),
		plan.Title.ValueString(),
		plan.Markdown.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Error creating page with markdown", err.Error())
		return
	}

	plan.ID = types.StringValue(normalizeID(pageID))
	plan.URL = types.StringValue(pageURL)

	if diags := r.applyMarkdownInsert(ctx, plan); diags != nil {
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Set icon if provided via a separate update since markdown create doesn't support it
	if plan.Icon.ValueString() != "" {
		emoji := notionapi.Emoji(plan.Icon.ValueString())
		page, err := r.client.Page.Update(ctx, notionapi.PageID(pageID), &notionapi.PageUpdateRequest{
			Icon: &notionapi.Icon{
				Type:  "emoji",
				Emoji: &emoji,
			},
			Properties: notionapi.Properties{},
		})
		if err != nil {
			resp.Diagnostics.AddError("Error setting page icon", err.Error())
			return
		}
		if page.Icon != nil && page.Icon.Emoji != nil {
			plan.Icon = types.StringValue(string(*page.Icon.Emoji))
		}
	} else {
		plan.Icon = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *PageResource) createWithoutMarkdown(ctx context.Context, plan *PageResourceModel, resp *resource.CreateResponse) {
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

	if diags := r.applyMarkdownInsert(ctx, plan); diags != nil {
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
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

	// Markdown is managed by the user's config; we don't read it back from the
	// API to avoid perpetual diffs caused by Notion's content normalization.

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *PageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Update page properties (title, icon)
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

	// Update markdown content if set
	if !plan.Markdown.IsNull() && !plan.Markdown.IsUnknown() {
		_, err = r.mdClient.ReplacePageMarkdown(ctx, plan.ID.ValueString(), plan.Markdown.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error updating page markdown", err.Error())
			return
		}
		// Keep plan value in state rather than API response to avoid normalization diffs
	}

	if diags := r.applyMarkdownInsert(ctx, &plan); diags != nil {
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// applyMarkdownInsert performs an insert_content PATCH if a markdown_insert
// block is configured on the plan. Returns diagnostics for the caller to
// append. Returns nil when no insert is configured.
func (r *PageResource) applyMarkdownInsert(ctx context.Context, plan *PageResourceModel) diag.Diagnostics {
	if plan.MarkdownInsert == nil {
		return nil
	}
	if plan.MarkdownInsert.Content.IsNull() || plan.MarkdownInsert.Content.IsUnknown() {
		return nil
	}

	_, err := r.mdClient.InsertPageMarkdown(
		ctx,
		plan.ID.ValueString(),
		plan.MarkdownInsert.Content.ValueString(),
		plan.MarkdownInsert.Position.ValueString(),
	)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Error inserting markdown into page", err.Error())
		return diags
	}
	return nil
}

func (r *PageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, err := tokenForClient(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error trashing page", err.Error())
		return
	}
	if err := trashObject(ctx, token, "pages", state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error trashing page", err.Error())
		return
	}
}

func (r *PageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
