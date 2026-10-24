package provider

import (
	"encoding/json"
	"reflect"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func nullIcons() iconFields {
	return iconFields{
		Emoji:         types.StringValue(""),
		Name:          types.StringNull(),
		Color:         types.StringNull(),
		URL:           types.StringNull(),
		CustomEmojiID: types.StringNull(),
	}
}

func TestIconFieldsRequest(t *testing.T) {
	cases := map[string]struct {
		in   iconFields
		want string
	}{
		"none": {nullIcons(), "null"},
		"emoji": {func() iconFields { f := nullIcons(); f.Emoji = types.StringValue("🚀"); return f }(),
			`{"emoji":"🚀","type":"emoji"}`},
		"native default color": {func() iconFields { f := nullIcons(); f.Name = types.StringValue("pizza"); return f }(),
			`{"icon":{"name":"pizza"},"type":"icon"}`},
		"native with color": {func() iconFields {
			f := nullIcons()
			f.Name, f.Color = types.StringValue("pizza"), types.StringValue("blue")
			return f
		}(), `{"icon":{"color":"blue","name":"pizza"},"type":"icon"}`},
		"external": {func() iconFields { f := nullIcons(); f.URL = types.StringValue("https://example.com/a.png"); return f }(),
			`{"external":{"url":"https://example.com/a.png"},"type":"external"}`},
		"custom emoji": {func() iconFields { f := nullIcons(); f.CustomEmojiID = types.StringValue("45ce"); return f }(),
			`{"custom_emoji":{"id":"45ce"},"type":"custom_emoji"}`},
	}
	for name, c := range cases {
		got, _ := json.Marshal(c.in.request())
		if string(got) != c.want {
			t.Errorf("%s: got %s, want %s", name, got, c.want)
		}
	}
}

func TestParseIcon(t *testing.T) {
	cases := map[string]parsedIcon{
		`null`:                         {},
		`{"type":"emoji","emoji":"😻"}`: {Type: "emoji", Emoji: "😻"},
		`{"type":"icon","icon":{"name":"pizza","color":"blue"}}`:                       {Type: "icon", Name: "pizza", Color: "blue"},
		`{"type":"external","external":{"url":"https://e.x/i.png"}}`:                   {Type: "external", URL: "https://e.x/i.png"},
		`{"type":"file","file":{"url":"https://s3/x","expiry_time":"t"}}`:              {Type: "file", URL: "https://s3/x"},
		`{"type":"custom_emoji","custom_emoji":{"id":"45ce","name":"bufo","url":"u"}}`: {Type: "custom_emoji", CustomEmojiID: "45ce"},
	}
	for in, want := range cases {
		if got := parseIcon(json.RawMessage(in)); got != want {
			t.Errorf("parseIcon(%s) = %+v, want %+v", in, got, want)
		}
	}
}

// Unmanaged attributes stay null even when Notion has a native icon, so an
// icon set in the UI isn't drift.
func TestReconcile_UnmanagedStaysNull(t *testing.T) {
	got := nullIcons().reconcile(parsedIcon{Type: "icon", Name: "pizza", Color: "blue"})
	if !got.Name.IsNull() || !got.Color.IsNull() || got.Emoji.ValueString() != "" {
		t.Errorf("unmanaged icon leaked into state: %+v", got)
	}
}

// A picker-style name matching the internal name after folding is kept as
// configured. A different name reads back as Notion's.
func TestReconcile_NameFolding(t *testing.T) {
	f := nullIcons()
	f.Name = types.StringValue("Star Circle")
	got := f.reconcile(parsedIcon{Type: "icon", Name: "star_circle", Color: "gray"})
	if got.Name.ValueString() != "Star Circle" {
		t.Errorf("folded name not kept: %q", got.Name.ValueString())
	}
	if !got.Color.IsNull() {
		t.Errorf("gray with unset color should stay null, got %q", got.Color.ValueString())
	}

	got = f.reconcile(parsedIcon{Type: "icon", Name: "token", Color: "gray"})
	if got.Name.ValueString() != "token" {
		t.Errorf("alias should read back as server name, got %q", got.Name.ValueString())
	}
}

func TestReconcile_ColorDrift(t *testing.T) {
	f := nullIcons()
	f.Name = types.StringValue("pizza")
	if got := f.reconcile(parsedIcon{Type: "icon", Name: "pizza", Color: "red"}); got.Color.ValueString() != "red" {
		t.Errorf("non-gray color with unset config should surface, got %v", got.Color)
	}
	f.Color = types.StringValue("blue")
	if got := f.reconcile(parsedIcon{Type: "icon", Name: "pizza", Color: "gray"}); got.Color.ValueString() != "gray" {
		t.Errorf("managed color should report server value, got %v", got.Color)
	}
}

func TestReconcile_TypeChangedIsDrift(t *testing.T) {
	f := nullIcons()
	f.Name = types.StringValue("pizza")
	got := f.reconcile(parsedIcon{Type: "emoji", Emoji: "🍕"})
	if !got.Name.IsNull() || got.Emoji.ValueString() != "🍕" {
		t.Errorf("type change not reflected: %+v", got)
	}
}

func TestReconcile_CustomEmojiIDNormalized(t *testing.T) {
	f := nullIcons()
	f.CustomEmojiID = types.StringValue("45ce454cd4274f539489e5d0f3d1db6b")
	got := f.reconcile(parsedIcon{Type: "custom_emoji", CustomEmojiID: "45ce454c-d427-4f53-9489-e5d0f3d1db6b"})
	if got.CustomEmojiID.ValueString() != "45ce454cd4274f539489e5d0f3d1db6b" {
		t.Errorf("hyphenated ID should match configured form, got %q", got.CustomEmojiID.ValueString())
	}
}

func TestIsNotionIconSetURL(t *testing.T) {
	cases := map[string]bool{
		"https://www.notion.so/icons/pizza_blue.svg":  true,
		"https://notion.com/icons/pizza_blue.svg":     true,
		"https://app.notion.com/icons/token_gray.svg": true,
		"https://www.notion.so/images/logo.png":       false,
		"https://example.com/icons/pizza.svg":         false,
	}
	for in, want := range cases {
		if got := isNotionIconSetURL(in); got != want {
			t.Errorf("isNotionIconSetURL(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestPagePositionRequest(t *testing.T) {
	if pagePositionRequest("", "") != nil {
		t.Error("empty should be nil")
	}
	if got := pagePositionRequest("page_start", ""); !reflect.DeepEqual(got, map[string]interface{}{"type": "page_start"}) {
		t.Errorf("page_start: %v", got)
	}
	got, _ := json.Marshal(pagePositionRequest("", "abc"))
	if string(got) != `{"after_block":{"id":"abc"},"type":"after_block"}` {
		t.Errorf("after_block: %s", got)
	}
}

func TestDecodePageObject(t *testing.T) {
	body := `{
	  "object":"page","id":"a-b","url":"https://app.notion.com/p/ab","in_trash":false,"is_locked":true,
	  "parent":{"type":"page_id","page_id":"p-1"},
	  "icon":{"type":"icon","icon":{"name":"pizza","color":"blue"}},
	  "properties":{"title":{"id":"title","type":"title","title":[
	    {"type":"text","text":{"content":"Docs","link":{"url":"https://e.x"}},"plain_text":"Docs"},
	    {"type":"text","text":{"content":" home"},"plain_text":" home"}
	  ]}}
	}`
	p, err := decodePageObject([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsLocked || p.Parent.PageID != "p-1" || parseIcon(p.Icon).Name != "pizza" {
		t.Errorf("unexpected decode: %+v", p)
	}
	if title, ok := p.title(); !ok || title != "[Docs](https://e.x) home" {
		t.Errorf("title: %q %v", title, ok)
	}
}

const iconUnitProvider = `
provider "notion" {
  token = "unit-test-token"
}
`

func planOnlyError(t *testing.T, config, pattern string) {
	t.Helper()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      iconUnitProvider + config,
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(pattern),
		}},
	})
}

func TestUnitPageIcon_Conflicts(t *testing.T) {
	planOnlyError(t, `
resource "notion_page" "t" {
  parent_page_id = "00000000000000000000000000000000"
  title          = "x"
  icon           = "🚀"
  icon_name      = "pizza"
}`, `Only one of icon, icon_name, icon_url`)
}

func TestUnitPageIcon_ColorNeedsName(t *testing.T) {
	planOnlyError(t, `
resource "notion_page" "t" {
  parent_page_id = "00000000000000000000000000000000"
  title          = "x"
  icon_color     = "blue"
}`, `icon_color only applies to a native icon`)
}

func TestUnitPageIcon_BadColor(t *testing.T) {
	planOnlyError(t, `
resource "notion_page" "t" {
  parent_page_id = "00000000000000000000000000000000"
  title          = "x"
  icon_name      = "pizza"
  icon_color     = "teal"
}`, `"teal" is not a native icon color`)
}

func TestUnitPageIcon_NotionIconURLRejected(t *testing.T) {
	planOnlyError(t, `
resource "notion_page" "t" {
  parent_page_id = "00000000000000000000000000000000"
  title          = "x"
  icon_url       = "https://www.notion.so/icons/pizza_blue.svg"
}`, `Use icon_name for Notion icons`)
}

func TestUnitPagePosition_Invalid(t *testing.T) {
	planOnlyError(t, `
resource "notion_page" "t" {
  parent_page_id = "00000000000000000000000000000000"
  title          = "x"
  position       = "top"
}`, `"top" is not a page position`)
}

func TestUnitPagePosition_Conflict(t *testing.T) {
	planOnlyError(t, `
resource "notion_page" "t" {
  parent_page_id = "00000000000000000000000000000000"
  title          = "x"
  position       = "page_end"
  after_block_id = "00000000000000000000000000000001"
}`, `Set either position or after_block_id`)
}

func TestUnitDatabaseIcon_Conflicts(t *testing.T) {
	planOnlyError(t, `
resource "notion_database" "t" {
  parent               = "00000000000000000000000000000000"
  title                = "x"
  title_column_title   = "Name"
  icon_url             = "https://example.com/i.png"
  icon_custom_emoji_id = "45ce"
}`, `Only one of icon, icon_name, icon_url`)
}

// A valid config with every new page attribute plans cleanly.
func TestUnitPageNewAttributes_ValidPlans(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: iconUnitProvider + `
resource "notion_page" "t" {
  parent_page_id = "00000000000000000000000000000000"
  title          = "x"
  icon_name      = "pizza"
  icon_color     = "blue"
  is_locked      = true
  position       = "page_start"
}

resource "notion_database" "t" {
  parent             = "00000000000000000000000000000000"
  title              = "x"
  title_column_title = "Name"
  icon               = "🚀"
  is_locked          = false
}`,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		}},
	})
}
