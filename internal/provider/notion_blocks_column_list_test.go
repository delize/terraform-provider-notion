package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// Plan-only config rules for column_list, through real Terraform.
func TestUnitBlockColumnList_ConfigRules(t *testing.T) {
	cases := []struct {
		name, config, err string
	}{
		{"missing columns", `
resource "notion_block" "c" {
  parent_id = "00000000000000000000000000000000"
  type      = "column_list"
}`, `A new column_list needs at least two entries in columns`},
		{"one column", `
resource "notion_block" "c" {
  parent_id = "00000000000000000000000000000000"
  type      = "column_list"
  columns   = [{}]
}`, `at least two columns`},
		{"columns on another type", `
resource "notion_block" "c" {
  parent_id = "00000000000000000000000000000000"
  type      = "paragraph"
  columns   = [{}, {}]
}`, `columns only applies to blocks of type column_list`},
		{"text on a column list", `
resource "notion_block" "c" {
  parent_id = "00000000000000000000000000000000"
  type      = "column_list"
  rich_text = "nope"
  columns   = [{}, {}]
}`, `no effect on a column_list block`},
		{"ratio out of range", `
resource "notion_block" "c" {
  parent_id = "00000000000000000000000000000000"
  type      = "column_list"
  columns   = [{ width_ratio = 1 }, { width_ratio = 0 }]
}`, `greater than 0 and less than 1`},
		{"ratio on some columns", `
resource "notion_block" "c" {
  parent_id = "00000000000000000000000000000000"
  type      = "column_list"
  columns   = [{ width_ratio = 0.5 }, {}]
}`, `Set width_ratio on every column or none`},
		{"ratios not adding up", `
resource "notion_block" "c" {
  parent_id = "00000000000000000000000000000000"
  type      = "column_list"
  columns   = [{ width_ratio = 0.3 }, { width_ratio = 0.3 }]
}`, `must add up to 1`},
		{"standalone column", `
resource "notion_block" "c" {
  parent_id = "00000000000000000000000000000000"
  type      = "column"
}`, `Create columns through column_list`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps:                    []resource.TestStep{blockUnitStep(tc.config, tc.err)},
			})
		})
	}
}

// A valid column list plans and applies, and other block types plan
// columns as null rather than as a computed value. heading_4 stands in for
// the other types because it also goes over direct HTTP to the fake.
func TestUnitBlockColumnList_ValidPlan(t *testing.T) {
	newFakeColumnBlocks(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: columnListConfig(`  columns   = [{ width_ratio = 0.333 }, { width_ratio = 0.333 }, { width_ratio = 0.334 }]
`) + `
resource "notion_block" "h" {
  parent_id = "11111111111111111111111111111111"
  type      = "heading_4"
}
`,
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					plancheck.ExpectKnownValue("notion_block.h", tfjsonpath.New("columns"), knownvalue.Null()),
					plancheck.ExpectKnownValue("notion_block.cols", tfjsonpath.New("columns").AtSliceIndex(0).AtMapKey("width_ratio"), knownvalue.Float64Exact(0.333)),
					plancheck.ExpectUnknownValue("notion_block.cols", tfjsonpath.New("columns").AtSliceIndex(0).AtMapKey("id")),
				},
			},
			Check: resource.TestCheckResourceAttr("notion_block.cols", "columns.2.width_ratio", "0.334"),
		}},
	})
}

func TestColumnListBlockShape(t *testing.T) {
	half := 0.5
	got := mustJSON(t, columnListBlock([]columnSpec{{WidthRatio: &half}, {}}))
	var parsed struct {
		Type       string `json:"type"`
		ColumnList struct {
			Children []struct {
				Type   string `json:"type"`
				Column struct {
					WidthRatio *float64 `json:"width_ratio"`
					Children   []struct {
						Type      string `json:"type"`
						Paragraph struct {
							RichText []interface{} `json:"rich_text"`
						} `json:"paragraph"`
					} `json:"children"`
				} `json:"column"`
			} `json:"children"`
		} `json:"column_list"`
	}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatal(err)
	}
	cols := parsed.ColumnList.Children
	if parsed.Type != "column_list" || len(cols) != 2 {
		t.Fatalf("unexpected column list: %s", got)
	}
	for i, c := range cols {
		if c.Type != "column" || len(c.Column.Children) != 1 || c.Column.Children[0].Type != "paragraph" || len(c.Column.Children[0].Paragraph.RichText) != 0 {
			t.Errorf("column %d should hold one empty paragraph: %s", i, got)
		}
	}
	if cols[0].Column.WidthRatio == nil || *cols[0].Column.WidthRatio != 0.5 {
		t.Errorf("first column ratio: %s", got)
	}
	// Notion's schema has no null for width_ratio, so an unset ratio leaves
	// the key out.
	if cols[1].Column.WidthRatio != nil || strings.Count(got, "width_ratio") != 1 {
		t.Errorf("second column should have no width_ratio: %s", got)
	}
}

// col builds a columnModel. ratio is a float64, nil for null, or "unknown".
func col(id string, ratio interface{}) columnModel {
	c := columnModel{ID: types.StringValue(id), WidthRatio: types.Float64Null()}
	switch r := ratio.(type) {
	case float64:
		c.WidthRatio = types.Float64Value(r)
	case string:
		if r == "unknown" {
			c.WidthRatio = types.Float64Unknown()
		}
	}
	return c
}

func TestPlanColumnOps(t *testing.T) {
	prior := []columnModel{col("a", 0.5), col("b", 0.5), col("c", nil)}

	ops := planColumnOps(prior, []columnModel{col("a", 0.5), col("b", 0.25), col("c", "unknown"), col("", nil)})
	if len(ops.Update) != 1 || ops.Update[0] != 1 || len(ops.Append) != 1 || ops.Append[0] != 3 || len(ops.Delete) != 0 {
		t.Errorf("ratio change + append: %+v", ops)
	}

	ops = planColumnOps(prior, prior[:2])
	if len(ops.Update) != 0 || len(ops.Append) != 0 || len(ops.Delete) != 1 || ops.Delete[0] != 2 {
		t.Errorf("shrink: %+v", ops)
	}

	// A ratio within float noise isn't a change, and a ratio set where the
	// prior had none is.
	ops = planColumnOps(prior, []columnModel{col("a", 0.5000000001), col("b", 0.5), col("c", 0.2)})
	if len(ops.Update) != 1 || ops.Update[0] != 2 {
		t.Errorf("noise and new ratio: %+v", ops)
	}
}

func TestColumnsFromChildren(t *testing.T) {
	var page struct {
		Results []rawBlock `json:"results"`
	}
	if err := json.Unmarshal([]byte(`{"results":[
    {"object":"block","id":"aaaa-1111","type":"column","has_children":true,"column":{"width_ratio":0.30000001}},
    {"object":"block","id":"bbbb-2222","type":"column","has_children":true,"column":{}},
    {"object":"block","id":"cccc-3333","type":"column","in_trash":true,"column":{}},
    {"object":"block","id":"dddd-4444","type":"paragraph","paragraph":{"rich_text":[]}}
  ]}`), &page); err != nil {
		t.Fatal(err)
	}

	cols := columnsFromChildren(page.Results, []columnModel{col("aaaa1111", 0.3)})
	if len(cols) != 2 {
		t.Fatalf("want the two live columns, got %+v", cols)
	}
	if cols[0].ID.ValueString() != "aaaa1111" || cols[0].WidthRatio.ValueFloat64() != 0.3 {
		t.Errorf("column 0 should keep the prior ratio within float noise: %+v", cols[0])
	}
	if cols[1].ID.ValueString() != "bbbb2222" || !cols[1].WidthRatio.IsNull() {
		t.Errorf("column 1 has no ratio: %+v", cols[1])
	}

	// An import has no prior state and takes Notion's value.
	if cols := columnsFromChildren(page.Results, nil); cols[0].WidthRatio.ValueFloat64() != 0.30000001 {
		t.Errorf("import ratio: %+v", cols[0])
	}
}

func TestReconcileColumns(t *testing.T) {
	const token = "test-token-columns"
	f := newFakeNotion(t, token, func(method, path, body string) (int, string) {
		switch {
		case method == http.MethodPatch && path == "/v1/blocks/list1/children":
			return 200, `{"results":[{"object":"block","id":"new-4","type":"column","column":{}}]}`
		case method == http.MethodPatch:
			return 200, `{"object":"block","id":"b","type":"column","column":{"width_ratio":0.25}}`
		case method == http.MethodDelete:
			return 200, `{"object":"block","id":"x","type":"column","in_trash":true}`
		}
		return 404, `{"object":"error"}`
	})

	prior := []columnModel{col("a", nil), col("b", 0.5), col("c", nil)}
	got, err := reconcileColumns(context.Background(), token, "list1", prior,
		[]columnModel{col("a", "unknown"), col("b", 0.25), col("c", "unknown"), col("", "unknown")})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range got {
		ids = append(ids, c.ID.ValueString())
	}
	if strings.Join(ids, ",") != "a,b,c,new4" {
		t.Errorf("ids: %v", ids)
	}
	if !got[0].WidthRatio.IsUnknown() || got[1].WidthRatio.ValueFloat64() != 0.25 || !got[3].WidthRatio.IsNull() {
		t.Errorf("ratios: %+v", got)
	}

	reqs := strings.Join(f.reqs, "\n")
	for _, want := range []string{
		`PATCH /v1/blocks/b {"column":{"width_ratio":0.25}}`,
		`PATCH /v1/blocks/list1/children {"children":[{"column":{"children":[{"object":"block","paragraph":{"rich_text":[]},"type":"paragraph"}]},"object":"block","type":"column"}]}`,
	} {
		if !strings.Contains(reqs, want) {
			t.Errorf("missing request %q in:\n%s", want, reqs)
		}
	}
	if strings.Contains(reqs, "DELETE") || strings.Contains(reqs, "/v1/blocks/a ") {
		t.Errorf("only b's ratio changed and nothing is removed:\n%s", reqs)
	}

	f.reqs = nil
	if _, err := reconcileColumns(context.Background(), token, "list1", prior, prior[:2]); err != nil {
		t.Fatal(err)
	}
	if reqs := strings.Join(f.reqs, "\n"); !strings.Contains(reqs, "DELETE /v1/blocks/c") || strings.Count(reqs, "DELETE") != 1 {
		t.Errorf("expected only c deleted:\n%s", reqs)
	}
}
