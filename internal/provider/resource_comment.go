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
	_ resource.Resource              = &CommentResource{}
	_ resource.ResourceWithConfigure = &CommentResource{}
)

type CommentResource struct {
	client *notionapi.Client
}

type CommentResourceModel struct {
	ID             types.String `tfsdk:"id"`
	ParentPageID   types.String `tfsdk:"parent_page_id"`
	DiscussionID   types.String `tfsdk:"discussion_id"`
	RichText       types.String `tfsdk:"rich_text"`
	Markdown       types.String `tfsdk:"markdown"`
	PlainText      types.String `tfsdk:"plain_text"`
	AnchorBlockID  types.String `tfsdk:"anchor_block_id"`
	CreatedTime    types.String `tfsdk:"created_time"`
	LastEditedTime types.String `tfsdk:"last_edited_time"`
	CreatedBy      types.String `tfsdk:"created_by"`
}

func NewCommentResource() resource.Resource {
	return &CommentResource{}
}

func (r *CommentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_comment"
}

func (r *CommentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a comment on a Notion page or within an existing discussion thread. " +
			"Only comments created by this integration can be updated or deleted (a Notion API constraint).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The comment ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"parent_page_id": schema.StringAttribute{
				Description: "ID of the page to comment on. Creates a new discussion thread on that page. " +
					"Mutually exclusive with `discussion_id`.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"discussion_id": schema.StringAttribute{
				Description: "ID of an existing discussion thread to reply to. " +
					"Mutually exclusive with `parent_page_id`.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"rich_text": schema.StringAttribute{
				Description: "Plain-text body of the comment. Mutually exclusive with `markdown`. " +
					"Use this for unformatted content; the body is sent as a single rich_text text segment.",
				Optional: true,
			},
			"markdown": schema.StringAttribute{
				Description: "Markdown body of the comment. Mutually exclusive with `rich_text`. " +
					"**Inline formatting only** — Notion documents that fenced code blocks, headings, lists, " +
					"tables, and blockquotes do not render as structured blocks in comments. " +
					"Supported inline: bold, italic, strikethrough, code, links, inline equations, mentions.",
				Optional: true,
			},
			"plain_text": schema.StringAttribute{
				Description: "Server-rendered plain-text representation of the comment body. " +
					"Computed after create/update.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"anchor_block_id": schema.StringAttribute{
				Description: "The block ID (page or block) that this comment's discussion is anchored to. " +
					"Used internally to look the comment back up during refresh.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_time": schema.StringAttribute{
				Description: "RFC3339 timestamp of when the comment was created.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"last_edited_time": schema.StringAttribute{
				Description: "RFC3339 timestamp of the most recent edit.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_by": schema.StringAttribute{
				Description: "ID of the user/bot that created the comment.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *CommentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// validateBody enforces "exactly one of parent_page_id/discussion_id" and
// "exactly one of rich_text/markdown" at the resource boundary. The Notion
// API also rejects invalid combos, but doing it locally surfaces clearer
// errors than the API's validation message.
func (r *CommentResource) validateBody(plan CommentResourceModel, diag interface{ AddError(string, string) }) bool {
	hasParent := plan.ParentPageID.ValueString() != ""
	hasDiscussion := plan.DiscussionID.ValueString() != "" && !plan.DiscussionID.IsUnknown()
	if hasParent == hasDiscussion {
		diag.AddError("Invalid comment parent",
			"Exactly one of `parent_page_id` or `discussion_id` must be set.")
		return false
	}
	hasRichText := plan.RichText.ValueString() != ""
	hasMarkdown := plan.Markdown.ValueString() != ""
	if hasRichText == hasMarkdown {
		diag.AddError("Invalid comment body",
			"Exactly one of `rich_text` or `markdown` must be set.")
		return false
	}
	return true
}

func (r *CommentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan CommentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.validateBody(plan, &resp.Diagnostics) {
		return
	}

	token, err := tokenForClient(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error creating comment", err.Error())
		return
	}

	created, err := createComment(ctx, token,
		plan.ParentPageID.ValueString(),
		plan.DiscussionID.ValueString(),
		plan.RichText.ValueString(),
		plan.Markdown.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Error creating comment", err.Error())
		return
	}

	r.applyResponseToState(&plan, created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CommentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state CommentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	anchor := state.AnchorBlockID.ValueString()
	if anchor == "" {
		anchor = state.ParentPageID.ValueString()
	}
	if anchor == "" {
		// Nothing to query against — can't refresh.
		return
	}

	wantID := state.ID.ValueString()
	var cursor notionapi.Cursor
	for {
		page, err := r.client.Comment.Get(ctx, notionapi.BlockID(anchor), &notionapi.Pagination{
			StartCursor: cursor,
			PageSize:    100,
		})
		if err != nil {
			resp.Diagnostics.AddError("Error refreshing comment", err.Error())
			return
		}
		for _, c := range page.Results {
			if string(c.ID) == wantID {
				state.LastEditedTime = types.StringValue(c.LastEditedTime.Format("2006-01-02T15:04:05.000Z"))
				state.CreatedTime = types.StringValue(c.CreatedTime.Format("2006-01-02T15:04:05.000Z"))
				state.CreatedBy = types.StringValue(string(c.CreatedBy.ID))
				state.DiscussionID = types.StringValue(string(c.DiscussionID))
				// Refresh plain_text from server's rich_text array.
				plain := ""
				for _, rt := range c.RichText {
					plain += rt.PlainText
				}
				state.PlainText = types.StringValue(plain)
				resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
				return
			}
		}
		if !page.HasMore {
			break
		}
		cursor = notionapi.Cursor(page.NextCursor)
	}

	// Comment not found under its anchor — assume it was deleted externally.
	resp.State.RemoveResource(ctx)
}

func (r *CommentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state CommentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.validateBody(plan, &resp.Diagnostics) {
		return
	}

	token, err := tokenForClient(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error updating comment", err.Error())
		return
	}

	updated, err := updateComment(ctx, token, state.ID.ValueString(),
		plan.RichText.ValueString(), plan.Markdown.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating comment", err.Error())
		return
	}

	// Preserve the user's parent_page_id / discussion_id intent from state
	// (the update endpoint doesn't change those).
	plan.ID = state.ID
	plan.AnchorBlockID = state.AnchorBlockID
	plan.ParentPageID = state.ParentPageID
	r.applyResponseToState(&plan, updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CommentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state CommentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, err := tokenForClient(r.client)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting comment", err.Error())
		return
	}
	if err := deleteComment(ctx, token, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting comment", err.Error())
		return
	}
}

func (r *CommentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applyResponseToState merges the create/update response into the plan state.
// Preserves rich_text/markdown as the user submitted them (they're write-only
// in spirit; we don't try to round-trip them through Notion's normalized
// rich_text array).
func (r *CommentResource) applyResponseToState(state *CommentResourceModel, resp *commentResponse) {
	state.ID = types.StringValue(resp.ID)
	state.DiscussionID = types.StringValue(resp.DiscussionID)
	state.CreatedTime = types.StringValue(resp.CreatedTime)
	state.LastEditedTime = types.StringValue(resp.LastEditedTime)
	state.CreatedBy = types.StringValue(resp.CreatedBy.ID)
	state.PlainText = types.StringValue(richTextPlainFromShim(resp.RichText))

	// Anchor is the block the comment's discussion lives on. For a fresh
	// comment created with parent_page_id, that's the page itself. For a
	// reply to an existing discussion, we don't get the anchor directly in
	// the response — fall back to parent_page_id if set (will be empty for
	// the reply case, which is fine; Read just won't refresh those).
	if state.ParentPageID.ValueString() != "" {
		state.AnchorBlockID = state.ParentPageID
	} else if state.AnchorBlockID.IsUnknown() || state.AnchorBlockID.IsNull() {
		state.AnchorBlockID = types.StringValue("")
	}
}
