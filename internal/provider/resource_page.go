package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var (
	_ resource.Resource                   = &PageResource{}
	_ resource.ResourceWithImportState    = &PageResource{}
	_ resource.ResourceWithValidateConfig = &PageResource{}
)

type PageResource struct {
	client   *notionapi.Client
	mdClient *markdownClient
}

type PageResourceModel struct {
	ID                types.String         `tfsdk:"id"`
	ParentPageID      types.String         `tfsdk:"parent_page_id"`
	Title             types.String         `tfsdk:"title"`
	URL               types.String         `tfsdk:"url"`
	Icon              types.String         `tfsdk:"icon"`
	IconName          types.String         `tfsdk:"icon_name"`
	IconColor         types.String         `tfsdk:"icon_color"`
	IconURL           types.String         `tfsdk:"icon_url"`
	IconCustomEmojiID types.String         `tfsdk:"icon_custom_emoji_id"`
	IsLocked          types.Bool           `tfsdk:"is_locked"`
	Position          types.String         `tfsdk:"position"`
	AfterBlockID      types.String         `tfsdk:"after_block_id"`
	Markdown          types.String         `tfsdk:"markdown"`
	MarkdownInsert    *MarkdownInsertModel `tfsdk:"markdown_insert"`
	TemplateID        types.String         `tfsdk:"template_id"`
	TemplateTimezone  types.String         `tfsdk:"template_timezone"`
}

func (m *PageResourceModel) iconFields() iconFields {
	return iconFields{Emoji: m.Icon, Name: m.IconName, Color: m.IconColor, URL: m.IconURL, CustomEmojiID: m.IconCustomEmojiID}
}

func (m *PageResourceModel) setIconFields(f iconFields) {
	m.Icon, m.IconName, m.IconColor, m.IconURL, m.IconCustomEmojiID = f.Emoji, f.Name, f.Color, f.URL, f.CustomEmojiID
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
				Description: "The ID of the parent page. Changes are applied via the 2026-01-15 " +
					"`POST /v1/pages/{id}/move` endpoint rather than recreating the resource.",
				Required: true,
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
				Description: "Emoji icon for the page. Defaults to an empty string, so an emoji set in the Notion UI is removed on the next apply unless it's configured here. " +
					"Conflicts with icon_name, icon_url, and icon_custom_emoji_id.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
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
				Description: "External image URL for the page icon. URLs from Notion's own icon set are rejected, use icon_name for those.",
				Optional:    true,
			},
			"icon_custom_emoji_id": schema.StringAttribute{
				Description: "ID of a custom emoji in the integration's workspace to use as the page icon.",
				Optional:    true,
			},
			"is_locked": schema.BoolAttribute{
				Description: "Whether the page is locked from editing in the Notion app. It does not block API updates. " +
					"When unset, the current value is reported and left alone.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"position": schema.StringAttribute{
				Description: "Where to create the page inside its parent: `page_start` or `page_end` (2026-01-15). " +
					"Only used when the page is created. Changing it later does not move the page.",
				Optional: true,
			},
			"after_block_id": schema.StringAttribute{
				Description: "Create the page directly after this block in the parent page. Conflicts with position. " +
					"Only used when the page is created.",
				Optional: true,
			},
			"markdown": schema.StringAttribute{
				Description: "Page content as enhanced markdown. Mutually exclusive with managing content via notion_block resources. " +
					"Note: Notion may normalize the markdown content, so the stored value may differ slightly from what was submitted.",
				Optional: true,
			},
			"template_id": schema.StringAttribute{
				Description: "Optional Notion template page ID to apply at creation (2026-01-15 API addition). " +
					"Once set, the template is applied asynchronously by Notion; the page is initially returned blank. " +
					"Changing this forces a new resource since templates are a creation-time concept.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"template_timezone": schema.StringAttribute{
				Description: "Optional IANA timezone (e.g. `America/New_York`) used when resolving template " +
					"variables. Only meaningful when `template_id` is set. Changing it forces a new resource.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
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

func (r *PageResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg PageResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateIconFields(cfg.iconFields(), &resp.Diagnostics)

	if setString(cfg.Position) {
		if p := cfg.Position.ValueString(); p != "page_start" && p != "page_end" {
			resp.Diagnostics.AddAttributeError(path.Root("position"), "Invalid position",
				fmt.Sprintf("%q is not a page position. Use page_start or page_end, or after_block_id to place it after a block.", p))
		}
		if setString(cfg.AfterBlockID) {
			resp.Diagnostics.AddAttributeError(path.Root("after_block_id"), "Conflicting position attributes",
				"Set either position or after_block_id, not both.")
		}
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

	hasTemplate := !plan.TemplateID.IsNull() || !plan.TemplateTimezone.IsNull()
	hasMarkdown := !plan.Markdown.IsNull() && !plan.Markdown.IsUnknown()

	switch {
	case hasTemplate && hasMarkdown:
		resp.Diagnostics.AddError(
			"template and markdown are mutually exclusive at create time",
			"Notion disallows the children parameter (and by extension the markdown body) when a "+
				"template is being applied. Use markdown_insert after create if you need to add content "+
				"to a template-instantiated page.",
		)
		return
	case hasTemplate:
		r.createWithTemplate(ctx, &plan, resp)
	case hasMarkdown:
		r.createWithMarkdown(ctx, &plan, resp)
	default:
		r.createWithoutMarkdown(ctx, &plan, resp)
	}
	if plan.ID.IsUnknown() || plan.ID.ValueString() == "" {
		return
	}

	if !resp.Diagnostics.HasError() {
		r.applyCreateExtras(ctx, &plan, &resp.Diagnostics)
	}
	// The page exists from here on, so save it even when a later step
	// failed. With an error, Terraform marks it tainted and replaces it on
	// the next apply. Returning without state would orphan the page and the
	// next apply would create a duplicate.
	if plan.IsLocked.IsUnknown() {
		plan.IsLocked = types.BoolValue(false)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PageResource) position(plan *PageResourceModel) map[string]interface{} {
	return pagePositionRequest(plan.Position.ValueString(), plan.AfterBlockID.ValueString())
}

// applyCreateExtras sets a non-emoji icon and is_locked after the page
// exists, on Notion-Version 2026-03-11. Every create path shares it, so the
// template, markdown, and plain paths don't each need their own icon logic.
// Locking runs last, after any markdown_insert.
func (r *PageResource) applyCreateExtras(ctx context.Context, plan *PageResourceModel, diags *diag.Diagnostics) {
	lockSet := !plan.IsLocked.IsNull() && !plan.IsLocked.IsUnknown()
	if !plan.iconFields().managed() && !lockSet {
		// Nothing to change. A new page starts unlocked, so skip the extra
		// request.
		plan.IsLocked = types.BoolValue(false)
		return
	}
	token, err := tokenForClient(r.client)
	if err != nil {
		diags.AddError("Error setting page icon or lock", err.Error())
		return
	}

	var icon map[string]interface{}
	if plan.iconFields().managed() {
		icon = plan.iconFields().request()
	}
	extras, err := patchContainerExtras(ctx, token, "pages", plan.ID.ValueString(), icon, false, plan.IsLocked)
	if err != nil {
		diags.AddError("Error setting page icon or lock", err.Error())
		return
	}
	plan.IsLocked = types.BoolValue(extras.IsLocked)
}

func (r *PageResource) createWithTemplate(ctx context.Context, plan *PageResourceModel, resp *resource.CreateResponse) {
	token, err := tokenForClient(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error creating page with template", err.Error())
		return
	}

	pageID, pageURL, err := createPageWithTemplate(
		ctx,
		token,
		plan.ParentPageID.ValueString(),
		plan.Title.ValueString(),
		plan.TemplateID.ValueString(),
		plan.TemplateTimezone.ValueString(),
		r.position(plan),
	)
	if err != nil {
		resp.Diagnostics.AddError("Error creating page with template", err.Error())
		return
	}

	plan.ID = types.StringValue(normalizeID(pageID))
	plan.URL = types.StringValue(pageURL)

	// Notion returns the page blank initially and applies the template
	// asynchronously, so we can't trust the response for icon/title round-trip.
	// Keep plan values in state.
	if plan.Icon.IsNull() {
		plan.Icon = types.StringValue("")
	}

	if diags := r.applyMarkdownInsert(ctx, plan); diags != nil {
		resp.Diagnostics.Append(diags...)
	}
}

func (r *PageResource) createWithMarkdown(ctx context.Context, plan *PageResourceModel, resp *resource.CreateResponse) {
	pageID, pageURL, err := r.mdClient.CreatePageWithMarkdownAndTitle(
		ctx,
		plan.ParentPageID.ValueString(),
		plan.Title.ValueString(),
		plan.Markdown.ValueString(),
		r.position(plan),
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
}

func (r *PageResource) createWithoutMarkdown(ctx context.Context, plan *PageResourceModel, resp *resource.CreateResponse) {
	// The SDK has no position field, so a configured position goes over
	// direct HTTP. Everything else stays on the SDK path as before.
	if position := r.position(plan); position != nil {
		token, err := tokenForClient(r.client)
		if err != nil {
			resp.Diagnostics.AddError("Error creating page", err.Error())
			return
		}
		var icon map[string]interface{}
		if plan.Icon.ValueString() != "" {
			icon = iconFields{Emoji: plan.Icon}.request()
		}
		pageID, pageURL, err := createPagePlain(ctx, token, plan.ParentPageID.ValueString(), plan.Title.ValueString(), icon, position)
		if err != nil {
			resp.Diagnostics.AddError("Error creating page", err.Error())
			return
		}
		plan.ID = types.StringValue(normalizeID(pageID))
		plan.URL = types.StringValue(pageURL)
		if plan.Icon.IsNull() {
			plan.Icon = types.StringValue("")
		}
	} else {
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
	}

	if diags := r.applyMarkdownInsert(ctx, plan); diags != nil {
		resp.Diagnostics.Append(diags...)
	}
}

// Read fetches the page on Notion-Version 2026-03-11 in one call. That
// version returns native icons, custom emoji, and is_locked, which the SDK's
// 2022-06-28 Page type drops.
func (r *PageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, err := tokenForClient(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error reading page", err.Error())
		return
	}
	page, err := getPageRaw(ctx, token, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading page", err.Error())
		return
	}

	if page.InTrash {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(normalizeID(page.ID))
	state.URL = types.StringValue(page.URL)
	state.IsLocked = types.BoolValue(page.IsLocked)

	if page.Parent.Type == "page_id" {
		state.ParentPageID = types.StringValue(normalizeID(page.Parent.PageID))
	} else {
		// 2026-05-11: pages can now be parented by an agent ({"type": "agent_id"}).
		// Anything other than page_id falls through here. Surface a warning so
		// the user notices the parent moved out from under Terraform instead of
		// silently keeping the old parent_page_id in state.
		resp.Diagnostics.AddWarning(
			"Page parent type changed",
			fmt.Sprintf("Page %s now has parent type %q (Terraform-managed pages expect page_id). "+
				"State retains the previously known parent_page_id; you may need to recreate this "+
				"resource or reparent the page in Notion to keep state and reality in sync. "+
				"agent_id parents (2026-05-11) are not yet manageable through this provider.",
				state.ID.ValueString(), page.Parent.Type),
		)
	}

	if title, ok := page.title(); ok {
		state.Title = types.StringValue(title)
	}

	state.setIconFields(state.iconFields().reconcile(parseIcon(page.Icon)))

	// Markdown is managed by the user's config; we don't read it back from the
	// API to avoid perpetual diffs caused by Notion's content normalization.

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *PageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config PageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, err := tokenForClient(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error updating page", err.Error())
		return
	}

	// If the parent_page_id changed, move the page first (2026-01-15 move endpoint).
	// Done before the title/icon Update so the rest of the update lands on the
	// page already at its new location.
	if plan.ParentPageID.ValueString() != state.ParentPageID.ValueString() {
		if err := movePage(ctx, token, plan.ID.ValueString(), plan.ParentPageID.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error moving page", err.Error())
			return
		}
	}

	if plan.Position.ValueString() != state.Position.ValueString() || plan.AfterBlockID.ValueString() != state.AfterBlockID.ValueString() {
		resp.Diagnostics.AddWarning("Page position is only used at creation",
			"position and after_block_id only place a page when it is created. The new value is stored, but the existing page does not move.")
	}

	// One PATCH on 2026-03-11 for title, icon, and lock. The SDK path can't
	// send native icons, custom emoji, or is_locked.
	body := map[string]interface{}{
		"properties": map[string]interface{}{
			"title": map[string]interface{}{
				"title": plainToRichText(plan.Title.ValueString()),
			},
		},
	}
	if icon := plan.iconFields().request(); icon != nil {
		body["icon"] = icon
	} else if state.Icon.ValueString() != "" || state.iconFields().managed() {
		// Every icon attribute was removed from the config, so clear it.
		body["icon"] = nil
	}
	if !config.IsLocked.IsNull() && !config.IsLocked.IsUnknown() {
		body["is_locked"] = config.IsLocked.ValueBool()
	}

	page, err := updatePageRaw(ctx, token, plan.ID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Error updating page", err.Error())
		return
	}

	plan.URL = types.StringValue(page.URL)
	plan.IsLocked = types.BoolValue(page.IsLocked)
	if p := parseIcon(page.Icon); p.Type == "emoji" {
		plan.Icon = types.StringValue(p.Emoji)
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
