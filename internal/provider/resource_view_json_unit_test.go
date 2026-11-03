package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// notionOrderJSON encodes v with object keys in reverse order and with
// indentation, so the bytes never match what jsonencode produces even when
// the value is the same.
func notionOrderJSON(v interface{}, indent string) string {
	switch t := v.(type) {
	case map[string]interface{}:
		if len(t) == 0 {
			return "{}"
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(keys)))
		parts := make([]string, len(keys))
		for i, k := range keys {
			kb, _ := json.Marshal(k)
			parts[i] = indent + "  " + string(kb) + ": " + notionOrderJSON(t[k], indent+"  ")
		}
		return "{\n" + strings.Join(parts, ",\n") + "\n" + indent + "}"
	case []interface{}:
		if len(t) == 0 {
			return "[]"
		}
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = indent + "  " + notionOrderJSON(e, indent+"  ")
		}
		return "[\n" + strings.Join(parts, ",\n") + "\n" + indent + "]"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// fakeViews serves /v1/views the way the views guide describes. filter and
// sorts are replaced on update (null clears them). quick_filters and
// configuration are merged per top-level key, and a key set to null is
// removed. Notion returns null for anything not set. Responses use their
// own key order and whitespace.
type fakeViews struct {
	mu    sync.Mutex
	next  int
	views map[string]map[string]interface{}
}

func newFakeViews(t *testing.T) *fakeViews {
	t.Helper()
	f := &fakeViews{views: map[string]map[string]interface{}{}}
	routeNotionTo(t, f)
	return f
}

func mergeTopLevel(cur interface{}, upd map[string]interface{}) interface{} {
	m, _ := cur.(map[string]interface{})
	if m == nil {
		m = map[string]interface{}{}
	}
	for k, v := range upd {
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// edit changes a stored view, as an edit in the Notion app would.
func (f *fakeViews) edit(fn func(v map[string]interface{})) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range f.views {
		fn(v)
	}
}

func (f *fakeViews) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&body)
	w.Header().Set("Content-Type", "application/json")
	reply := func(status int, v interface{}) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(notionOrderJSON(v, "")))
	}
	id := normalizeID(strings.TrimPrefix(r.URL.Path, "/v1/views/"))

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/views":
		f.next++
		newID := fmt.Sprintf("%032d", f.next+700)
		v := map[string]interface{}{
			"object": "view", "id": newID[:8] + "-" + newID[8:12] + "-" + newID[12:16] + "-" + newID[16:20] + "-" + newID[20:],
			"parent":         map[string]interface{}{"type": "database_id", "database_id": body["database_id"]},
			"data_source_id": body["data_source_id"], "name": body["name"], "type": body["type"],
			"filter": body["filter"], "sorts": body["sorts"], "quick_filters": body["quick_filters"],
			"configuration": body["configuration"],
			"created_time":  "2026-11-03T10:00:00.000Z", "last_edited_time": "2026-11-03T10:00:00.000Z",
			"created_by": map[string]interface{}{"object": "user", "id": "e7f3a4b2-1234-5678-9abc-def012345678"},
			"url":        "https://www.notion.so/db?v=" + newID,
		}
		f.views[newID] = v
		reply(200, v)
	case r.Method == http.MethodPatch && f.views[id] != nil:
		v := f.views[id]
		for _, k := range []string{"name", "filter", "sorts"} {
			if val, ok := body[k]; ok {
				v[k] = val
			}
		}
		for _, k := range []string{"quick_filters", "configuration"} {
			switch upd := body[k].(type) {
			case map[string]interface{}:
				v[k] = mergeTopLevel(v[k], upd)
			case nil:
				if _, ok := body[k]; ok {
					v[k] = nil
				}
			}
		}
		reply(200, v)
	case r.Method == http.MethodGet && f.views[id] != nil:
		reply(200, f.views[id])
	case r.Method == http.MethodDelete && f.views[id] != nil:
		v := f.views[id]
		delete(f.views, id)
		reply(200, map[string]interface{}{"object": "view", "id": v["id"], "parent": v["parent"], "type": v["type"]})
	default:
		reply(404, map[string]interface{}{"object": "error", "code": "object_not_found", "message": r.Method + " " + r.URL.Path})
	}
}

type viewJSONStep struct {
	priority     string
	wrapCells    bool
	quickFilters string
}

func viewJSONConfig(s viewJSONStep) string {
	wrap := ""
	if s.wrapCells {
		wrap = "wrap_cells = true"
	}
	return fmt.Sprintf(`
provider "notion" {
  token = "secret_unit_view_json"
}

resource "notion_view" "v" {
  database_id    = "22222222222222222222222222222222"
  data_source_id = "33333333333333333333333333333333"
  name           = "Open high priority"
  type           = "table"

  filter = jsonencode({
    and = [
      { property = "Priority", select = { equals = %q } },
      { property = "Done", checkbox = { equals = false } },
    ]
  })

  # Written by hand with odd spacing. The stored value must be this string.
  sorts = <<-EOT
    [ { "direction": "descending", "timestamp": "created_time" },
      { "property": "Name",  "direction": "ascending" } ]
  EOT

  quick_filters = jsonencode(%s)

  configuration = jsonencode({
    type = "table"
    properties = [
      { property_id = "title", visible = true, width = 320 },
    ]
    %s
  })
}
`, s.priority, s.quickFilters, wrap)
}

const viewSortsHeredoc = `[ { "direction": "descending", "timestamp": "created_time" },
  { "property": "Name",  "direction": "ascending" } ]
`

// A view with a filter, sorts, quick filters, and a table configuration
// plans empty after create and after update, though Notion answers with its
// own key order and whitespace and keeps configuration fields an update
// leaves out. Removing a quick filter sends null for it, so it's really
// gone. An edit in the Notion app shows as drift and the next apply undoes
// it. Import stores Notion's values in jsonencode's form.
func TestUnitViewJSONAttributes_RoundTrip(t *testing.T) {
	f := newFakeViews(t)
	const addr = "notion_view.v"
	twoQuick := `{ title = {}, "%3AUPp" = { select = { equals = "High" } } }`
	oneQuick := `{ title = {} }`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: viewJSONConfig(viewJSONStep{priority: "High", wrapCells: true, quickFilters: twoQuick}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "sorts", viewSortsHeredoc),
					resource.TestCheckResourceAttr(addr, "filter",
						`{"and":[{"property":"Priority","select":{"equals":"High"}},{"checkbox":{"equals":false},"property":"Done"}]}`),
					resource.TestCheckResourceAttr(addr, "configuration",
						`{"properties":[{"property_id":"title","visible":true,"width":320}],"type":"table","wrap_cells":true}`),
				),
			},
			{
				// Change the filter, drop wrap_cells (Notion keeps it, per
				// the shallow merge), and remove one quick filter.
				Config: viewJSONConfig(viewJSONStep{priority: "Low", quickFilters: oneQuick}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "quick_filters", `{"title":{}}`),
					resource.TestCheckResourceAttr(addr, "configuration",
						`{"properties":[{"property_id":"title","visible":true,"width":320}],"type":"table"}`),
				),
			},
			{
				// Someone changes the filter and a column width in the app.
				PreConfig: func() {
					f.edit(func(v map[string]interface{}) {
						v["filter"] = map[string]interface{}{"property": "Priority", "select": map[string]interface{}{"equals": "High"}}
						cfg := v["configuration"].(map[string]interface{})
						cfg["properties"].([]interface{})[0].(map[string]interface{})["width"] = 500
					})
				},
				Config: viewJSONConfig(viewJSONStep{priority: "Low", quickFilters: oneQuick}),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(addr, "filter",
					`{"and":[{"property":"Priority","select":{"equals":"Low"}},{"checkbox":{"equals":false},"property":"Done"}]}`),
			},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateVerify: true,
				// Notion still has wrap_cells from step 1, which the config
				// no longer sets, and the sorts string was written by hand.
				ImportStateVerifyIgnore: []string{"configuration", "sorts"},
			},
		},
	})
}

// JSON attributes left out of the config follow Notion. A rename plans them
// unknown, and the update stores what Notion has, normalized.
func TestUnitViewJSONAttributes_UnsetFollowNotion(t *testing.T) {
	f := newFakeViews(t)
	cfg := func(name string) string {
		return fmt.Sprintf(`
provider "notion" {
  token = "secret_unit_view_json_computed"
}

resource "notion_view" "v" {
  database_id    = "22222222222222222222222222222222"
  data_source_id = "33333333333333333333333333333333"
  name           = %q
  type           = "table"
}
`, name)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("First"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_view.v", "filter", ""),
					resource.TestCheckResourceAttr("notion_view.v", "configuration", ""),
				),
			},
			{
				// Notion gives the view a configuration of its own.
				PreConfig: func() {
					f.edit(func(v map[string]interface{}) {
						v["configuration"] = map[string]interface{}{"type": "table", "wrap_cells": false}
					})
				},
				Config: cfg("Second"),
				Check:  resource.TestCheckResourceAttr("notion_view.v", "configuration", `{"type":"table","wrap_cells":false}`),
			},
		},
	})
}

func TestViewJSONReadback(t *testing.T) {
	notion := json.RawMessage(`{
  "type": "table",
  "wrap_cells": true,
  "properties": [ { "width": 320, "visible": true, "property_id": "title" } ]
}`)
	canonical := `{"properties":[{"property_id":"title","visible":true,"width":320}],"type":"table","wrap_cells":true}`
	cases := []struct {
		name   string
		prior  types.String
		raw    json.RawMessage
		merged bool
		want   string
	}{
		{"same value, other order", types.StringValue(`{"properties":[{"property_id":"title","visible":true,"width":320}],"type":"table","wrap_cells":true}`), notion, false, "keep"},
		{"merged key ignored for configuration", types.StringValue(`{"type":"table","properties":[{"property_id":"title","visible":true,"width":320}]}`), notion, true, "keep"},
		{"merged key is drift elsewhere", types.StringValue(`{"type":"table","properties":[{"property_id":"title","visible":true,"width":320}]}`), notion, false, canonical},
		{"configured key changed", types.StringValue(`{"type":"table","wrap_cells":false,"properties":[{"property_id":"title","visible":true,"width":320}]}`), notion, true, canonical},
		{"nested change", types.StringValue(`{"type":"table","properties":[{"property_id":"title","visible":true,"width":300}]}`), notion, true, canonical},
		{"configured null, absent in Notion", types.StringValue(`{"type":"table","group_by":null,"wrap_cells":true,"properties":[{"property_id":"title","visible":true,"width":320}]}`), notion, true, "keep"},
		{"import", types.StringNull(), notion, true, canonical},
		{"unknown on create", types.StringUnknown(), notion, true, canonical},
		{"unset", types.StringValue(""), json.RawMessage(`null`), false, ""},
		{"empty sorts", types.StringValue("[]"), nil, false, "keep"},
		{"cleared in Notion", types.StringValue(`[{"timestamp":"created_time","direction":"ascending"}]`), json.RawMessage(`null`), false, ""},
	}
	for _, c := range cases {
		got := viewJSONReadback(c.prior, c.raw, c.merged)
		want := c.want
		if want == "keep" {
			want = c.prior.ValueString()
		}
		if got.ValueString() != want || got.IsNull() || got.IsUnknown() {
			t.Errorf("%s: got %v, want %q", c.name, got, want)
		}
	}
}

func TestClearRemovedQuickFilters(t *testing.T) {
	got, err := clearRemovedQuickFilters(json.RawMessage(`{"title":{}}`), types.StringValue(`{"title":{},"abc1":{"select":{"equals":"High"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"abc1":null,"title":{}}` {
		t.Fatalf("body = %s", got)
	}
	same := json.RawMessage(`{"title":{}}`)
	got, _ = clearRemovedQuickFilters(same, types.StringValue(`{"title":{"title":{"contains":"x"}}}`))
	if string(got) != string(same) {
		t.Fatalf("nothing removed, body = %s", got)
	}
	// State holds Notion's ID key, the config names the property. Nulling
	// the ID could remove the filter being set, so nothing is nulled.
	byName := json.RawMessage(`{"Status":{"status":{"equals":"Done"}}}`)
	got, _ = clearRemovedQuickFilters(byName, types.StringValue(`{"%3AUPp":{"status":{"equals":"Done"}}}`))
	if string(got) != string(byName) {
		t.Fatalf("name in plan, ID in state, body = %s", got)
	}
	got, _ = clearRemovedQuickFilters(nil, types.StringValue(`{"title":{}}`))
	if got != nil {
		t.Fatalf("quick_filters not sent, body = %s", got)
	}
}
