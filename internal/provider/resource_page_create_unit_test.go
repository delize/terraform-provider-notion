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

// fakePages is a minimal in-memory /v1/pages API for create, read, patch,
// and trash. A PATCH whose icon name is "piza" fails with a 400, like a
// typo in icon_name does against Notion.
type fakePages struct {
	mu      sync.Mutex
	next    int
	pages   map[string]map[string]interface{}
	created int
	trashed int
	gets    int
}

func newFakePages(t *testing.T) *fakePages {
	t.Helper()
	f := &fakePages{pages: map[string]map[string]interface{}{}}
	srv := httptest.NewServer(f)
	prev := notionAPIBaseURL
	notionAPIBaseURL = srv.URL + "/v1"
	t.Cleanup(func() {
		notionAPIBaseURL = prev
		srv.Close()
	})
	return f
}

func (f *fakePages) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&body)
	w.Header().Set("Content-Type", "application/json")
	reply := func(status int, v interface{}) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}

	id := strings.TrimPrefix(r.URL.Path, "/v1/pages/")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/pages":
		f.next++
		f.created++
		id := fmt.Sprintf("0000000000000000000000000000000%d", f.next)
		parent := body["parent"].(map[string]interface{})
		props := body["properties"].(map[string]interface{})
		for _, rt := range props["title"].(map[string]interface{})["title"].([]interface{}) {
			rt := rt.(map[string]interface{})
			rt["plain_text"] = rt["text"].(map[string]interface{})["content"]
		}
		page := map[string]interface{}{
			"object":     "page",
			"id":         id,
			"url":        "https://www.notion.so/" + id,
			"in_trash":   false,
			"is_locked":  false,
			"parent":     map[string]interface{}{"type": "page_id", "page_id": parent["page_id"]},
			"properties": props,
			"icon":       body["icon"],
		}
		f.pages[id] = page
		reply(200, page)
	case r.Method == http.MethodGet && f.pages[id] != nil:
		f.gets++
		reply(200, f.pages[id])
	case r.Method == http.MethodPatch && f.pages[id] != nil:
		page := f.pages[id]
		if body["in_trash"] == true {
			f.trashed++
			page["in_trash"] = true
			reply(200, page)
			return
		}
		if icon, ok := body["icon"].(map[string]interface{}); ok {
			if native, ok := icon["icon"].(map[string]interface{}); ok && native["name"] == "piza" {
				reply(400, map[string]string{"object": "error", "code": "validation_error", "message": "Invalid icon name."})
				return
			}
		}
		if v, ok := body["icon"]; ok {
			page["icon"] = v
		}
		if v, ok := body["is_locked"]; ok {
			page["is_locked"] = v
		}
		reply(200, page)
	default:
		reply(404, map[string]string{"object": "error", "code": "object_not_found", "message": r.Method + " " + r.URL.Path})
	}
}

func pageCreateConfig(iconName string) string {
	return fmt.Sprintf(`
provider "notion" {
  token = "secret_unit_page"
}

resource "notion_page" "p" {
  parent_page_id = "11111111111111111111111111111111"
  title          = "Runbook"
  position       = "page_end"
  icon_name      = %q
}
`, iconName)
}

// A failed icon PATCH after the page exists must still save the page, so
// the next apply replaces it instead of creating a second one next to an
// orphan.
func TestUnitPage_CreateExtrasFailureKeepsState(t *testing.T) {
	f := newFakePages(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      pageCreateConfig("piza"),
				ExpectError: regexp.MustCompile(`Error setting page icon or lock`),
			},
			{
				Config: pageCreateConfig("pizza"),
				Check:  resource.TestCheckResourceAttr("notion_page.p", "icon_name", "pizza"),
			},
		},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.created != 2 || f.trashed != 2 {
		t.Fatalf("created %d pages and trashed %d, want the tainted first page replaced (2 created, both trashed by the end)", f.created, f.trashed)
	}
}

// With no native icon and no is_locked, create shouldn't need a second
// request to learn the lock state.
func TestUnitPage_PlainCreateSkipsExtrasRequest(t *testing.T) {
	f := newFakePages(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "notion" {
  token = "secret_unit_page"
}

resource "notion_page" "p" {
  parent_page_id = "11111111111111111111111111111111"
  title          = "Plain"
  position       = "page_start"
}
`,
				Check: resource.TestCheckResourceAttr("notion_page.p", "is_locked", "false"),
			},
		},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.created != 1 {
		t.Fatalf("created %d pages, want 1", f.created)
	}
}
