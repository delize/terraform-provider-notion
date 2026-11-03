package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

var testMarkdownWarnings = []markdownWarning{
	{Code: "deprecated_tag", Message: "Converted <details> to <toggle>."},
	{Code: "unsupported_tag", Message: "Saved <blink> as text."},
}

func checkWarningDiags(t *testing.T, label string, diags diag.Diagnostics) {
	t.Helper()
	if diags.HasError() {
		t.Fatalf("%s: unexpected error %v", label, diags)
	}
	if got := diags.WarningsCount(); got != len(testMarkdownWarnings) {
		t.Fatalf("%s: %d warnings, want %d: %v", label, got, len(testMarkdownWarnings), diags)
	}
	for i, w := range testMarkdownWarnings {
		d := diags.Warnings()[i]
		if !strings.Contains(d.Summary(), w.Code) || !strings.Contains(d.Detail(), w.Message) {
			t.Errorf("%s: warning %d is %q / %q, want code %q and message %q", label, i, d.Summary(), d.Detail(), w.Code, w.Message)
		}
	}
}

func TestUnitMarkdownWarnings_ParsedFromWrites(t *testing.T) {
	f := newFakeMarkdownPages(t)
	f.warnings = testMarkdownWarnings
	const id = "cccccccccccccccccccccccccccccc01"
	f.titles[id] = "Doc"
	mc := &markdownClient{token: "secret_unit_md_warnings"}
	ctx := context.Background()

	replaced, err := mc.ReplacePageMarkdown(ctx, id, "# A", "v2")
	if err != nil {
		t.Fatal(err)
	}
	inserted, err := mc.InsertPageMarkdown(ctx, id, "# B", "end", "v2")
	if err != nil {
		t.Fatal(err)
	}
	for label, got := range map[string][]markdownWarning{"replace": replaced.Warnings, "insert": inserted.Warnings} {
		var diags diag.Diagnostics
		appendMarkdownWarnings(&diags, id, got)
		checkWarningDiags(t, label, diags)
	}
}

func TestUnitPage_InsertWarningsBecomeDiagnostics(t *testing.T) {
	f := newFakeMarkdownPages(t)
	f.warnings = testMarkdownWarnings
	const id = "cccccccccccccccccccccccccccccc02"
	f.titles[id] = "Doc"
	r := &PageResource{mdClient: &markdownClient{token: "secret_unit_md_warnings"}}
	plan := &PageResourceModel{
		ID:              types.StringValue(id),
		MarkdownVersion: types.StringValue("v2"),
		MarkdownInsert:  &MarkdownInsertModel{Content: types.StringValue("<details>x</details>"), Position: types.StringValue("end")},
	}
	checkWarningDiags(t, "insert", r.applyMarkdownInsert(context.Background(), plan))
}

// Warnings are diagnostics of warning severity, so an apply that gets them
// still succeeds and saves state.
func TestUnitPage_WarningsDoNotFailApply(t *testing.T) {
	f := newFakeMarkdownPages(t)
	f.warnings = testMarkdownWarnings
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []tfresource.TestStep{
			{Config: pageResendConfig("Doc", "# Body", "v2")},
			{Config: pageResendConfig("Doc", "<details>x</details>", "v2"), Check: replaceCount(t, f, 1)},
		},
	})
}

func TestUnitDatabaseEntry_ReplaceWarningsBecomeDiagnostics(t *testing.T) {
	f := newFakeMarkdownPages(t)
	f.warnings = testMarkdownWarnings
	r := newEntryTestResource(t, f)
	const id = "cccccccccccccccccccccccccccccc03"
	f.titles[id] = "Row"
	resp := runEntryUpdate(t, r, entryModel(id, "Row", "# Old", "v2"), entryModel(id, "Row", "<details>x</details>", "v2"))
	checkWarningDiags(t, "entry replace", resp.Diagnostics)
}
