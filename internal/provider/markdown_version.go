package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// markdownVersionResourceAttribute is the markdown_version attribute shared
// by notion_page and notion_database_entry. It is Optional and Computed so
// it can record the provider default that was used. It has no
// RequiresReplace. A change rewrites the page body in place instead.
func markdownVersionResourceAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		Description: "Markdown syntax used to write this resource's markdown:`v2` (Notion-flavored Markdown) or `v1` (enhanced markdown, deprecated by Notion). " +
			"Overrides the provider's markdown_version. When neither is set, the field is left out of requests and Notion uses v1. " +
			"Changing the effective version rewrites the page body once with the current markdown, so rewrite the markdown in the new syntax in the same change.",
		Optional: true,
		Computed: true,
		Validators: []validator.String{
			MarkdownVersionValidator(),
		},
	}
}

// modifyMarkdownVersionPlan sets the planned markdown_version from the
// configured value, falling back to the provider default for resources that
// manage markdown. With neither set it plans null, so state written before
// this attribute existed plans no diff.
func modifyMarkdownVersionPlan(ctx context.Context, mc *markdownClient, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var configured, markdown types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("markdown_version"), &configured)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("markdown"), &markdown)...)
	if resp.Diagnostics.HasError() {
		return
	}
	planned := plannedMarkdownVersion(configured, markdown, mc.providerMarkdownVersion())
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("markdown_version"), planned)...)
}
