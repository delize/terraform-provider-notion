package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
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

// Refresh reads a comment by ID. A reply has no parent_page_id, so its
// anchor and property_id come from the response.
func TestGetCommentAndApplyToReply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/comments/c-1":
			_, _ = w.Write([]byte(`{"object":"comment","id":"c-1","discussion_id":"d-1","created_time":"2026-11-03T10:00:00.000Z",
				"last_edited_time":"2026-11-03T10:00:00.000Z","created_by":{"id":"bot-1"},
				"parent":{"type":"page_id","page_id":"aaaa-bbbb"},"property_id":"%3AUPp",
				"rich_text":[{"type":"text","plain_text":"Reply text"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"object":"error","status":404,"code":"object_not_found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	prev := notionAPIBaseURL
	notionAPIBaseURL = srv.URL + "/v1"
	t.Cleanup(func() { notionAPIBaseURL = prev })

	if _, found, err := getComment(context.Background(), "t", "gone"); err != nil || found {
		t.Fatalf("deleted comment: found=%v err=%v", found, err)
	}
	c, found, err := getComment(context.Background(), "t", "c-1")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	state := CommentResourceModel{
		ParentPageID:  types.StringNull(),
		DiscussionID:  types.StringValue("d-1"),
		AnchorBlockID: types.StringValue(""),
	}
	(&CommentResource{}).applyResponseToState(&state, c)
	if state.AnchorBlockID.ValueString() != "aaaabbbb" || state.PropertyID.ValueString() != "%3AUPp" || state.PlainText.ValueString() != "Reply text" {
		t.Fatalf("anchor=%q property_id=%q plain_text=%q", state.AnchorBlockID.ValueString(), state.PropertyID.ValueString(), state.PlainText.ValueString())
	}
}
