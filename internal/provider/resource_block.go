package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var (
	_ resource.Resource                   = &BlockResource{}
	_ resource.ResourceWithImportState    = &BlockResource{}
	_ resource.ResourceWithValidateConfig = &BlockResource{}
)

type BlockResource struct {
	client *notionapi.Client
}

type BlockResourceModel struct {
	ID           types.String `tfsdk:"id"`
	ParentID     types.String `tfsdk:"parent_id"`
	Type         types.String `tfsdk:"type"`
	After        types.String `tfsdk:"after"`
	HasChildren  types.Bool   `tfsdk:"has_children"`
	RichText     types.String `tfsdk:"rich_text"`
	RichTextJSON types.String `tfsdk:"rich_text_json"`
	Color        types.String `tfsdk:"color"`
	IsToggleable types.Bool   `tfsdk:"is_toggleable"`
	Checked      types.Bool   `tfsdk:"checked"`
	Icon         types.String `tfsdk:"icon"`
	Language     types.String `tfsdk:"language"`
	Caption      types.String `tfsdk:"caption"`
	URL          types.String `tfsdk:"url"`
	Expression   types.String `tfsdk:"expression"`
	SyncedFrom   types.String `tfsdk:"synced_from"`
	TabItems     types.List   `tfsdk:"tab_items"`
}

func NewBlockResource() resource.Resource {
	return &BlockResource{}
}

func (r *BlockResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_block"
}

func (r *BlockResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a content block on a Notion page or inside another block.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the block.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"parent_id": schema.StringAttribute{
				Description: "The ID of the parent page or block.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Description: "The block type (e.g. paragraph, heading_1, heading_4, code, tab, etc.).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					BlockTypeValidator(),
				},
			},
			"after": schema.StringAttribute{
				Description: "Insert this block after the specified block ID. If omitted, appends to the end.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"has_children": schema.BoolAttribute{
				Description: "Whether this block has child blocks.",
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"rich_text": schema.StringAttribute{
				Description: "Text content of the block. Supports markdown links: [text](url).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"rich_text_json": schema.StringAttribute{
				Description: "JSON-encoded array of Notion rich text objects. When set, takes precedence over rich_text.",
				Optional:    true,
			},
			"color": schema.StringAttribute{
				Description: "Block color (e.g. default, red, blue_background).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
				Validators: []validator.String{
					BlockColorValidator(),
				},
			},
			"is_toggleable": schema.BoolAttribute{
				Description: "Whether a heading block (heading_1 to heading_4) is toggleable.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"checked": schema.BoolAttribute{
				Description: "Whether a to-do block is checked.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"icon": schema.StringAttribute{
				Description: "Emoji icon for callout blocks.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"language": schema.StringAttribute{
				Description: "Programming language for code blocks.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"caption": schema.StringAttribute{
				Description: "Caption text for code, bookmark, and image blocks.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"url": schema.StringAttribute{
				Description: "URL for bookmark, embed, and image blocks.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"expression": schema.StringAttribute{
				Description: "LaTeX expression for equation blocks.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"synced_from": schema.StringAttribute{
				Description: "Source block ID for synced block copies.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tab_items": schema.ListNestedAttribute{
				Description: "Tabs of a tab block, in order. Required when type is tab, and a tab needs at least one. " +
					"Each tab item is a paragraph block, so put a tab's content in it with notion_block resources whose parent_id is the item's id. " +
					"Items are matched by position: changing a label or icon updates that tab in place, new items are added at the end, " +
					"and removing items from the end deletes those tabs and their content.",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "Block ID of the tab item. Use it as parent_id for the tab's content.",
							Computed:    true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"label": schema.StringAttribute{
							Description: "Tab label. Supports markdown links: [text](url).",
							Required:    true,
						},
						"icon": schema.StringAttribute{
							Description: "Emoji shown next to the label.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString(""),
						},
					},
				},
			},
		},
	}
}

// ValidateConfig checks the tab-specific rules: tab_items is required for
// a tab and only valid there, and a tab has no text or color of its own.
func (r *BlockResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg BlockResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Type.IsUnknown() || cfg.Type.IsNull() {
		return
	}

	if cfg.Type.ValueString() != "tab" {
		if !cfg.TabItems.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("tab_items"), "tab_items not allowed",
				fmt.Sprintf("tab_items only applies to blocks of type tab, not %q.", cfg.Type.ValueString()))
		}
		return
	}

	if cfg.TabItems.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("tab_items"), "Missing tab_items",
			"A tab block needs at least one entry in tab_items.")
	} else if !cfg.TabItems.IsUnknown() && len(cfg.TabItems.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(path.Root("tab_items"), "Empty tab_items",
			"Notion requires at least one tab item when a tab block exists.")
	}

	for name, v := range map[string]attr.Value{
		"rich_text":      cfg.RichText,
		"rich_text_json": cfg.RichTextJSON,
		"color":          cfg.Color,
		"icon":           cfg.Icon,
	} {
		if !v.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root(name), "Attribute not used by tab blocks",
				fmt.Sprintf("%s has no effect on a tab block. Set labels and icons in tab_items.", name))
		}
	}
}

func (r *BlockResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *BlockResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan BlockResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if isRawBlockType(plan.Type.ValueString()) {
		resp.Diagnostics.Append(r.createRawBlock(ctx, &plan)...)
		// Once the block exists, save it even if reading its tab items
		// failed, so Terraform taints it rather than losing track of it.
		if plan.ID.IsUnknown() || plan.ID.ValueString() == "" {
			return
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	block, err := buildBlockForCreate(plan)
	if err != nil {
		resp.Diagnostics.AddError("Error building block", err.Error())
		return
	}

	parentID := notionapi.BlockID(plan.ParentID.ValueString())
	appendReq := &notionapi.AppendBlockChildrenRequest{
		Children: []notionapi.Block{block},
	}
	if !plan.After.IsNull() && !plan.After.IsUnknown() {
		appendReq.After = notionapi.BlockID(plan.After.ValueString())
	}

	result, err := r.client.Block.AppendChildren(ctx, parentID, appendReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating block", err.Error())
		return
	}

	if len(result.Results) == 0 {
		resp.Diagnostics.AddError("Error creating block", "No block returned from Notion API")
		return
	}

	created := result.Results[0]
	readBlockIntoState(created, &plan)

	// Preserve the after value from the plan (it's not returned by the API)
	if !plan.After.IsNull() && !plan.After.IsUnknown() {
		// keep it
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BlockResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state BlockResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// heading_4 and tab go over direct HTTP. After an import the type isn't
	// known yet, so fetch the raw block once to find out.
	if state.Type.IsNull() || state.Type.IsUnknown() || isRawBlockType(state.Type.ValueString()) {
		token, err := tokenForClient(r.client)
		if err != nil {
			resp.Diagnostics.AddError("Error reading block", err.Error())
			return
		}
		raw, err := getBlockRaw(ctx, token, state.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading block", err.Error())
			return
		}
		if isRawBlockType(raw.Type) {
			after := state.After
			found, diags := r.readRawBlock(ctx, token, raw, &state)
			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}
			if !found {
				resp.State.RemoveResource(ctx)
				return
			}
			state.After = after
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	block, err := r.client.Block.Get(ctx, notionapi.BlockID(state.ID.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading block", err.Error())
		return
	}

	if block.GetArchived() {
		resp.State.RemoveResource(ctx)
		return
	}

	// Preserve after from state since the API doesn't return it
	after := state.After
	syncedFrom := state.SyncedFrom

	readBlockIntoState(block, &state)

	state.After = after
	// Preserve synced_from if it wasn't set by readBlockIntoState
	if state.SyncedFrom.IsNull() || state.SyncedFrom.IsUnknown() {
		state.SyncedFrom = syncedFrom
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *BlockResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan BlockResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if isRawBlockType(plan.Type.ValueString()) {
		var prior BlockResourceModel
		resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
		if resp.Diagnostics.HasError() {
			return
		}
		after := plan.After
		resp.Diagnostics.Append(r.updateRawBlock(ctx, &prior, &plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.After = after
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	updateReq, err := buildBlockUpdateRequest(plan)
	if err != nil {
		resp.Diagnostics.AddError("Error building block update", err.Error())
		return
	}

	updated, err := r.client.Block.Update(ctx, notionapi.BlockID(plan.ID.ValueString()), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Error updating block", err.Error())
		return
	}

	// Preserve after from plan
	after := plan.After
	syncedFrom := plan.SyncedFrom

	readBlockIntoState(updated, &plan)

	plan.After = after
	if plan.SyncedFrom.IsNull() || plan.SyncedFrom.IsUnknown() {
		plan.SyncedFrom = syncedFrom
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BlockResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state BlockResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if isRawBlockType(state.Type.ValueString()) {
		token, err := tokenForClient(r.client)
		if err == nil {
			err = deleteBlockRaw(ctx, token, state.ID.ValueString())
		}
		if err != nil {
			resp.Diagnostics.AddError("Error deleting block", err.Error())
		}
		return
	}

	_, err := r.client.Block.Delete(ctx, notionapi.BlockID(state.ID.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Error deleting block", err.Error())
		return
	}
}

func (r *BlockResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
