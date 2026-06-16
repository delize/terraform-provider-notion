package provider

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// precheckMeetingNotesAvailable issues a probe POST to the meeting notes query
// endpoint and returns false if the workspace's plan doesn't include AI
// meeting notes. Notion returns 400 with code "validation_error" and a
// message containing "AI meeting notes" for plan-gated workspaces. We treat
// that as "skip, not fail" — the data source itself still returns an error in
// that case, which is the right behavior for a real user who configured it.
func precheckMeetingNotesAvailable(t *testing.T) bool {
	t.Helper()
	token := os.Getenv("NOTION_TOKEN")
	if token == "" {
		return false
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		notionAPIBaseURL+"/blocks/meeting_notes/query",
		bytes.NewReader([]byte("{}")),
	)
	if err != nil {
		t.Fatalf("building meeting notes probe request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", notionTrashAPIVersion)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("probing meeting notes endpoint: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true
	}
	if resp.StatusCode == 400 && strings.Contains(string(body), "AI meeting notes") {
		return false
	}
	// Any other failure mode is a real problem — let the test surface it.
	t.Fatalf("unexpected probe response status %d: %s", resp.StatusCode, string(body))
	return false
}

// TestAccMeetingNotesDataSource is a smoke test against the 2026-05-11
// POST /v1/blocks/meeting_notes/query endpoint. The workspace may have zero
// meeting notes available to the integration — that's fine, we only assert
// that the call succeeds and raw_json is populated (it'll be the literal
// response, which still contains at least `{"results":[]}`).
//
// Skips when the workspace's plan doesn't include AI meeting notes (the
// endpoint returns 400 in that case — not a provider bug, just a plan gate).
func TestAccMeetingNotesDataSource(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}
	if !precheckMeetingNotesAvailable(t) {
		t.Skip("workspace plan does not include AI meeting notes; endpoint returned 400 validation_error")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "notion_meeting_notes" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.notion_meeting_notes.all", "raw_json"),
					// results may be empty; the attribute itself must be set.
					resource.TestCheckResourceAttrSet("data.notion_meeting_notes.all", "results.#"),
				),
			},
		},
	})
}

// TestAccMeetingNotesDataSource_WithLimit exercises the limit parameter to
// make sure the JSON request body is encoded correctly. Still tolerates an
// empty workspace. Same plan-gate skip applies.
func TestAccMeetingNotesDataSource_WithLimit(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}
	if !precheckMeetingNotesAvailable(t) {
		t.Skip("workspace plan does not include AI meeting notes; endpoint returned 400 validation_error")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "notion_meeting_notes" "limited" {
  limit = 5
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.notion_meeting_notes.limited", "raw_json"),
				),
			},
		},
	})
}
