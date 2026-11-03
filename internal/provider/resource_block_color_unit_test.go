package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jomei/notionapi"
)

// coloredSDKBlocks returns one SDK block per colored type on the SDK path,
// each carrying the given color the way Notion returns it.
func coloredSDKBlocks(color string) map[string]notionapi.Block {
	h := notionapi.Heading{Color: color}
	return map[string]notionapi.Block{
		"paragraph":          &notionapi.ParagraphBlock{Paragraph: notionapi.Paragraph{Color: color}},
		"heading_1":          &notionapi.Heading1Block{Heading1: h},
		"heading_2":          &notionapi.Heading2Block{Heading2: h},
		"heading_3":          &notionapi.Heading3Block{Heading3: h},
		"bulleted_list_item": &notionapi.BulletedListItemBlock{BulletedListItem: notionapi.ListItem{Color: color}},
		"numbered_list_item": &notionapi.NumberedListItemBlock{NumberedListItem: notionapi.ListItem{Color: color}},
		"to_do":              &notionapi.ToDoBlock{ToDo: notionapi.ToDo{Color: color}},
		"toggle":             &notionapi.ToggleBlock{Toggle: notionapi.Toggle{Color: color}},
		"quote":              &notionapi.QuoteBlock{Quote: notionapi.Quote{Color: color}},
		"table_of_contents":  &notionapi.TableOfContentsBlock{TableOfContents: notionapi.TableOfContents{Color: color}},
	}
}

// Notion reports an unset color as "default". Every colored SDK type must
// keep "" in state when the prior value was empty or missing (create with
// no color, import), and keep "default" when the config set it.
func TestReadBlockIntoState_NormalizesDefaultColor(t *testing.T) {
	cases := []struct {
		name  string
		api   string
		prior types.String
		want  string
	}{
		{"unset config", "default", types.StringValue(""), ""},
		{"import", "default", types.StringNull(), ""},
		{"explicit default", "default", types.StringValue("default"), "default"},
		{"real color", "red", types.StringValue(""), "red"},
		{"color changed in the UI", "blue_background", types.StringValue("red"), "blue_background"},
	}
	for _, c := range cases {
		for typ, block := range coloredSDKBlocks(c.api) {
			state := BlockResourceModel{Color: c.prior}
			readBlockIntoState(block, &state)
			if got := state.Color.ValueString(); got != c.want || state.Color.IsNull() {
				t.Errorf("%s, %s: color = %q (null %v), want %q", c.name, typ, got, state.Color.IsNull(), c.want)
			}
		}
	}
}

// An empty color on update is sent as "default" so a color removed from
// the config is reset in Notion. The SDK would otherwise omit it.
func TestBuildBlockUpdateRequest_EmptyColorSendsDefault(t *testing.T) {
	blockTypes := []string{"paragraph", "heading_1", "heading_2", "heading_3", "bulleted_list_item",
		"numbered_list_item", "to_do", "toggle", "quote"}
	for _, typ := range blockTypes {
		for color, want := range map[string]string{"": `"color":"default"`, "red": `"color":"red"`} {
			plan := BlockResourceModel{
				Type:     types.StringValue(typ),
				RichText: types.StringValue("x"),
				Color:    types.StringValue(color),
			}
			req, err := buildBlockUpdateRequest(plan)
			if err != nil {
				t.Fatalf("%s: %v", typ, err)
			}
			b, _ := json.Marshal(req)
			if !strings.Contains(string(b), want) {
				t.Errorf("%s with color %q: body %s lacks %s", typ, color, b, want)
			}
		}
	}
}

// default_background is in the Update a block color enum.
func TestUnitBlockColor_DefaultBackgroundAccepted(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			blockUnitStep(`
resource "notion_block" "p" {
  parent_id = "00000000000000000000000000000000"
  type      = "paragraph"
  rich_text = "x"
  color     = "default_background"
}`, ""),
			blockUnitStep(`
resource "notion_block" "p" {
  parent_id = "00000000000000000000000000000000"
  type      = "paragraph"
  rich_text = "x"
  color     = "default_backgrounds"
}`, `Expected one of`),
		},
	})
}

// An import starts from empty state, so attributes the block type doesn't
// use must come back as their schema defaults, the same as after a create.
func TestReadBlockIntoState_ImportFillsDefaults(t *testing.T) {
	var state BlockResourceModel
	readBlockIntoState(&notionapi.Heading1Block{Heading1: notionapi.Heading{Color: "default"}}, &state)
	for name, v := range map[string]types.String{
		"color": state.Color, "icon": state.Icon, "language": state.Language,
		"caption": state.Caption, "url": state.URL, "expression": state.Expression,
	} {
		if v.IsNull() || v.ValueString() != "" {
			t.Errorf("%s = %v, want \"\"", name, v)
		}
	}
	if state.Checked.IsNull() || state.Checked.ValueBool() || state.IsToggleable.IsNull() {
		t.Errorf("checked = %v, is_toggleable = %v, want false", state.Checked, state.IsToggleable)
	}
}
