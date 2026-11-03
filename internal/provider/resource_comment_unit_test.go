package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// After create, a comment made with parent_page_id also has discussion_id
// in state. Updating its body must not trip the parent check.
func TestCommentValidateBody_UpdateWithBothParents(t *testing.T) {
	r := &CommentResource{}
	plan := CommentResourceModel{
		ParentPageID: types.StringValue("page-1"),
		DiscussionID: types.StringValue("discussion-1"),
		RichText:     types.StringValue("edited"),
		Markdown:     types.StringNull(),
	}
	var d diag.Diagnostics
	if !r.validateBody(plan, false, &d) {
		t.Fatalf("update with both parents set was rejected: %v", d)
	}
	d = nil
	if r.validateBody(plan, true, &d) {
		t.Fatal("create with both parents set was accepted")
	}
}
