package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// mdCall is one request the markdown fake saw. Version is the
// markdown_version it carried, from the body on writes and from the query
// string on reads, "" when absent.
type mdCall struct {
	Method  string
	Path    string
	Type    string // replace_content or insert_content on PATCH markdown
	Version string
	HasKey  bool // whether markdown_version was present at all
}

// fakeMarkdownPages serves POST /v1/pages, GET and PATCH /v1/pages/{id},
// and GET and PATCH /v1/pages/{id}/markdown, enough for notion_page with
// markdown and the notion_page_markdown data source. warnings, when set, is
// returned on every PATCH markdown response.
type fakeMarkdownPages struct {
	mu       sync.Mutex
	next     int
	titles   map[string]string
	trashed  map[string]bool
	calls    []mdCall
	creates  []map[string]interface{} // POST /v1/pages bodies
	warnings []markdownWarning
}

func newFakeMarkdownPages(t *testing.T) *fakeMarkdownPages {
	t.Helper()
	f := &fakeMarkdownPages{titles: map[string]string{}, trashed: map[string]bool{}}
	srv := httptest.NewServer(f)
	prev := notionAPIBaseURL
	notionAPIBaseURL = srv.URL + "/v1"
	t.Cleanup(func() {
		notionAPIBaseURL = prev
		srv.Close()
	})
	return f
}

func (f *fakeMarkdownPages) page(id string) map[string]interface{} {
	return map[string]interface{}{
		"object":    "page",
		"id":        id,
		"url":       "https://www.notion.so/" + id,
		"in_trash":  f.trashed[id],
		"is_locked": false,
		"parent":    map[string]interface{}{"type": "page_id", "page_id": "11111111111111111111111111111111"},
		"properties": map[string]interface{}{
			"title": map[string]interface{}{
				"type":  "title",
				"title": []map[string]interface{}{{"type": "text", "plain_text": f.titles[id], "text": map[string]string{"content": f.titles[id]}}},
			},
		},
	}
}

func titleFromBody(body map[string]interface{}) (string, bool) {
	props, _ := body["properties"].(map[string]interface{})
	title, _ := props["title"].(map[string]interface{})
	parts, ok := title["title"].([]interface{})
	if !ok {
		return "", false
	}
	var b strings.Builder
	for _, p := range parts {
		text, _ := p.(map[string]interface{})["text"].(map[string]interface{})
		s, _ := text["content"].(string)
		b.WriteString(s)
	}
	return b.String(), true
}

func (f *fakeMarkdownPages) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&body)
	w.Header().Set("Content-Type", "application/json")
	reply := func(status int, v interface{}) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}

	call := mdCall{Method: r.Method, Path: r.URL.Path}
	if r.Method == http.MethodGet {
		call.Version = r.URL.Query().Get("markdown_version")
		call.HasKey = r.URL.Query().Has("markdown_version")
	} else if v, ok := body["markdown_version"]; ok {
		call.Version, _ = v.(string)
		call.HasKey = true
	}
	if t, ok := body["type"].(string); ok {
		call.Type = t
	}
	f.calls = append(f.calls, call)

	rest := strings.TrimPrefix(r.URL.Path, "/v1/pages/")
	id, sub, _ := strings.Cut(rest, "/")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/pages":
		f.next++
		id := fmt.Sprintf("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa%02d", f.next)
		f.titles[id], _ = titleFromBody(body)
		f.creates = append(f.creates, body)
		reply(200, f.page(id))
	case f.titles[id] == "" && !f.trashed[id]:
		reply(404, map[string]string{"object": "error", "code": "object_not_found", "message": r.Method + " " + r.URL.Path})
	case sub == "markdown" && r.Method == http.MethodGet:
		reply(200, map[string]interface{}{"object": "page_markdown", "id": id, "markdown": "# Hello", "truncated": false, "unknown_block_ids": []string{}})
	case sub == "markdown" && r.Method == http.MethodPatch:
		resp := map[string]interface{}{"object": "page_markdown", "id": id, "markdown": "# Hello", "truncated": false, "unknown_block_ids": []string{}}
		if len(f.warnings) > 0 {
			resp["warnings"] = f.warnings
		}
		reply(200, resp)
	case sub == "" && r.Method == http.MethodGet:
		reply(200, f.page(id))
	case sub == "" && r.Method == http.MethodPatch:
		if body["in_trash"] == true {
			f.trashed[id] = true
		}
		if title, ok := titleFromBody(body); ok {
			f.titles[id] = title
		}
		reply(200, f.page(id))
	default:
		reply(404, map[string]string{"object": "error", "code": "object_not_found", "message": r.Method + " " + r.URL.Path})
	}
}

// markdownCalls returns the calls that carry markdown: page creates and
// the /markdown endpoint.
func (f *fakeMarkdownPages) markdownCalls() []mdCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []mdCall
	for _, c := range f.calls {
		if (c.Method == http.MethodPost && c.Path == "/v1/pages") || strings.HasSuffix(c.Path, "/markdown") {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeMarkdownPages) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

func mdVersionConfig(providerVersion, resourceVersion, title, markdown string) string {
	prov := ""
	if providerVersion != "" {
		prov = fmt.Sprintf("  markdown_version = %q\n", providerVersion)
	}
	res := ""
	if resourceVersion != "" {
		res = fmt.Sprintf("  markdown_version = %q\n", resourceVersion)
	}
	return fmt.Sprintf(`
provider "notion" {
  token = "secret_unit_md_version"
%s}

resource "notion_page" "p" {
  parent_page_id = "11111111111111111111111111111111"
  title          = %q
  markdown       = %q
%s
  markdown_insert = {
    content  = "Appended."
    position = "end"
  }
}

data "notion_page_markdown" "d" {
  page_id = notion_page.p.id
}
`, prov, title, markdown, res)
}

// Every markdown call carries the resolved markdown_version: the resource
// value over the provider value, and no field at all when neither is set.
func TestUnitMarkdownVersion_SentOnEveryCall(t *testing.T) {
	cases := []struct {
		name, provider, resource, want string
	}{
		{"unset", "", "", ""},
		{"provider", "v2", "", "v2"},
		{"resource", "", "v2", "v2"},
		{"resource over provider", "v2", "v1", "v1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeMarkdownPages(t)
			check := resource.TestCheckNoResourceAttr("notion_page.p", "markdown_version")
			if c.want != "" {
				check = resource.TestCheckResourceAttr("notion_page.p", "markdown_version", c.want)
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: mdVersionConfig(c.provider, c.resource, "Doc", "# Hello"),
						Check:  check,
					},
					{
						Config: mdVersionConfig(c.provider, c.resource, "Doc", "# Hello again"),
					},
				},
			})
			seen := map[string]bool{}
			for _, call := range f.markdownCalls() {
				key := call.Method + " " + call.Type
				if call.Method == http.MethodGet {
					key = "GET"
				}
				seen[key] = true
				// The data source has no override here, so it follows the
				// provider value.
				want := c.want
				if key == "GET" {
					want = c.provider
				}
				if call.HasKey != (want != "") || call.Version != want {
					t.Errorf("%s %s (%s) sent markdown_version %q (present %v), want %q", call.Method, call.Path, call.Type, call.Version, call.HasKey, want)
				}
			}
			for _, k := range []string{"POST ", "PATCH insert_content", "PATCH replace_content", "GET"} {
				if !seen[k] {
					t.Errorf("no %q markdown call was made, so its version went unchecked (saw %v)", k, seen)
				}
			}
		})
	}
}

// A provider default must not show a diff on pages that don't manage
// markdown, and must not be stored for them. UnitTest fails on a
// non-empty plan after apply, so a perpetual diff would fail here.
func TestUnitMarkdownVersion_ProviderDefaultSkipsPagesWithoutMarkdown(t *testing.T) {
	newFakeMarkdownPages(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "notion" {
  token            = "secret_unit_md_version"
  markdown_version = "v2"
}

resource "notion_page" "p" {
  parent_page_id = "11111111111111111111111111111111"
  title          = "No body"
  position       = "page_end"
}
`,
				Check: resource.TestCheckNoResourceAttr("notion_page.p", "markdown_version"),
			},
		},
	})
}

func TestUnitMarkdownVersion_DatabaseEntryCreate(t *testing.T) {
	f := newFakeMarkdownPages(t)
	mc := &markdownClient{token: "secret_unit_md_entry"}
	for _, version := range []string{"", "v2"} {
		f.reset()
		if _, err := mc.CreateDatabaseEntryWithMarkdown(t.Context(), "db", "# Row", version, map[string]interface{}{}); err != nil {
			t.Fatal(err)
		}
		calls := f.markdownCalls()
		if len(calls) != 1 || calls[0].HasKey != (version != "") || calls[0].Version != version {
			t.Fatalf("version %q: got calls %+v", version, calls)
		}
	}
}
