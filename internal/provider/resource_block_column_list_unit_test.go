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
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeColumnBlocks serves just enough of /v1/blocks for a column_list. It
// enforces the documented create rules (at least two columns, each with at
// least one child) so a request the real API would refuse fails here too.
// While hideChildren is set, listing a list's columns returns nothing.
type fakeColumnBlocks struct {
	mu           sync.Mutex
	next         int
	blocks       map[string]map[string]interface{}
	children     map[string][]string
	hideChildren bool
	created      int
	deleted      []string
	ratioUpdates []string
	createBody   string
	appendBody   string
}

func newFakeColumnBlocks(t *testing.T) *fakeColumnBlocks {
	t.Helper()
	f := &fakeColumnBlocks{blocks: map[string]map[string]interface{}{}, children: map[string][]string{}}
	srv := httptest.NewServer(f)
	prev := notionAPIBaseURL
	notionAPIBaseURL = srv.URL + "/v1"
	t.Cleanup(func() {
		notionAPIBaseURL = prev
		srv.Close()
	})
	return f
}

func (f *fakeColumnBlocks) id() string {
	f.next++
	return fmt.Sprintf("c%031d", f.next)
}

// addColumn stores a column from a request body under listID and returns
// it, or an error message when the column breaks a documented rule.
func (f *fakeColumnBlocks) addColumn(listID string, in map[string]interface{}) (map[string]interface{}, string) {
	if in["type"] != "column" {
		return nil, "only columns can be appended to a column_list"
	}
	body, _ := in["column"].(map[string]interface{})
	kids, _ := body["children"].([]interface{})
	if len(kids) == 0 {
		return nil, "each column must have at least one child"
	}
	col := map[string]interface{}{}
	if r, ok := body["width_ratio"]; ok {
		col["width_ratio"] = r
	}
	blk := map[string]interface{}{
		"object": "block", "id": f.id(), "type": "column", "has_children": true, "column": col,
		"parent": map[string]string{"type": "block_id", "block_id": listID},
	}
	f.blocks[blk["id"].(string)] = blk
	f.children[listID] = append(f.children[listID], blk["id"].(string))
	return blk, ""
}

// liveColumns returns the column blocks of a list that aren't in the trash.
func (f *fakeColumnBlocks) liveColumns(listID string) []map[string]interface{} {
	var out []map[string]interface{}
	for _, id := range f.children[listID] {
		if f.blocks[id]["in_trash"] != true {
			out = append(out, f.blocks[id])
		}
	}
	return out
}

// setRatio stands in for resizing a column in the Notion app.
func (f *fakeColumnBlocks) setRatio(listID string, i int, v float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.liveColumns(listID)[i]["column"] = map[string]interface{}{"width_ratio": v}
}

func (f *fakeColumnBlocks) onlyList() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, b := range f.blocks {
		if b["type"] == "column_list" && b["in_trash"] != true {
			return id
		}
	}
	return ""
}

func (f *fakeColumnBlocks) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var raw json.RawMessage
	_ = json.NewDecoder(r.Body).Decode(&raw)
	var body map[string]interface{}
	_ = json.Unmarshal(raw, &body)
	w.Header().Set("Content-Type", "application/json")
	reply := func(status int, v interface{}) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	invalid := func(msg string) {
		reply(400, map[string]string{"object": "error", "code": "validation_error", "message": msg})
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/blocks/")
	id, sub, _ := strings.Cut(rest, "/")

	switch {
	case r.Method == http.MethodPatch && sub == "children" && f.blocks[id] != nil:
		// Appending columns to an existing column list.
		f.appendBody = string(raw)
		var out []interface{}
		for _, c := range body["children"].([]interface{}) {
			col, msg := f.addColumn(id, c.(map[string]interface{}))
			if msg != "" {
				invalid(msg)
				return
			}
			out = append(out, col)
		}
		reply(200, map[string]interface{}{"results": out})
	case r.Method == http.MethodPatch && sub == "" && f.blocks[id] != nil:
		blk := f.blocks[id]
		if c, ok := body["column"].(map[string]interface{}); ok {
			f.ratioUpdates = append(f.ratioUpdates, fmt.Sprintf("%s=%v", id, c["width_ratio"]))
			blk["column"] = c
		}
		reply(200, blk)
	case r.Method == http.MethodPatch && sub == "children":
		f.createBody = string(raw)
		in := body["children"].([]interface{})[0].(map[string]interface{})
		if in["type"] != "column_list" {
			// Any other block is stored as sent.
			in["id"] = f.id()
			in["parent"] = map[string]string{"type": "page_id", "page_id": id}
			f.blocks[in["id"].(string)] = in
			reply(200, map[string]interface{}{"results": []interface{}{in}})
			return
		}
		cols := in["column_list"].(map[string]interface{})["children"].([]interface{})
		if len(cols) < 2 {
			invalid("a column_list must have at least two columns")
			return
		}
		listID := f.id()
		f.created++
		block := map[string]interface{}{
			"object": "block", "id": listID, "type": "column_list", "has_children": true, "column_list": map[string]interface{}{},
			"parent": map[string]string{"type": "page_id", "page_id": id},
		}
		f.blocks[listID] = block
		for _, c := range cols {
			if _, msg := f.addColumn(listID, c.(map[string]interface{})); msg != "" {
				invalid(msg)
				return
			}
		}
		reply(200, map[string]interface{}{"results": []interface{}{block}})
	case r.Method == http.MethodGet && sub == "children":
		var kids []map[string]interface{}
		if !f.hideChildren {
			kids = f.liveColumns(id)
		}
		reply(200, map[string]interface{}{"results": kids, "has_more": false})
	case r.Method == http.MethodGet && f.blocks[id] != nil:
		reply(200, f.blocks[id])
	case r.Method == http.MethodDelete && f.blocks[id] != nil:
		f.deleted = append(f.deleted, id)
		f.blocks[id]["in_trash"] = true
		reply(200, f.blocks[id])
	default:
		reply(404, map[string]string{"object": "error", "code": "object_not_found", "message": r.Method + " " + r.URL.Path})
	}
}

func columnListConfig(columns string) string {
	return fmt.Sprintf(`
provider "notion" {
  token = "secret_unit_columns"
}

resource "notion_block" "cols" {
  parent_id = "11111111111111111111111111111111"
  type      = "column_list"
%s}
`, columns)
}

const twoColumns = `  columns   = [{}, {}]
`

const threeColumns = `  columns   = [{}, {}, {}]
`

// saveAttr copies an attribute out of state so a later step can compare.
func saveAttr(addr, key string, into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return fmt.Errorf("%s not in state", addr)
		}
		*into = rs.Primary.Attributes[key]
		return nil
	}
}

// Create, append a column, remove it again, and import. Each step ends with
// Terraform's own empty-plan check, so the placeholder paragraph in every
// new column never shows up as drift.
func TestUnitBlockColumnList_Lifecycle(t *testing.T) {
	f := newFakeColumnBlocks(t)
	var first, second, third string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: columnListConfig(twoColumns),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_block.cols", "type", "column_list"),
					resource.TestCheckResourceAttr("notion_block.cols", "has_children", "true"),
					resource.TestCheckResourceAttr("notion_block.cols", "columns.#", "2"),
					resource.TestCheckResourceAttrSet("notion_block.cols", "columns.0.id"),
					resource.TestCheckResourceAttrSet("notion_block.cols", "columns.1.id"),
					resource.TestCheckNoResourceAttr("notion_block.cols", "columns.0.width_ratio"),
					resource.TestCheckResourceAttr("notion_block.cols", "rich_text", ""),
					saveAttr("notion_block.cols", "columns.0.id", &first),
					saveAttr("notion_block.cols", "columns.1.id", &second),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						body := f.createBody
						if n := strings.Count(body, `"type":"column"`); n != 2 {
							return fmt.Errorf("want 2 columns in the create request, got %d: %s", n, body)
						}
						if n := strings.Count(body, `"type":"paragraph"`); n != 2 {
							return fmt.Errorf("want one placeholder paragraph per column, got %d: %s", n, body)
						}
						if strings.Contains(body, "width_ratio") {
							return fmt.Errorf("no width_ratio expected: %s", body)
						}
						return nil
					},
				),
			},
			{
				Config: columnListConfig(threeColumns),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_block.cols", "columns.#", "3"),
					resource.TestCheckResourceAttrSet("notion_block.cols", "columns.2.id"),
					saveAttr("notion_block.cols", "columns.2.id", &third),
					func(s *terraform.State) error {
						attrs := s.RootModule().Resources["notion_block.cols"].Primary.Attributes
						if attrs["columns.0.id"] != first || attrs["columns.1.id"] != second {
							return fmt.Errorf("existing columns changed id: %s %s", attrs["columns.0.id"], attrs["columns.1.id"])
						}
						f.mu.Lock()
						defer f.mu.Unlock()
						if !strings.Contains(f.appendBody, `"type":"paragraph"`) {
							return fmt.Errorf("appended column has no placeholder: %s", f.appendBody)
						}
						if f.created != 1 || len(f.deleted) != 0 {
							return fmt.Errorf("adding a column must not replace the list (created %d, deleted %v)", f.created, f.deleted)
						}
						return nil
					},
				),
			},
			{
				Config: columnListConfig(twoColumns),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_block.cols", "columns.#", "2"),
					func(s *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if len(f.deleted) != 1 || f.deleted[0] != third {
							return fmt.Errorf("want only the third column %s deleted, got %v", third, f.deleted)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "notion_block.cols",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// width_ratio goes out on create and a change updates the columns in place.
func TestUnitBlockColumnList_WidthRatio(t *testing.T) {
	f := newFakeColumnBlocks(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: columnListConfig(`  columns   = [{ width_ratio = 0.25 }, { width_ratio = 0.75 }]
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_block.cols", "columns.0.width_ratio", "0.25"),
					resource.TestCheckResourceAttr("notion_block.cols", "columns.1.width_ratio", "0.75"),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if !strings.Contains(f.createBody, `"width_ratio":0.25`) || !strings.Contains(f.createBody, `"width_ratio":0.75`) {
							return fmt.Errorf("ratios missing from create: %s", f.createBody)
						}
						return nil
					},
				),
			},
			{
				Config: columnListConfig(`  columns   = [{ width_ratio = 0.5 }, { width_ratio = 0.5 }]
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_block.cols", "columns.0.width_ratio", "0.5"),
					resource.TestCheckResourceAttr("notion_block.cols", "columns.1.width_ratio", "0.5"),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if len(f.ratioUpdates) != 2 || f.created != 1 {
							return fmt.Errorf("want two in-place ratio updates and no replace, got %v (created %d)", f.ratioUpdates, f.created)
						}
						return nil
					},
				),
			},
		},
	})
}

// A width set in the Notion app on columns with no width_ratio configured
// is taken into state without planning a change, and a column added later
// reads back whatever Notion reports.
func TestUnitBlockColumnList_UnconfiguredRatioFollowsNotion(t *testing.T) {
	f := newFakeColumnBlocks(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: columnListConfig(twoColumns)},
			{
				PreConfig: func() {
					list := f.onlyList()
					f.setRatio(list, 0, 0.3)
					f.setRatio(list, 1, 0.7)
				},
				Config: columnListConfig(twoColumns),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_block.cols", "columns.0.width_ratio", "0.3"),
					resource.TestCheckResourceAttr("notion_block.cols", "columns.1.width_ratio", "0.7"),
				),
			},
			{
				Config: columnListConfig(threeColumns),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_block.cols", "columns.#", "3"),
					resource.TestCheckResourceAttr("notion_block.cols", "columns.0.width_ratio", "0.3"),
					resource.TestCheckNoResourceAttr("notion_block.cols", "columns.2.width_ratio"),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if len(f.ratioUpdates) != 0 {
							return fmt.Errorf("unconfigured ratios must not be sent: %v", f.ratioUpdates)
						}
						return nil
					},
				),
			},
		},
	})
}

// A column list whose columns can't be read back after create must still be
// saved, so the next apply replaces it rather than leaving it orphaned.
func TestUnitBlockColumnList_CreateReadFailureKeepsState(t *testing.T) {
	f := newFakeColumnBlocks(t)
	f.hideChildren = true
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      columnListConfig(twoColumns),
				ExpectError: regexp.MustCompile(`Error reading new columns`),
			},
			{
				PreConfig: func() {
					f.mu.Lock()
					f.hideChildren = false
					f.mu.Unlock()
				},
				Config: columnListConfig(twoColumns),
				Check:  resource.TestCheckResourceAttr("notion_block.cols", "columns.#", "2"),
			},
		},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.created != 2 {
		t.Fatalf("created %d column lists, want the tainted first one replaced (2 created)", f.created)
	}
}

// An existing column list with no columns in its config, like one imported
// before columns existed, keeps planning nothing.
func TestUnitBlockColumnList_ColumnsLeftOutOfConfig(t *testing.T) {
	f := newFakeColumnBlocks(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: columnListConfig(twoColumns)},
			{
				Config: columnListConfig(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_block.cols", "columns.#", "2"),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if len(f.deleted) != 0 || f.created != 1 {
							return fmt.Errorf("leaving columns out must change nothing (created %d, deleted %v)", f.created, f.deleted)
						}
						return nil
					},
				),
			},
		},
	})
}
