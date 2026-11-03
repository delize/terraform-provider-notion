package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/jomei/notionapi"
)

// routeNotionTo sends every request the provider makes, through the SDK or
// over direct HTTP, to h. The provider's HTTP client wraps
// http.DefaultTransport when it's configured, so swapping that reaches the
// SDK too, which has no base URL hook.
func routeNotionTo(t *testing.T, h http.Handler) {
	t.Helper()
	srv := httptest.NewServer(h)
	target, _ := url.Parse(srv.URL)
	orig := http.DefaultTransport
	http.DefaultTransport = hostRoute{target: target, next: orig}
	prevBase := notionAPIBaseURL
	notionAPIBaseURL = "https://api.notion.com/v1"
	t.Cleanup(func() {
		http.DefaultTransport = orig
		notionAPIBaseURL = prevBase
		srv.Close()
	})
}

type hostRoute struct {
	target *url.URL
	next   http.RoundTripper
}

func (h hostRoute) RoundTrip(req *http.Request) (*http.Response, error) {
	out := req.Clone(req.Context())
	out.URL.Scheme = h.target.Scheme
	out.URL.Host = h.target.Host
	out.Host = h.target.Host
	return h.next.RoundTrip(out)
}

// fakeRichBlocks is a /v1/blocks fake that answers the way Notion does.
// Every rich text item comes back with plain_text, href, a null link when
// none was sent, and the full annotations object with defaults filled in.
// Block bodies get color "default" and, for headings, is_toggleable.
type fakeRichBlocks struct {
	mu     sync.Mutex
	next   int
	blocks map[string]map[string]interface{}
}

func newFakeRichBlocks(t *testing.T) *fakeRichBlocks {
	t.Helper()
	f := &fakeRichBlocks{blocks: map[string]map[string]interface{}{}}
	routeNotionTo(t, f)
	return f
}

func notionStyleRichText(items []interface{}) {
	for _, it := range items {
		m := it.(map[string]interface{})
		if m["type"] == nil {
			m["type"] = "text"
		}
		text := m["text"].(map[string]interface{})
		if _, ok := text["link"]; !ok {
			text["link"] = nil
		}
		ann := map[string]interface{}{"bold": false, "italic": false, "strikethrough": false,
			"underline": false, "code": false, "color": "default"}
		if a, ok := m["annotations"].(map[string]interface{}); ok {
			for k, v := range a {
				ann[k] = v
			}
		}
		m["annotations"] = ann
		m["plain_text"] = text["content"]
		m["href"] = nil
		if link, ok := text["link"].(map[string]interface{}); ok {
			m["href"] = link["url"]
		}
	}
}

func notionStyleBody(typ string, body map[string]interface{}) map[string]interface{} {
	delete(body, "children")
	if rt, ok := body["rich_text"].([]interface{}); ok {
		notionStyleRichText(rt)
	}
	if c, _ := body["color"].(string); c == "" {
		body["color"] = "default"
	}
	if strings.HasPrefix(typ, "heading_") {
		if _, ok := body["is_toggleable"]; !ok {
			body["is_toggleable"] = false
		}
	}
	return body
}

// mutate changes the body of every stored block of one type, as an edit in
// the Notion app would.
func (f *fakeRichBlocks) mutate(typ string, fn func(body map[string]interface{})) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, b := range f.blocks {
		if b["type"] == typ {
			fn(b[typ].(map[string]interface{}))
		}
	}
}

func (f *fakeRichBlocks) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	rawID, sub, _ := strings.Cut(rest, "/")
	id := normalizeID(rawID)

	switch {
	case r.Method == http.MethodPatch && sub == "children":
		var out []interface{}
		for _, c := range body["children"].([]interface{}) {
			c := c.(map[string]interface{})
			typ := c["type"].(string)
			f.next++
			newID := fmt.Sprintf("%032d", f.next+500)
			block := map[string]interface{}{
				"object": "block", "id": newID[:8] + "-" + newID[8:12] + "-" + newID[12:16] + "-" + newID[16:20] + "-" + newID[20:],
				"type": typ, "has_children": false, "archived": false, "in_trash": false,
				"created_time": "2026-11-03T10:00:00.000Z", "last_edited_time": "2026-11-03T10:00:00.000Z",
				"parent": map[string]string{"type": "page_id", "page_id": rawID},
				typ:      notionStyleBody(typ, c[typ].(map[string]interface{})),
			}
			f.blocks[newID] = block
			out = append(out, block)
		}
		reply(200, map[string]interface{}{"object": "list", "results": out})
	case r.Method == http.MethodPatch && f.blocks[id] != nil:
		b := f.blocks[id]
		typ := b["type"].(string)
		upd, ok := body[typ].(map[string]interface{})
		if !ok {
			reply(400, map[string]string{"object": "error", "code": "validation_error", "message": "missing " + typ})
			return
		}
		cur := b[typ].(map[string]interface{})
		for k, v := range notionStyleBody(typ, upd) {
			cur[k] = v
		}
		reply(200, b)
	case r.Method == http.MethodGet && f.blocks[id] != nil:
		reply(200, f.blocks[id])
	case r.Method == http.MethodDelete && f.blocks[id] != nil:
		f.blocks[id]["archived"] = true
		f.blocks[id]["in_trash"] = true
		reply(200, f.blocks[id])
	default:
		reply(404, map[string]string{"object": "error", "code": "object_not_found", "message": r.Method + " " + r.URL.Path})
	}
}

// The heredoc keeps odd key order and spacing on purpose. The stored value
// must be this exact string, not a re-encoding.
const richTextHeredoc = `<<-EOT
    [ {"text": {"link": {"url": "https://developers.notion.com/"}, "content": "API docs"},
       "annotations": {"bold": true, "italic": false}, "type": "text"},
      {"text": {"content": " for details"}} ]
  EOT`

func richTextJSONConfig(word string) string {
	return fmt.Sprintf(`
provider "notion" {
  token = "secret_unit_rich_text_json"
}

locals {
  rich = jsonencode([
    { type = "text", text = { content = "Read the " } },
    { type = "text", text = { content = "%[1]s", link = { url = "https://developers.notion.com/reference/rich-text" } }, annotations = { bold = true } },
    { text = { content = " today" }, annotations = { italic = true, color = "red" } },
  ])
}

resource "notion_block" "p" {
  parent_id      = "11111111111111111111111111111111"
  type           = "paragraph"
  rich_text_json = %[2]s
}

resource "notion_block" "h1" {
  parent_id      = "11111111111111111111111111111111"
  type           = "heading_1"
  rich_text_json = local.rich
}

resource "notion_block" "h4" {
  parent_id      = "11111111111111111111111111111111"
  type           = "heading_4"
  rich_text_json = local.rich
}

resource "notion_block" "c" {
  parent_id      = "11111111111111111111111111111111"
  type           = "callout"
  icon           = "💡"
  rich_text_json = local.rich
}
`, word, richTextHeredoc)
}

// Blocks created with rich_text_json, on the SDK path (paragraph,
// heading_1) and over direct HTTP (heading_4, callout), plan empty after
// create and after update even though Notion adds plain_text, href, and
// default annotations. Bolding text in the Notion app shows as drift and
// the next apply puts it back.
func TestUnitBlockRichTextJSON_RoundTrip(t *testing.T) {
	f := newFakeRichBlocks(t)
	italicFirst := func(body map[string]interface{}) {
		first := body["rich_text"].([]interface{})[0].(map[string]interface{})
		first["annotations"].(map[string]interface{})["italic"] = true
	}
	heredocValue := `[ {"text": {"link": {"url": "https://developers.notion.com/"}, "content": "API docs"},
   "annotations": {"bold": true, "italic": false}, "type": "text"},
  {"text": {"content": " for details"}} ]
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: richTextJSONConfig("rich text docs"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_block.p", "rich_text_json", heredocValue),
					resource.TestCheckResourceAttr("notion_block.p", "rich_text", ""),
					resource.TestCheckResourceAttr("notion_block.h1", "rich_text", ""),
					resource.TestCheckResourceAttr("notion_block.h4", "rich_text", ""),
					resource.TestCheckResourceAttr("notion_block.c", "rich_text", ""),
					resource.TestCheckResourceAttrPair("notion_block.h1", "rich_text_json", "notion_block.h4", "rich_text_json"),
				),
			},
			{
				Config: richTextJSONConfig("rich text reference"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("notion_block.h1", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("notion_block.h4", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("notion_block.c", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("notion_block.p", plancheck.ResourceActionNoop),
					},
				},
			},
			{
				// Someone italicizes the first word in the Notion app.
				PreConfig: func() {
					f.mutate("heading_1", italicFirst)
					f.mutate("heading_4", italicFirst)
				},
				Config: richTextJSONConfig("rich text reference"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("notion_block.h1", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("notion_block.h4", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("notion_block.c", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("notion_block.p", plancheck.ResourceActionNoop),
					},
				},
			},
		},
	})
}

// Switching an existing block between rich_text and rich_text_json leaves
// a clean plan both ways.
func TestUnitBlockRichTextJSON_SwitchFromRichText(t *testing.T) {
	newFakeRichBlocks(t)
	plain := `
provider "notion" {
  token = "secret_unit_rich_text_switch"
}

resource "notion_block" "p" {
  parent_id = "11111111111111111111111111111111"
  type      = "paragraph"
  rich_text = "See [the docs](https://developers.notion.com/)"
}
`
	withJSON := strings.Replace(plain, `rich_text = "See [the docs](https://developers.notion.com/)"`,
		`rich_text_json = jsonencode([{ type = "text", text = { content = "See " } }, { type = "text", text = { content = "the docs", link = { url = "https://developers.notion.com/" } }, annotations = { code = true } }])`, 1)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: plain, Check: resource.TestCheckResourceAttr("notion_block.p", "rich_text", "See [the docs](https://developers.notion.com/)")},
			{Config: withJSON, Check: resource.TestCheckResourceAttr("notion_block.p", "rich_text", "")},
			{Config: plain, Check: resource.TestCheckNoResourceAttr("notion_block.p", "rich_text_json")},
		},
	})
}

// richTextFromNotion decodes a rich text array the way the provider sees a
// Notion response.
func richTextFromNotion(t *testing.T, s string) []notionapi.RichText {
	t.Helper()
	var rt []notionapi.RichText
	if err := json.Unmarshal([]byte(s), &rt); err != nil {
		t.Fatal(err)
	}
	return rt
}

// A Notion response for a bold link, an equation, and a user mention.
const notionRichTextResponse = `[
  {"type":"text","text":{"content":"API docs","link":{"url":"https://developers.notion.com/"}},
   "annotations":{"bold":true,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},
   "plain_text":"API docs","href":"https://developers.notion.com/"},
  {"type":"equation","equation":{"expression":"E = mc^2"},
   "annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},
   "plain_text":"E = mc^2","href":null},
  {"type":"mention","mention":{"type":"user","user":{"object":"user","id":"6794760a-1f15-45cd-9c65-0dfe42f5135a","name":"Ada","avatar_url":null,"type":"person","person":{"email":"ada@example.com"}}},
   "annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},
   "plain_text":"@Ada","href":null},
  {"type":"mention","mention":{"type":"date","date":{"start":"2026-11-03","end":null}},
   "annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"blue"},
   "plain_text":"November 3, 2026","href":null}
]`

func TestRichTextMatchesJSON(t *testing.T) {
	rt := richTextFromNotion(t, notionRichTextResponse)
	cases := []struct {
		name string
		cfg  string
		want bool
	}{
		{"request shape", `[
			{"type":"text","text":{"content":"API docs","link":{"url":"https://developers.notion.com/"}},"annotations":{"bold":true}},
			{"type":"equation","equation":{"expression":"E = mc^2"}},
			{"type":"mention","mention":{"type":"user","user":{"id":"6794760a1f1545cd9c650dfe42f5135a"}}},
			{"type":"mention","mention":{"type":"date","date":{"start":"2026-11-03"}},"annotations":{"color":"blue"}}]`, true},
		{"type left out, explicit defaults", `[
			{"text":{"link":{"url":"https://developers.notion.com/"},"content":"API docs"},"annotations":{"bold":true,"italic":false,"color":"default"}},
			{"equation":{"expression":"E = mc^2"},"annotations":{"color":""}},
			{"mention":{"user":{"id":"6794760a-1f15-45cd-9c65-0dfe42f5135a"}}},
			{"mention":{"date":{"start":"2026-11-03","end":null}},"annotations":{"color":"blue"}}]`, true},
		{"response copied back, plain_text and href ignored", notionRichTextResponse, true},
		{"bold missing is drift", `[
			{"type":"text","text":{"content":"API docs","link":{"url":"https://developers.notion.com/"}}},
			{"type":"equation","equation":{"expression":"E = mc^2"}},
			{"type":"mention","mention":{"user":{"id":"6794760a1f1545cd9c650dfe42f5135a"}}},
			{"type":"mention","mention":{"date":{"start":"2026-11-03"}},"annotations":{"color":"blue"}}]`, false},
		{"different link", `[
			{"type":"text","text":{"content":"API docs","link":{"url":"https://example.com/"}},"annotations":{"bold":true}},
			{"type":"equation","equation":{"expression":"E = mc^2"}},
			{"type":"mention","mention":{"user":{"id":"6794760a1f1545cd9c650dfe42f5135a"}}},
			{"type":"mention","mention":{"date":{"start":"2026-11-03"}},"annotations":{"color":"blue"}}]`, false},
		{"different user", `[
			{"type":"text","text":{"content":"API docs","link":{"url":"https://developers.notion.com/"}},"annotations":{"bold":true}},
			{"type":"equation","equation":{"expression":"E = mc^2"}},
			{"type":"mention","mention":{"user":{"id":"00000000000000000000000000000001"}}},
			{"type":"mention","mention":{"date":{"start":"2026-11-03"}},"annotations":{"color":"blue"}}]`, false},
		{"one item short", `[{"type":"text","text":{"content":"API docs","link":{"url":"https://developers.notion.com/"}},"annotations":{"bold":true}}]`, false},
		{"not JSON", `[{`, false},
	}
	for _, c := range cases {
		if got := richTextMatchesJSON(c.cfg, rt); got != c.want {
			t.Errorf("%s: match = %v, want %v", c.name, got, c.want)
		}
	}
}

// Import and drift store Notion's rich text in request shape, which then
// matches itself.
func TestRichTextToRequestJSON(t *testing.T) {
	rt := richTextFromNotion(t, notionRichTextResponse)
	got, err := richTextToRequestJSON(rt)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"annotations":{"bold":true},"text":{"content":"API docs","link":{"url":"https://developers.notion.com/"}},"type":"text"},` +
		`{"equation":{"expression":"E = mc^2"},"type":"equation"},` +
		`{"mention":{"type":"user","user":{"id":"6794760a-1f15-45cd-9c65-0dfe42f5135a"}},"type":"mention"},` +
		`{"annotations":{"color":"blue"},"mention":{"date":{"end":null,"start":"2026-11-03T00:00:00Z"},"type":"date"},"type":"mention"}]`
	if got != want {
		t.Fatalf("request JSON\n got %s\nwant %s", got, want)
	}
	if !richTextMatchesJSON(got, rt) {
		t.Fatal("request JSON doesn't match the rich text it came from")
	}
}

// setRichTextState on both read paths: the configured string stays when it
// matches, rich_text keeps its planned value, and an import fills rich_text
// only.
func TestSetRichTextState(t *testing.T) {
	rt := richTextFromNotion(t, notionRichTextResponse)[:1]
	cfg := `[{"text": {"content": "API docs", "link": {"url": "https://developers.notion.com/"}}, "annotations": {"bold": true}}]`

	state := BlockResourceModel{RichText: types.StringValue(""), RichTextJSON: types.StringValue(cfg)}
	setRichTextState(rt, &state)
	if state.RichTextJSON.ValueString() != cfg || state.RichText.ValueString() != "" {
		t.Fatalf("configured: rich_text_json=%q rich_text=%q", state.RichTextJSON.ValueString(), state.RichText.ValueString())
	}

	drifted := `[{"text": {"content": "API docs", "link": {"url": "https://developers.notion.com/"}}}]`
	state = BlockResourceModel{RichText: types.StringValue(""), RichTextJSON: types.StringValue(drifted)}
	setRichTextState(rt, &state)
	if want := `[{"annotations":{"bold":true},"text":{"content":"API docs","link":{"url":"https://developers.notion.com/"}},"type":"text"}]`; state.RichTextJSON.ValueString() != want {
		t.Fatalf("drift: rich_text_json=%q, want %q", state.RichTextJSON.ValueString(), want)
	}

	imported := BlockResourceModel{RichTextJSON: types.StringNull()}
	setRichTextState(rt, &imported)
	if !imported.RichTextJSON.IsNull() || imported.RichText.ValueString() != "[API docs](https://developers.notion.com/)" {
		t.Fatalf("import: rich_text_json=%v rich_text=%q", imported.RichTextJSON, imported.RichText.ValueString())
	}
}
