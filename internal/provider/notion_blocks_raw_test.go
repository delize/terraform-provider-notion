package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHeading4Body(t *testing.T) {
	got := mustJSON(t, map[string]interface{}{
		"heading_4": heading4Body(plainToRichText("Lacinato kale"), "", true),
	})
	for _, want := range []string{`"heading_4":{`, `"content":"Lacinato kale"`, `"is_toggleable":true`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, `"color"`) {
		t.Errorf("empty color should be omitted: %s", got)
	}
	if got := mustJSON(t, heading4Body(nil, "blue", false)); !strings.Contains(got, `"color":"blue"`) || !strings.Contains(got, `"rich_text":[]`) {
		t.Errorf("unexpected body: %s", got)
	}
}

func TestTabBlockShape(t *testing.T) {
	got := mustJSON(t, tabBlock([]tabItemSpec{{Label: "Overview", Icon: "📋"}, {Label: "Details"}}))
	var parsed struct {
		Type string `json:"type"`
		Tab  struct {
			Children []struct {
				Type      string `json:"type"`
				Paragraph struct {
					RichText []struct {
						Text struct {
							Content string `json:"content"`
						} `json:"text"`
					} `json:"rich_text"`
					Icon *struct {
						Type  string `json:"type"`
						Emoji string `json:"emoji"`
					} `json:"icon"`
				} `json:"paragraph"`
			} `json:"children"`
		} `json:"tab"`
	}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Type != "tab" || len(parsed.Tab.Children) != 2 {
		t.Fatalf("unexpected tab block: %s", got)
	}
	first := parsed.Tab.Children[0]
	if first.Type != "paragraph" || first.Paragraph.RichText[0].Text.Content != "Overview" || first.Paragraph.Icon == nil || first.Paragraph.Icon.Emoji != "📋" {
		t.Errorf("first item wrong: %s", got)
	}
	if parsed.Tab.Children[1].Paragraph.Icon != nil {
		t.Errorf("second item should have no icon: %s", got)
	}
}

func TestTabItemBodyClearIcon(t *testing.T) {
	if got := mustJSON(t, tabItemBody(tabItemSpec{Label: "A"}, true)); !strings.Contains(got, `"icon":null`) {
		t.Errorf("clearIcon should send null: %s", got)
	}
	if got := mustJSON(t, tabItemBody(tabItemSpec{Label: "A"}, false)); strings.Contains(got, `"icon"`) {
		t.Errorf("no icon key expected: %s", got)
	}
}

func TestAppendBlockRequestPosition(t *testing.T) {
	got := mustJSON(t, appendBlockRequest(nil, "abc"))
	if !strings.Contains(got, `"position":{"after_block":{"id":"abc"},"type":"after_block"}`) {
		t.Errorf("position missing: %s", got)
	}
	if strings.Contains(got, `"after":`) {
		t.Errorf("deprecated after must not be sent: %s", got)
	}
	if got := mustJSON(t, appendBlockRequest(nil, "")); strings.Contains(got, "position") {
		t.Errorf("no position expected: %s", got)
	}
}

func item(id, label, icon string) tabItemModel {
	return tabItemModel{ID: types.StringValue(id), Label: types.StringValue(label), Icon: types.StringValue(icon)}
}

func TestPlanTabItemOps(t *testing.T) {
	prior := []tabItemModel{item("a", "One", ""), item("b", "Two", "🔍"), item("c", "Three", "")}

	ops := planTabItemOps(prior, []tabItemModel{item("a", "One", ""), item("b", "Two!", "🔍"), item("c", "Three", ""), item("", "Four", "")})
	if len(ops.Update) != 1 || ops.Update[0] != 1 || len(ops.Append) != 1 || ops.Append[0] != 3 || len(ops.Delete) != 0 {
		t.Errorf("rename+append: %+v", ops)
	}

	ops = planTabItemOps(prior, []tabItemModel{item("a", "One", "")})
	if len(ops.Update) != 0 || len(ops.Append) != 0 || len(ops.Delete) != 2 || ops.Delete[0] != 1 || ops.Delete[1] != 2 {
		t.Errorf("shrink: %+v", ops)
	}

	ops = planTabItemOps(prior, []tabItemModel{item("a", "One", ""), item("b", "Two", ""), item("c", "Three", "")})
	if len(ops.Update) != 1 || ops.Update[0] != 1 {
		t.Errorf("icon removal: %+v", ops)
	}
}

const tabChildrenFixture = `{
  "results": [
    {"object":"block","id":"11111111-1111-1111-1111-111111111111","type":"paragraph","has_children":true,"in_trash":false,
     "paragraph":{"rich_text":[{"type":"text","text":{"content":"Overview"},"plain_text":"Overview"}],"color":"default","icon":{"type":"emoji","emoji":"📋"}}},
    {"object":"block","id":"22222222-2222-2222-2222-222222222222","type":"paragraph","has_children":false,"in_trash":false,
     "paragraph":{"rich_text":[{"type":"text","text":{"content":"Details"},"plain_text":"Details"}],"color":"default","icon":{"type":"icon","icon":{"name":"pizza","color":"gray"}}}}
  ],
  "has_more": false,
  "next_cursor": null
}`

func TestTabItemsFromChildren(t *testing.T) {
	var page struct {
		Results []rawBlock `json:"results"`
	}
	if err := json.Unmarshal([]byte(tabChildrenFixture), &page); err != nil {
		t.Fatal(err)
	}
	items := tabItemsFromChildren(page.Results)
	if len(items) != 2 {
		t.Fatalf("items: %d", len(items))
	}
	if items[0].ID.ValueString() != "11111111111111111111111111111111" || items[0].Label.ValueString() != "Overview" || items[0].Icon.ValueString() != "📋" {
		t.Errorf("item 0: %+v", items[0])
	}
	// A native icon isn't representable as an emoji string, so it reads as "".
	if items[1].Icon.ValueString() != "" {
		t.Errorf("item 1 icon: %q", items[1].Icon.ValueString())
	}
}

func TestReadRawBlockIntoState_Heading4(t *testing.T) {
	b, err := parseRawBlock([]byte(`{"object":"block","id":"aaaa-bbbb","type":"heading_4","has_children":false,
		"parent":{"type":"page_id","page_id":"pppp-qqqq"},
		"heading_4":{"rich_text":[{"type":"text","text":{"content":"Kale"},"plain_text":"Kale"}],"color":"default","is_toggleable":true}}`))
	if err != nil {
		t.Fatal(err)
	}

	// Config left color empty: "default" from Notion stays "".
	state := BlockResourceModel{Color: types.StringValue(""), RichTextJSON: types.StringNull(), TabItems: types.ListNull(types.ObjectType{AttrTypes: tabItemAttrTypes})}
	readRawBlockIntoState(b, &state)
	if state.ID.ValueString() != "aaaabbbb" || state.ParentID.ValueString() != "ppppqqqq" {
		t.Errorf("ids: %s %s", state.ID, state.ParentID)
	}
	if state.RichText.ValueString() != "Kale" || state.Color.ValueString() != "" || !state.IsToggleable.ValueBool() {
		t.Errorf("fields: %+v", state)
	}

	// Config set "default" explicitly: keep it.
	state = BlockResourceModel{Color: types.StringValue("default"), RichTextJSON: types.StringNull()}
	readRawBlockIntoState(b, &state)
	if state.Color.ValueString() != "default" {
		t.Errorf("explicit default lost: %q", state.Color.ValueString())
	}

	// Import: every attribute starts null and ends at its schema default.
	state = BlockResourceModel{}
	state.RichTextJSON = types.StringNull()
	readRawBlockIntoState(b, &state)
	if state.Icon.IsNull() || state.Checked.IsNull() || state.Language.IsNull() {
		t.Errorf("defaults not filled on import: %+v", state)
	}
}

// fakeNotion records requests and answers from a handler. It's wired in
// through the per-token HTTP client registry, so the real request helpers
// run unchanged against it.
type fakeNotion struct {
	mu   sync.Mutex
	reqs []string // "METHOD path body"
}

func newFakeNotion(t *testing.T, token string, handler func(method, path, body string) (int, string)) *fakeNotion {
	t.Helper()
	f := &fakeNotion{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, r.Method+" "+r.URL.Path+" "+string(b))
		f.mu.Unlock()
		status, resp := handler(r.Method, r.URL.Path, string(b))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	registerHTTPClient(token, &http.Client{Transport: rewriteHost{target: target}})
	t.Cleanup(func() { providerHTTPClients.Delete(token) })
	return f
}

type rewriteHost struct{ target *url.URL }

func (rh rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	out := req.Clone(req.Context())
	out.URL.Scheme = rh.target.Scheme
	out.URL.Host = rh.target.Host
	out.Host = rh.target.Host
	return http.DefaultTransport.RoundTrip(out)
}

func TestReconcileTabItems(t *testing.T) {
	const token = "test-token-reconcile"
	f := newFakeNotion(t, token, func(method, path, body string) (int, string) {
		switch {
		case method == http.MethodPatch && path == "/v1/blocks/tab1/children":
			return 200, `{"results":[{"object":"block","id":"new-4","type":"paragraph"}]}`
		case method == http.MethodPatch:
			return 200, `{"object":"block","id":"x","type":"paragraph"}`
		case method == http.MethodDelete:
			return 200, `{"object":"block","id":"x","type":"paragraph","in_trash":true}`
		}
		return 404, `{"object":"error"}`
	})

	prior := []tabItemModel{item("a", "One", ""), item("b", "Two", "🔍"), item("c", "Three", "")}
	plan := []tabItemModel{item("a", "One", ""), item("b", "Two", ""), item("", "Three!", ""), item("", "Four", "")}
	ids, err := reconcileTabItems(context.Background(), token, "tab1", prior, plan)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ids, ",") != "a,b,c,new4" {
		t.Errorf("ids: %v", ids)
	}

	got := strings.Join(f.reqs, "\n")
	for _, want := range []string{
		`PATCH /v1/blocks/b {"paragraph":{"icon":null,`,  // icon cleared
		`PATCH /v1/blocks/c {"paragraph":{"rich_text":[`, // relabeled
		`"content":"Three!"`,
		`PATCH /v1/blocks/tab1/children {"children":[{"object":"block","paragraph":`,
		`"content":"Four"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing request %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "DELETE") {
		t.Errorf("nothing should be deleted:\n%s", got)
	}

	f.reqs = nil
	if _, err := reconcileTabItems(context.Background(), token, "tab1", prior, prior[:1]); err != nil {
		t.Fatal(err)
	}
	got = strings.Join(f.reqs, "\n")
	if !strings.Contains(got, "DELETE /v1/blocks/b") || !strings.Contains(got, "DELETE /v1/blocks/c") {
		t.Errorf("expected deletes of b and c:\n%s", got)
	}
}

func TestListBlockChildrenRawFollowsCursor(t *testing.T) {
	const token = "test-token-list"
	calls := 0
	newFakeNotion(t, token, func(method, path, body string) (int, string) {
		calls++
		if calls == 1 {
			return 200, `{"results":[{"id":"p1","type":"paragraph"}],"has_more":true,"next_cursor":"opaque-cursor:1"}`
		}
		return 200, `{"results":[{"id":"p2","type":"paragraph"}],"has_more":false,"next_cursor":null}`
	})
	got, err := listBlockChildrenRaw(context.Background(), token, "tab1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].ID != "p2" {
		t.Errorf("children: %+v", got)
	}
}
