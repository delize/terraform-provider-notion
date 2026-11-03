package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jomei/notionapi"
)

func TestMarkdownNeedsReplace(t *testing.T) {
	s := types.StringValue
	null := types.StringNull()
	cases := []struct {
		name                           string
		planMd, stateMd, planV, stateV types.String
		want                           bool
	}{
		{"unchanged", s("a"), s("a"), null, null, false},
		{"text changed", s("b"), s("a"), null, null, true},
		{"markdown newly set", s("a"), null, null, null, true},
		{"markdown removed", null, s("a"), null, null, false},
		{"markdown unknown", types.StringUnknown(), s("a"), null, null, false},
		{"version v1 to v2", s("a"), s("a"), s("v2"), s("v1"), true},
		{"unset to v2", s("a"), s("a"), s("v2"), null, true},
		{"unset to explicit v1 is the same syntax", s("a"), s("a"), s("v1"), null, false},
		{"v1 back to unset", s("a"), s("a"), null, s("v1"), false},
		{"v2 back to unset", s("a"), s("a"), null, s("v2"), true},
	}
	for _, c := range cases {
		if got := markdownNeedsReplace(c.planMd, c.stateMd, c.planV, c.stateV); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func pageResendConfig(title, markdown, version string) string {
	v := ""
	if version != "" {
		v = fmt.Sprintf("  markdown_version = %q\n", version)
	}
	return fmt.Sprintf(`
provider "notion" {
  token = "secret_unit_md_resend"
}

resource "notion_page" "p" {
  parent_page_id = "11111111111111111111111111111111"
  title          = %q
  markdown       = %q
%s}
`, title, markdown, v)
}

// replaceCount runs after a step and asserts how many replace_content
// PATCHes the step made, then clears the log for the next step.
func replaceCount(t *testing.T, f *fakeMarkdownPages, want int) tfresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		got := 0
		for _, c := range f.markdownCalls() {
			if c.Method == http.MethodPatch && c.Type == "replace_content" {
				got++
			}
		}
		f.reset()
		if got != want {
			return fmt.Errorf("step made %d replace_content calls, want %d", got, want)
		}
		return nil
	}
}

// A title-only update must not resend the page body. Changing the markdown
// or the effective version sends exactly one replace.
func TestUnitPage_MarkdownResentOnlyOnChange(t *testing.T) {
	f := newFakeMarkdownPages(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []tfresource.TestStep{
			{Config: pageResendConfig("One", "# Body", ""), Check: replaceCount(t, f, 0)},
			{Config: pageResendConfig("Two", "# Body", ""), Check: replaceCount(t, f, 0)},
			{Config: pageResendConfig("Two", "# Body changed", ""), Check: replaceCount(t, f, 1)},
			{Config: pageResendConfig("Two", "# Body changed", "v1"), Check: replaceCount(t, f, 0)},
			{Config: pageResendConfig("Two", "# Body changed", "v2"), Check: replaceCount(t, f, 1)},
			{Config: pageResendConfig("Three", "# Body changed", "v2"), Check: replaceCount(t, f, 0)},
		},
	})
}

// entryFake serves the SDK's GET /v1/databases/{id} on top of the page fake,
// so DatabaseEntryResource.Update can run against it.
func newEntryTestResource(t *testing.T, f *fakeMarkdownPages) *DatabaseEntryResource {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/databases/db" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"database","id":"db","properties":{"title":{"id":"title","type":"title","title":{}}}}`))
			return
		}
		f.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	notionAPIBaseURL = srv.URL + "/v1"
	target, _ := url.Parse(srv.URL)
	const token = "secret_unit_md_entry_update"
	c := notionapi.NewClient(notionapi.Token(token), notionapi.WithHTTPClient(&http.Client{Transport: rewriteHost{target: target}}))
	registerClientToken(c, token)
	return &DatabaseEntryResource{client: c, mdClient: newMarkdownClient(c)}
}

func entryModel(id, title, markdown, version string) DatabaseEntryResourceModel {
	str := func(v string) types.String {
		if v == "" {
			return types.StringNull()
		}
		return types.StringValue(v)
	}
	return DatabaseEntryResourceModel{
		ID:                    types.StringValue(id),
		Database:              types.StringValue("db"),
		Title:                 types.StringValue(title),
		URL:                   types.StringValue("https://www.notion.so/" + id),
		Markdown:              str(markdown),
		MarkdownVersion:       str(version),
		RichTextProperties:    types.MapNull(types.StringType),
		NumberProperties:      types.MapNull(types.Float64Type),
		CheckboxProperties:    types.MapNull(types.BoolType),
		SelectProperties:      types.MapNull(types.StringType),
		StatusProperties:      types.MapNull(types.StringType),
		URLProperties:         types.MapNull(types.StringType),
		EmailProperties:       types.MapNull(types.StringType),
		PhoneNumberProperties: types.MapNull(types.StringType),
		DateProperties:        types.MapNull(types.StringType),
	}
}

// runEntryUpdate calls Update directly with the given prior state and plan
// and returns the diagnostics. The SDK has no base URL option, so the
// entry can't run through a real Terraform plan against a fake.
func runEntryUpdate(t *testing.T, r *DatabaseEntryResource, prior, planned DatabaseEntryResourceModel) resource.UpdateResponse {
	t.Helper()
	ctx := context.Background()
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	null := tftypes.NewValue(sr.Schema.Type().TerraformType(ctx), nil)

	state := tfsdk.State{Schema: sr.Schema, Raw: null}
	plan := tfsdk.Plan{Schema: sr.Schema, Raw: null}
	config := tfsdk.Config{Schema: sr.Schema, Raw: null}
	if d := state.Set(ctx, &prior); d.HasError() {
		t.Fatal(d)
	}
	if d := plan.Set(ctx, &planned); d.HasError() {
		t.Fatal(d)
	}
	configState := tfsdk.State{Schema: sr.Schema, Raw: null}
	if d := configState.Set(ctx, &planned); d.HasError() {
		t.Fatal(d)
	}
	config.Raw = configState.Raw

	resp := resource.UpdateResponse{State: tfsdk.State{Schema: sr.Schema, Raw: null}}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state, Config: config}, &resp)
	return resp
}

func TestUnitDatabaseEntry_MarkdownResentOnlyOnChange(t *testing.T) {
	f := newFakeMarkdownPages(t)
	r := newEntryTestResource(t, f)
	const id = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbb01"
	f.titles[id] = "Row"

	steps := []struct {
		name           string
		prior, planned DatabaseEntryResourceModel
		wantReplace    int
		wantVersion    string
	}{
		{"title only", entryModel(id, "Row", "# Body", ""), entryModel(id, "Row 2", "# Body", ""), 0, ""},
		{"markdown changed", entryModel(id, "Row 2", "# Body", ""), entryModel(id, "Row 2", "# New", ""), 1, ""},
		{"version changed", entryModel(id, "Row 2", "# New", ""), entryModel(id, "Row 2", "# New", "v2"), 1, "v2"},
		{"title only on v2", entryModel(id, "Row 2", "# New", "v2"), entryModel(id, "Row 3", "# New", "v2"), 0, ""},
	}
	for _, s := range steps {
		f.reset()
		resp := runEntryUpdate(t, r, s.prior, s.planned)
		if resp.Diagnostics.HasError() {
			t.Fatalf("%s: %v", s.name, resp.Diagnostics)
		}
		var replaces []mdCall
		for _, c := range f.markdownCalls() {
			if c.Method == http.MethodPatch && c.Type == "replace_content" {
				replaces = append(replaces, c)
			}
		}
		if len(replaces) != s.wantReplace {
			t.Fatalf("%s: %d replace_content calls, want %d", s.name, len(replaces), s.wantReplace)
		}
		if s.wantReplace > 0 && replaces[0].Version != s.wantVersion {
			t.Errorf("%s: replace sent markdown_version %q, want %q", s.name, replaces[0].Version, s.wantVersion)
		}
	}
}

func patchCount(t *testing.T, f *fakeMarkdownPages, typ string, want int) tfresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		got := 0
		for _, c := range f.markdownCalls() {
			if c.Method == http.MethodPatch && c.Type == typ {
				got++
			}
		}
		f.reset()
		if got != want {
			return fmt.Errorf("step made %d %s calls, want %d", got, typ, want)
		}
		return nil
	}
}

func pageInsertConfig(title, markdown, content, position string) string {
	insert := ""
	if content != "" {
		insert = fmt.Sprintf(`
  markdown_insert = {
    content  = %q
    position = %q
  }
`, content, position)
	}
	return fmt.Sprintf(`
provider "notion" {
  token = "secret_unit_md_insert"
}

resource "notion_page" "p" {
  parent_page_id = "11111111111111111111111111111111"
  title          = %q
  markdown       = %q
%s}
`, title, markdown, insert)
}

// An insert adds content each time it runs and can't be undone, so it must
// run only when markdown_insert is added or its content or position changes.
func TestUnitPage_MarkdownInsertOnlyOnChange(t *testing.T) {
	f := newFakeMarkdownPages(t)
	inserts := func(n int) tfresource.TestCheckFunc { return patchCount(t, f, "insert_content", n) }
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []tfresource.TestStep{
			{Config: pageInsertConfig("One", "# Body", "A", "end"), Check: inserts(1)},
			{Config: pageInsertConfig("Two", "# Body", "A", "end"), Check: inserts(0)},
			{Config: pageInsertConfig("Two", "# Body changed", "A", "end"), Check: inserts(0)},
			{Config: pageInsertConfig("Two", "# Body changed", "B", "end"), Check: inserts(1)},
			{Config: pageInsertConfig("Two", "# Body changed", "B", "start"), Check: inserts(1)},
			{Config: pageInsertConfig("Two", "# Body changed", "", ""), Check: inserts(0)},
			{Config: pageInsertConfig("Three", "# Body changed", "", ""), Check: inserts(0)},
			{Config: pageInsertConfig("Three", "# Body changed", "B", "start"), Check: inserts(1)},
		},
	})
}
