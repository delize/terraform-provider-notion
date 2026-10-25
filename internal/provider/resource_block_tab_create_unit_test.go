package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// fakeTabBlocks serves just enough of /v1/blocks for a tab block. While
// hideChildren is set, listing a tab's children returns nothing, which
// makes the provider's item count check fail after the block was created.
type fakeTabBlocks struct {
	mu           sync.Mutex
	next         int
	blocks       map[string]map[string]interface{}
	children     map[string][]map[string]interface{}
	hideChildren bool
	created      int
	deleted      int
}

func newFakeTabBlocks(t *testing.T) *fakeTabBlocks {
	t.Helper()
	f := &fakeTabBlocks{blocks: map[string]map[string]interface{}{}, children: map[string][]map[string]interface{}{}}
	srv := httptest.NewServer(f)
	prev := notionAPIBaseURL
	notionAPIBaseURL = srv.URL + "/v1"
	t.Cleanup(func() {
		notionAPIBaseURL = prev
		srv.Close()
	})
	return f
}

func (f *fakeTabBlocks) id() string {
	f.next++
	return fmt.Sprintf("%032d", f.next)
}

func (f *fakeTabBlocks) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&body)
	w.Header().Set("Content-Type", "application/json")
	reply := func(status int, v interface{}) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/blocks/")
	id, sub, _ := strings.Cut(rest, "/")

	switch {
	case r.Method == http.MethodPatch && sub == "children":
		in := body["children"].([]interface{})[0].(map[string]interface{})
		tabID := f.id()
		f.created++
		var kids []map[string]interface{}
		for _, c := range in["tab"].(map[string]interface{})["children"].([]interface{}) {
			c := c.(map[string]interface{})
			p := c["paragraph"].(map[string]interface{})
			for _, rt := range p["rich_text"].([]interface{}) {
				rt := rt.(map[string]interface{})
				rt["plain_text"] = rt["text"].(map[string]interface{})["content"]
			}
			kids = append(kids, map[string]interface{}{
				"object": "block", "id": f.id(), "type": "paragraph", "paragraph": p,
				"parent": map[string]string{"type": "block_id", "block_id": tabID},
			})
		}
		block := map[string]interface{}{
			"object": "block", "id": tabID, "type": "tab", "has_children": true,
			"parent": map[string]string{"type": "page_id", "page_id": id},
		}
		f.blocks[tabID] = block
		f.children[tabID] = kids
		reply(200, map[string]interface{}{"results": []interface{}{block}})
	case r.Method == http.MethodGet && sub == "children":
		kids := f.children[id]
		if f.hideChildren {
			kids = nil
		}
		reply(200, map[string]interface{}{"results": kids, "has_more": false})
	case r.Method == http.MethodGet && f.blocks[id] != nil:
		reply(200, f.blocks[id])
	case r.Method == http.MethodDelete && f.blocks[id] != nil:
		f.deleted++
		f.blocks[id]["in_trash"] = true
		reply(200, f.blocks[id])
	default:
		reply(404, map[string]string{"object": "error", "code": "object_not_found", "message": r.Method + " " + r.URL.Path})
	}
}

const tabCreateConfig = `
provider "notion" {
  token = "secret_unit_tab"
}

resource "notion_block" "t" {
  parent_id = "11111111111111111111111111111111"
  type      = "tab"
  tab_items = [
    { label = "Overview", icon = "📋" },
    { label = "Details", icon = "📎" },
  ]
}
`

// A tab whose items can't be read back after create must still be saved,
// so the next apply replaces it rather than leaving it orphaned.
func TestUnitBlockTab_CreateItemReadFailureKeepsState(t *testing.T) {
	f := newFakeTabBlocks(t)
	f.hideChildren = true
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      tabCreateConfig,
				ExpectError: regexp.MustCompile(`Error reading new tab items`),
			},
			{
				PreConfig: func() {
					f.mu.Lock()
					f.hideChildren = false
					f.mu.Unlock()
				},
				Config: tabCreateConfig,
				Check:  resource.TestCheckResourceAttr("notion_block.t", "tab_items.#", "2"),
			},
		},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.created != 2 || f.deleted != 2 {
		t.Fatalf("created %d tabs and deleted %d, want the tainted first tab replaced (2 created, both deleted by the end)", f.created, f.deleted)
	}
}
