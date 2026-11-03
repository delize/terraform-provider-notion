package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
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

// fakeSchema is the data source the fake views belong to, keyed by property
// name. The data source endpoint returns IDs URL-escaped, and views return
// them unescaped, as Notion does.
var fakeSchema = map[string]string{"Name": "title", "Priority": "sFqA", "Done": "b%3Ax"}

func fakePropID(ref string) string {
	if id, ok := fakeSchema[ref]; ok {
		ref = id
	}
	if un, err := url.PathUnescape(ref); err == nil {
		return un
	}
	return ref
}

// toNotionRefs rewrites property names to IDs the way Notion stores a
// view: "property" and "property_id" values at any depth, and with keys set
// the top-level keys (quick filters).
func toNotionRefs(v interface{}, keys bool) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := map[string]interface{}{}
		for k, val := range t {
			if s, ok := val.(string); ok && (k == "property" || k == "property_id") {
				out[k] = fakePropID(s)
				continue
			}
			if keys {
				k = fakePropID(k)
			}
			out[k] = toNotionRefs(val, false)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, e := range t {
			out[i] = toNotionRefs(e, false)
		}
		return out
	}
	return v
}

// expandTableProperties lists every schema property in a table
// configuration, with property_name, the way Notion answers.
func expandTableProperties(cfg interface{}) interface{} {
	m, ok := cfg.(map[string]interface{})
	if !ok || m["properties"] == nil {
		return cfg
	}
	seen := map[string]bool{}
	var out []interface{}
	names := map[string]string{}
	for name := range fakeSchema {
		names[fakePropID(name)] = name
	}
	for _, e := range m["properties"].([]interface{}) {
		pm := e.(map[string]interface{})
		id := pm["property_id"].(string)
		seen[id] = true
		pm["property_name"] = names[id]
		out = append(out, pm)
	}
	ids := make([]string, 0, len(names))
	for id := range names {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if !seen[id] {
			out = append(out, map[string]interface{}{"property_id": id, "property_name": names[id], "visible": false})
		}
	}
	m["properties"] = out
	return m
}

// fakeViews serves /v1/views the way the views guide describes, plus what
// the real API does that the guide doesn't say. filter and sorts are
// replaced on update (null clears them). quick_filters and configuration
// are merged per top-level key, and a key set to null is removed. Notion
// returns null for anything not set. Property names come back as IDs, and a
// table configuration lists every property. Responses use their own key
// order and whitespace. GET /v1/data_sources/{id} returns fakeSchema.
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
	for _, k := range []string{"filter", "sorts", "configuration"} {
		if body[k] != nil {
			body[k] = toNotionRefs(body[k], false)
		}
	}
	if body["quick_filters"] != nil {
		body["quick_filters"] = toNotionRefs(body["quick_filters"], true)
	}

	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/data_sources/"):
		props := map[string]interface{}{}
		for name, pid := range fakeSchema {
			props[name] = map[string]interface{}{"id": pid, "name": name}
		}
		reply(200, map[string]interface{}{"object": "data_source", "id": strings.TrimPrefix(r.URL.Path, "/v1/data_sources/"), "properties": props})
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
		v["configuration"] = expandTableProperties(v["configuration"])
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
		v["configuration"] = expandTableProperties(v["configuration"])
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
    [ { "direction": "descending", "property": "Priority" },
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

const viewSortsHeredoc = `[ { "direction": "descending", "property": "Priority" },
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
	twoQuick := `{ title = {}, "b%3Ax" = { checkbox = { equals = true } } }`
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
				// Import has no config to compare with, so the filter keeps
				// Notion's property IDs where the config named properties.
				ImportStateVerifyIgnore: []string{"configuration", "sorts", "filter"},
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
		name  string
		prior types.String
		raw   json.RawMessage
		kind  viewJSONKind
		want  string
	}{
		{"same value, other order", types.StringValue(`{"properties":[{"property_id":"title","visible":true,"width":320}],"type":"table","wrap_cells":true}`), notion, viewJSONFilter, "keep"},
		{"merged key ignored for configuration", types.StringValue(`{"type":"table","properties":[{"property_id":"title","visible":true,"width":320}]}`), notion, viewJSONConfiguration, "keep"},
		{"merged key is drift elsewhere", types.StringValue(`{"type":"table","properties":[{"property_id":"title","visible":true,"width":320}]}`), notion, viewJSONFilter, canonical},
		{"configured key changed", types.StringValue(`{"type":"table","wrap_cells":false,"properties":[{"property_id":"title","visible":true,"width":320}]}`), notion, viewJSONConfiguration, canonical},
		{"nested change", types.StringValue(`{"type":"table","properties":[{"property_id":"title","visible":true,"width":300}]}`), notion, viewJSONConfiguration, canonical},
		{"configured null, absent in Notion", types.StringValue(`{"type":"table","group_by":null,"wrap_cells":true,"properties":[{"property_id":"title","visible":true,"width":320}]}`), notion, viewJSONConfiguration, "keep"},
		{"import", types.StringNull(), notion, viewJSONConfiguration, canonical},
		{"unknown on create", types.StringUnknown(), notion, viewJSONConfiguration, canonical},
		{"unset", types.StringValue(""), json.RawMessage(`null`), viewJSONSorts, ""},
		{"empty sorts", types.StringValue("[]"), nil, viewJSONSorts, "keep"},
		{"cleared with jsonencode(null)", types.StringValue("null"), json.RawMessage(`null`), viewJSONSorts, "keep"},
		{"cleared in Notion", types.StringValue(`[{"timestamp":"created_time","direction":"ascending"}]`), json.RawMessage(`null`), viewJSONSorts, ""},
	}
	for _, c := range cases {
		got := viewJSONReadback(c.prior, c.raw, c.kind, nil)
		want := c.want
		if want == "keep" {
			want = c.prior.ValueString()
		}
		if got.ValueString() != want || got.IsNull() || got.IsUnknown() {
			t.Errorf("%s: got %v, want %q", c.name, got, want)
		}
	}
}

// Notion answers with property IDs where the config named properties, and
// lists every property in a table configuration with property_name. With
// the data source's names and IDs, those still match the config.
func TestViewJSONReadback_PropertyRefs(t *testing.T) {
	refs := newViewPropertyRefs(map[string]struct {
		ID string `json:"id"`
	}{"Name": {ID: "title"}, "Priority": {ID: "sFqA"}, "Done": {ID: "b%3Ax"}})
	cases := []struct {
		name  string
		prior string
		raw   string
		kind  viewJSONKind
		keep  bool
	}{
		{"filter by name", `{"property":"Priority","select":{"equals":"High"}}`, `{"property":"sFqA","select":{"equals":"High"}}`, viewJSONFilter, true},
		{"compound filter, escaped ID", `{"and":[{"property":"Done","checkbox":{"equals":false}}]}`, `{"and":[{"property":"b:x","checkbox":{"equals":false}}]}`, viewJSONFilter, true},
		{"filter value changed", `{"property":"Priority","select":{"equals":"High"}}`, `{"property":"sFqA","select":{"equals":"Low"}}`, viewJSONFilter, false},
		{"sorts by title name", `[{"property":"Name","direction":"ascending"}]`, `[{"direction":"ascending","property":"title"}]`, viewJSONSorts, true},
		{"sort dropped", `[{"property":"Name","direction":"ascending"},{"property":"Priority","direction":"descending"}]`, `[{"direction":"ascending","property":"title"}]`, viewJSONSorts, false},
		{"quick filter key by name", `{"Priority":{"select":{"equals":"High"}}}`, `{"sFqA":{"select":{"equals":"High"}}}`, viewJSONQuickFilters, true},
		{"table configuration lists every property",
			`{"type":"table","properties":[{"property_id":"title","visible":true,"width":320}],"wrap_cells":true}`,
			`{"properties":[{"property_id":"title","property_name":"Name","visible":true,"width":320},{"property_id":"sFqA","property_name":"Priority","visible":false}],"type":"table","wrap_cells":true}`,
			viewJSONConfiguration, true},
		{"configured property by name", `{"type":"table","properties":[{"property_id":"Priority","visible":true}]}`,
			`{"properties":[{"property_id":"title","visible":true},{"property_id":"sFqA","property_name":"Priority","visible":true}],"type":"table"}`,
			viewJSONConfiguration, true},
		{"configured property changed", `{"type":"table","properties":[{"property_id":"Priority","visible":true}]}`,
			`{"properties":[{"property_id":"sFqA","property_name":"Priority","visible":false}],"type":"table"}`,
			viewJSONConfiguration, false},
		{"configured property missing", `{"type":"table","properties":[{"property_id":"Gone","visible":true}]}`,
			`{"properties":[{"property_id":"title","visible":true}],"type":"table"}`,
			viewJSONConfiguration, false},
	}
	for _, c := range cases {
		got := viewJSONReadback(types.StringValue(c.prior), json.RawMessage(c.raw), c.kind, refs)
		if kept := got.ValueString() == c.prior; kept != c.keep {
			t.Errorf("%s: kept = %v, want %v (got %s)", c.name, kept, c.keep, got.ValueString())
		}
	}
	// Without the schema, a name and an ID don't match.
	got := viewJSONReadback(types.StringValue(`{"property":"Priority","select":{"equals":"High"}}`), json.RawMessage(`{"property":"sFqA","select":{"equals":"High"}}`), viewJSONFilter, nil)
	if got.ValueString() == `{"property":"Priority","select":{"equals":"High"}}` {
		t.Errorf("matched a name to an ID without refs")
	}
}

func TestTimestampSortError(t *testing.T) {
	if err := timestampSortError(`[{"property":"Name","direction":"ascending"}]`); err != nil {
		t.Errorf("property sort rejected: %v", err)
	}
	if err := timestampSortError(`[{"property":"Name","direction":"ascending"},{"timestamp":"created_time","direction":"descending"}]`); err == nil || !strings.Contains(err.Error(), "sorts[1]") {
		t.Errorf("timestamp sort error = %v", err)
	}
	if err := timestampSortError(""); err != nil {
		t.Errorf("empty sorts rejected: %v", err)
	}
}

func TestClearRemovedQuickFilters(t *testing.T) {
	got, err := clearRemovedQuickFilters(json.RawMessage(`{"title":{}}`), types.StringValue(`{"title":{},"abc1":{"select":{"equals":"High"}}}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"abc1":null,"title":{}}` {
		t.Fatalf("body = %s", got)
	}
	same := json.RawMessage(`{"title":{}}`)
	got, _ = clearRemovedQuickFilters(same, types.StringValue(`{"title":{"title":{"contains":"x"}}}`), nil)
	if string(got) != string(same) {
		t.Fatalf("nothing removed, body = %s", got)
	}
	// State holds Notion's ID key, the config names the property. Nulling
	// the ID could remove the filter being set, so nothing is nulled.
	byName := json.RawMessage(`{"Status":{"status":{"equals":"Done"}}}`)
	got, _ = clearRemovedQuickFilters(byName, types.StringValue(`{"%3AUPp":{"status":{"equals":"Done"}}}`), nil)
	if string(got) != string(byName) {
		t.Fatalf("name in plan, ID in state, body = %s", got)
	}
	got, _ = clearRemovedQuickFilters(nil, types.StringValue(`{"title":{}}`), nil)
	if got != nil {
		t.Fatalf("quick_filters not sent, body = %s", got)
	}
	// With the schema, the plan's name and the state's ID are the same
	// property, so it stays, and the dropped one is nulled by its ID.
	refs := newViewPropertyRefs(map[string]struct {
		ID string `json:"id"`
	}{"Status": {ID: "%3AUPp"}, "Priority": {ID: "sFqA"}})
	got, _ = clearRemovedQuickFilters(byName, types.StringValue(`{":UPp":{"status":{"equals":"Done"}},"sFqA":{}}`), refs)
	if string(got) != `{"Status":{"status":{"equals":"Done"}},"sFqA":null}` {
		t.Fatalf("with refs, body = %s", got)
	}
}

// A timestamp sort is refused at plan time, before anything is sent.
func TestUnitView_TimestampSortRejected(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
provider "notion" {
  token = "secret_unit_view_timestamp_sort"
}

resource "notion_view" "v" {
  database_id    = "22222222222222222222222222222222"
  data_source_id = "33333333333333333333333333333333"
  name           = "Newest first"
  type           = "table"
  sorts          = jsonencode([{ timestamp = "created_time", direction = "descending" }])
}
`,
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`Timestamp sort on a view`),
		}},
	})
}
