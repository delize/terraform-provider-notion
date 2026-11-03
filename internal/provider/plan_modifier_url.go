package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// urlFollowsTitle keeps the prior url in the plan unless the title changes.
// Notion puts a slug of the title in page URLs, so a rename changes the URL
// and the plan has to leave it unknown.
func urlFollowsTitle() planmodifier.String {
	return urlFollowsTitleModifier{}
}

type urlFollowsTitleModifier struct{}

func (urlFollowsTitleModifier) Description(context.Context) string {
	return "Keeps the prior URL unless the title changes."
}

func (m urlFollowsTitleModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (urlFollowsTitleModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || !req.PlanValue.IsUnknown() {
		return
	}
	var planTitle, stateTitle types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("title"), &planTitle)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("title"), &stateTitle)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if planTitle.Equal(stateTitle) {
		resp.PlanValue = req.StateValue
	}
}
