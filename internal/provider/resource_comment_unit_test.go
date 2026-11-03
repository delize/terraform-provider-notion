package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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

// plain_text and last_edited_time change when the body is edited. Keeping
// the prior value in the plan makes Terraform reject the update result.
func TestCommentSchema_UpdatedFieldsStayUnknown(t *testing.T) {
	var resp resource.SchemaResponse
	(&CommentResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	for _, name := range []string{"plain_text", "last_edited_time"} {
		attr, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s is missing or not a string attribute", name)
		}
		if len(attr.PlanModifiers) != 0 {
			t.Errorf("%s has plan modifiers, so its old value would be planned for an update", name)
		}
	}
}
