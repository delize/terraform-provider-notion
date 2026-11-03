package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jomei/notionapi"
)

func listMap(t *testing.T, m map[string][]string) types.Map {
	t.Helper()
	vals := map[string]attr.Value{}
	for k, items := range m {
		elems := make([]attr.Value, 0, len(items))
		for _, s := range items {
			elems = append(elems, types.StringValue(s))
		}
		vals[k] = types.ListValueMust(types.StringType, elems)
	}
	return types.MapValueMust(entryListElemType, vals)
}

func mapOfLists(t *testing.T, m types.Map) map[string][]string {
	t.Helper()
	if m.IsNull() {
		return nil
	}
	var out map[string][]string
	if d := m.ElementsAs(context.Background(), &out, false); d.HasError() {
		t.Fatal(d)
	}
	return out
}

// propsJSON marshals built properties the way both create paths send them.
func propsJSON(t *testing.T, props notionapi.Properties) map[string]map[string]interface{} {
	t.Helper()
	b, err := json.Marshal(props)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBuildEntryListProperties(t *testing.T) {
	m := entryModel("", "Row", "", "")
	m.MultiSelectProperties = listMap(t, map[string][]string{"Tags": {"A", "B"}, "Empty": {}})
	m.RelationProperties = listMap(t, map[string][]string{"Links": {"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}})
	m.PeopleProperties = listMap(t, map[string][]string{"Owner": {"u1"}})
	var diags diag.Diagnostics
	props := buildEntryProperties(context.Background(), &m, &diags)
	if diags.HasError() {
		t.Fatal(diags)
	}
	got := propsJSON(t, props)

	tags, _ := json.Marshal(got["Tags"]["multi_select"])
	if string(tags) != `[{"name":"A"},{"name":"B"}]` {
		t.Errorf("multi_select = %s", tags)
	}
	empty, _ := json.Marshal(got["Empty"]["multi_select"])
	if string(empty) != `[]` {
		t.Errorf("empty multi_select must be sent as [], got %s", empty)
	}
	links, _ := json.Marshal(got["Links"]["relation"])
	if string(links) != `[{"id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}]` {
		t.Errorf("relation = %s", links)
	}
	owner, _ := json.Marshal(got["Owner"]["people"])
	if string(owner) != `[{"id":"u1"}]` {
		t.Errorf("people = %s", owner)
	}
	for name, typ := range map[string]string{"Tags": "multi_select", "Links": "relation", "Owner": "people"} {
		if got[name]["type"] != typ {
			t.Errorf("%s type = %v, want %s", name, got[name]["type"], typ)
		}
	}
}

func TestClearRemovedListProperties(t *testing.T) {
	state := entryModel("", "Row", "", "")
	state.MultiSelectProperties = listMap(t, map[string][]string{"Tags": {"A"}, "Kept": {"B"}})
	state.RelationProperties = listMap(t, map[string][]string{"Links": {"p1"}})
	state.PeopleProperties = listMap(t, map[string][]string{"Owner": {"u1"}})
	plan := entryModel("", "Row", "", "")
	plan.MultiSelectProperties = listMap(t, map[string][]string{"Kept": {"B"}})

	props := notionapi.Properties{}
	clearRemovedProperties(&state, &plan, props)
	got := propsJSON(t, props)
	for name, field := range map[string]string{"Tags": "multi_select", "Links": "relation", "Owner": "people"} {
		v, _ := json.Marshal(got[name][field])
		if string(v) != `[]` {
			t.Errorf("%s.%s = %s, want []", name, field, v)
		}
	}
	if _, ok := got["Kept"]; ok {
		t.Errorf("Kept is still configured and must not be cleared")
	}
}

func TestReconcileListValues(t *testing.T) {
	cases := []struct {
		name       string
		prior, api []string
		norm       func(string) string
		want       []string
	}{
		{"same order", []string{"A", "B"}, []string{"A", "B"}, identity, []string{"A", "B"}},
		{"reordered keeps prior", []string{"B", "A"}, []string{"A", "B"}, identity, []string{"B", "A"}},
		{"changed takes api", []string{"A"}, []string{"A", "C"}, identity, []string{"A", "C"}},
		{"both empty", []string{}, []string{}, identity, []string{}},
		{"hyphenated config kept", []string{"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}, []string{"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}, normalizeID, []string{"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}},
		{"bare config kept", []string{"aaaaaaaabbbbccccddddeeeeeeeeeeee"}, []string{"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}, normalizeID, []string{"aaaaaaaabbbbccccddddeeeeeeeeeeee"}},
		{"new id normalized", []string{"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}, []string{"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "11111111-2222-3333-4444-555555555555"}, normalizeID, []string{"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "11111111222233334444555555555555"}},
		{"removed in notion", []string{"a", "b"}, []string{"b"}, identity, []string{"b"}},
	}
	for _, c := range cases {
		got := reconcileListValues(c.prior, c.api, c.norm)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

const listEntryPageJSON = `{
  "object": "page",
  "id": "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee",
  "url": "https://www.notion.so/e",
  "parent": {"type": "database_id", "database_id": "dddddddd-dddd-dddd-dddd-dddddddddddd"},
  "properties": {
    "Name": {"id": "title", "type": "title", "title": [{"type": "text", "plain_text": "Row", "text": {"content": "Row"}}]},
    "Tags": {"id": "t%3Ag", "type": "multi_select", "multi_select": [{"id": "1", "name": "B", "color": "red"}, {"id": "2", "name": "A", "color": "blue"}]},
    "Other Tags": {"id": "o", "type": "multi_select", "multi_select": [{"id": "3", "name": "Z", "color": "red"}]},
    "Links": {"id": "l", "type": "relation", "relation": [{"id": "11111111-2222-3333-4444-555555555555"}, {"id": "66666666-7777-8888-9999-000000000000"}], "has_more": false},
    "Owner": {"id": "p", "type": "people", "people": [{"object": "user", "id": "aaaaaaaa-0000-0000-0000-000000000001"}]}
  }
}`

func TestReadEntryListProperties(t *testing.T) {
	var page notionapi.Page
	if err := json.Unmarshal([]byte(listEntryPageJSON), &page); err != nil {
		t.Fatal(err)
	}
	state := entryModel("e", "Row", "", "")
	state.MultiSelectProperties = listMap(t, map[string][]string{"Tags": {"A", "B"}, "Gone": {"X"}})
	state.RelationProperties = listMap(t, map[string][]string{"Links": {"11111111-2222-3333-4444-555555555555"}})
	state.PeopleProperties = listMap(t, map[string][]string{"Owner": {"aaaaaaaa000000000000000000000001"}})

	var diags diag.Diagnostics
	readEntryProperties(&page, &state, &diags)
	if diags.HasError() {
		t.Fatal(diags)
	}

	ms := mapOfLists(t, state.MultiSelectProperties)
	if !reflect.DeepEqual(ms, map[string][]string{"Tags": {"A", "B"}}) {
		t.Errorf("multi_select = %v. Same members in another order must keep the config order, an unconfigured key must stay out, and a deleted property drops out", ms)
	}
	rel := mapOfLists(t, state.RelationProperties)
	want := []string{"11111111-2222-3333-4444-555555555555", "66666666777788889999000000000000"}
	if !reflect.DeepEqual(rel["Links"], want) {
		t.Errorf("relation = %v, want %v (config form kept, new page normalized)", rel["Links"], want)
	}
	ppl := mapOfLists(t, state.PeopleProperties)
	if !reflect.DeepEqual(ppl["Owner"], []string{"aaaaaaaa000000000000000000000001"}) {
		t.Errorf("people = %v, want the config form", ppl["Owner"])
	}

	// Existing state without the new maps reads back without them.
	old := entryModel("e", "Row", "", "")
	readEntryProperties(&page, &old, &diags)
	if !old.MultiSelectProperties.IsNull() || !old.RelationProperties.IsNull() || !old.PeopleProperties.IsNull() {
		t.Errorf("unset maps must stay null on read")
	}
}

func relationResults(from, to int) string {
	var items []string
	for i := from; i < to; i++ {
		items = append(items, fmt.Sprintf(`{"object":"property_item","id":"l","type":"relation","relation":{"id":"%08d-0000-0000-0000-000000000000"}}`, i))
	}
	return strings.Join(items, ",")
}

func TestListPropertyItemIDs_Paginates(t *testing.T) {
	const token = "test-token-property-items"
	// newFakeNotion's handler only sees the decoded path, so wire a
	// server here that can also check the query and the raw path.
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.EscapedPath()+"?"+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("start_cursor") == "" {
			_, _ = fmt.Fprintf(w, `{"object":"list","type":"property_item","results":[%s],"has_more":true,"next_cursor":"c2","property_item":{"type":"relation","relation":{},"id":"l"}}`, relationResults(0, 100))
			return
		}
		_, _ = fmt.Fprintf(w, `{"object":"list","type":"property_item","results":[%s],"has_more":false,"next_cursor":null,"property_item":{"type":"relation","relation":{},"id":"l"}}`, relationResults(100, 130))
	}))
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	registerHTTPClient(token, &http.Client{Transport: rewriteHost{target: target}})
	t.Cleanup(func() { providerHTTPClients.Delete(token) })

	ids, err := listPropertyItemIDs(context.Background(), token, "pg", "a%3Fb", "relation")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 130 || ids[129] != "00000129-0000-0000-0000-000000000000" {
		t.Errorf("got %d ids, last %q", len(ids), ids[len(ids)-1])
	}
	if len(seen) != 2 || !strings.HasPrefix(seen[0], "/v1/pages/pg/properties/a%3Fb?") || !strings.Contains(seen[1], "start_cursor=c2") {
		t.Errorf("requests = %v", seen)
	}
}

// longRelationServer serves an entry page whose relation shows the first
// 25 of 30 references, the property item endpoint with all 30, and the
// SDK's database GET.
func longRelationServer(t *testing.T, token string) (*DatabaseEntryResource, *[]string) {
	t.Helper()
	var reqs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/pages/pg":
			var rel []string
			for i := 0; i < 25; i++ {
				rel = append(rel, fmt.Sprintf(`{"id":"%08d-0000-0000-0000-000000000000"}`, i))
			}
			_, _ = fmt.Fprintf(w, `{"object":"page","id":"pg","url":"https://www.notion.so/pg","parent":{"type":"database_id","database_id":"db"},
"properties":{"Name":{"id":"title","type":"title","title":[{"type":"text","plain_text":"Row","text":{"content":"Row"}}]},
"Links":{"id":"rel","type":"relation","relation":[%s],"has_more":true}}}`, strings.Join(rel, ","))
		case "/v1/pages/pg/properties/rel":
			_, _ = fmt.Fprintf(w, `{"object":"list","type":"property_item","results":[%s],"has_more":false,"next_cursor":null}`, relationResults(0, 30))
		default:
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"object":"error","code":"object_not_found","message":"not found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	hc := &http.Client{Transport: rewriteHost{target: target}}
	registerHTTPClient(token, hc)
	t.Cleanup(func() { providerHTTPClients.Delete(token) })
	c := notionapi.NewClient(notionapi.Token(token), notionapi.WithHTTPClient(hc))
	registerClientToken(c, token)
	return &DatabaseEntryResource{client: c, mdClient: newMarkdownClient(c)}, &reqs
}

func runEntryRead(t *testing.T, r *DatabaseEntryResource, prior DatabaseEntryResourceModel) DatabaseEntryResourceModel {
	t.Helper()
	ctx := context.Background()
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	null := tftypes.NewValue(sr.Schema.Type().TerraformType(ctx), nil)
	state := tfsdk.State{Schema: sr.Schema, Raw: null}
	if d := state.Set(ctx, &prior); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var out DatabaseEntryResourceModel
	if d := resp.State.Get(ctx, &out); d.HasError() {
		t.Fatal(d)
	}
	return out
}

// A relation longer than 25 is cut off in the page response. Read must
// fetch the rest, or a config with 30 IDs would show a diff forever.
func TestUnitDatabaseEntry_ReadExpandsLongRelation(t *testing.T) {
	r, reqs := longRelationServer(t, "secret_unit_long_relation")
	var ids []string
	for i := 0; i < 30; i++ {
		ids = append(ids, fmt.Sprintf("%08d000000000000000000000000", i))
	}
	prior := entryModel("pg", "Row", "", "")
	prior.RelationProperties = listMap(t, map[string][]string{"Links": ids})

	got := runEntryRead(t, r, prior)
	rel := mapOfLists(t, got.RelationProperties)["Links"]
	if !reflect.DeepEqual(rel, ids) {
		t.Errorf("relation after read has %d ids, want all 30 in config form: %v", len(rel), rel)
	}
	if !strings.Contains(strings.Join(*reqs, "\n"), "GET /v1/pages/pg/properties/rel") {
		t.Errorf("property item endpoint not called: %v", *reqs)
	}

	// Without a configured relation the extra request is skipped.
	*reqs = nil
	runEntryRead(t, r, entryModel("pg", "Row", "", ""))
	if strings.Contains(strings.Join(*reqs, "\n"), "/properties/") {
		t.Errorf("unmanaged relation must not be expanded: %v", *reqs)
	}
}

// Both create paths and the update must carry the list maps. The markdown
// path posts JSON built from the same properties, so check its body.
func TestUnitDatabaseEntry_MarkdownCreateSendsListProperties(t *testing.T) {
	f := newFakeMarkdownPages(t)
	r := newEntryTestResource(t, f)
	ctx := context.Background()

	planned := entryModel("", "Row", "## Notes", "v2")
	planned.ID = types.StringUnknown()
	planned.URL = types.StringUnknown()
	planned.MultiSelectProperties = listMap(t, map[string][]string{"Tags": {"A", "New option"}})
	planned.RelationProperties = listMap(t, map[string][]string{"Links": {"11111111222233334444555555555555"}})
	planned.PeopleProperties = listMap(t, map[string][]string{"Owner": {"u1"}})

	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	null := tftypes.NewValue(sr.Schema.Type().TerraformType(ctx), nil)
	plan := tfsdk.Plan{Schema: sr.Schema, Raw: null}
	if d := plan.Set(ctx, &planned); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: sr.Schema, Raw: null}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.creates) != 1 {
		t.Fatalf("made %d page creates, want 1", len(f.creates))
	}
	got, _ := json.Marshal(f.creates[0]["properties"])
	for _, want := range []string{
		`"Tags":{"multi_select":[{"name":"A"},{"name":"New option"}],"type":"multi_select"}`,
		`"Links":{"relation":[{"id":"11111111222233334444555555555555"}],"type":"relation"}`,
		`"Owner":{"people":[{"id":"u1"}],"type":"people"}`,
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("create body missing %s: %s", want, got)
		}
	}
}

func TestUnitDatabaseEntry_UpdateSendsAndClearsListProperties(t *testing.T) {
	f := newFakeMarkdownPages(t)
	r := newEntryTestResource(t, f)
	const id = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbb02"
	f.titles[id] = "Row"

	prior := entryModel(id, "Row", "", "")
	prior.MultiSelectProperties = listMap(t, map[string][]string{"Tags": {"A"}})
	prior.RelationProperties = listMap(t, map[string][]string{"Links": {"p1"}})
	planned := entryModel(id, "Row", "", "")
	planned.MultiSelectProperties = listMap(t, map[string][]string{"Tags": {"A", "B"}})

	resp := runEntryUpdate(t, r, prior, planned)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.updates) != 1 {
		t.Fatalf("made %d page updates, want 1", len(f.updates))
	}
	got, _ := json.Marshal(f.updates[0]["properties"])
	for _, want := range []string{
		`"Tags":{"multi_select":[{"name":"A"},{"name":"B"}],"type":"multi_select"}`,
		`"Links":{"relation":[],"type":"relation"}`,
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("update body missing %s: %s", want, got)
		}
	}
}

func TestValidateEntryListMaps(t *testing.T) {
	long := make([]string, 101)
	for i := range long {
		long[i] = fmt.Sprintf("p%d", i)
	}
	cases := []struct {
		name    string
		edit    func(m *DatabaseEntryResourceModel)
		wantErr string
	}{
		{"valid", func(m *DatabaseEntryResourceModel) {
			m.MultiSelectProperties = listMap(t, map[string][]string{"Tags": {"A", "B"}})
			m.RelationProperties = listMap(t, map[string][]string{"Links": {"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "11111111222233334444555555555555"}})
		}, ""},
		{"too many", func(m *DatabaseEntryResourceModel) {
			m.RelationProperties = listMap(t, map[string][]string{"Links": long})
		}, "Too many values"},
		{"comma", func(m *DatabaseEntryResourceModel) {
			m.MultiSelectProperties = listMap(t, map[string][]string{"Tags": {"a,b"}})
		}, "Invalid option name"},
		{"duplicate id in two forms", func(m *DatabaseEntryResourceModel) {
			m.PeopleProperties = listMap(t, map[string][]string{"Owner": {"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "aaaaaaaabbbbccccddddeeeeeeeeeeee"}})
		}, "Duplicate value"},
		{"unknown list skipped", func(m *DatabaseEntryResourceModel) {
			m.RelationProperties = types.MapValueMust(entryListElemType, map[string]attr.Value{"Links": types.ListUnknown(types.StringType)})
		}, ""},
	}
	for _, c := range cases {
		m := entryModel("", "Row", "", "")
		c.edit(&m)
		var diags diag.Diagnostics
		validateEntryListMaps(&m, &diags)
		switch {
		case c.wantErr == "" && diags.HasError():
			t.Errorf("%s: unexpected error %v", c.name, diags)
		case c.wantErr != "" && (!diags.HasError() || diags.Errors()[0].Summary() != c.wantErr):
			t.Errorf("%s: got %v, want %q", c.name, diags, c.wantErr)
		}
	}
}

// The acceptance configs plan cleanly through real Terraform, so a typo in
// them or in the schema shows up without a workspace.
func TestUnitDatabaseEntryLists_ValidPlans(t *testing.T) {
	const parent = "00000000000000000000000000000000"
	for _, cfg := range []string{
		testAccDatabaseEntryListsConfig(parent, `
  multi_select_properties = { "Tags" = ["A", "B"] }
  relation_properties     = { "Links" = [local.target_c_hyphenated] }
  phone_number_properties = { "Phone" = "+1 415 555 0100" }`),
		testAccDatabaseEntryListsConfig(parent, `
  relation_properties = { "Links" = [] }`),
		testAccDatabaseEntryPeopleConfig(parent, `["aaaaaaaa-0000-0000-0000-000000000001"]`),
	} {
		tfresource.UnitTest(t, tfresource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []tfresource.TestStep{{
				Config:             iconUnitProvider + cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			}},
		})
	}
}

func TestUnitDatabaseEntryLists_RejectedAtPlan(t *testing.T) {
	entry := func(body string) string {
		return `
resource "notion_database_entry" "t" {
  database = "00000000000000000000000000000000"
  title    = "x"
  ` + body + `
}`
	}
	planOnlyError(t, entry(`multi_select_properties = { "Tags" = ["a,b"] }`), `can't contain commas`)
	planOnlyError(t, entry(`relation_properties = { "Links" = ["aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "aaaaaaaabbbbccccddddeeeeeeeeeeee"] }`), `listed more than once`)
	planOnlyError(t, entry(`people_properties = { "Owner" = [for i in range(101) : format("u%d", i)] }`), `at most 100 values`)
}
