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

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeCallouts serves the slice of /v1/blocks a callout needs. It keeps
// each callout's object as Notion would return it and logs every write so
// tests can check the request bodies.
type fakeCallouts struct {
	mu     sync.Mutex
	next   int
	blocks map[string]map[string]interface{}
	writes []string // "METHOD /path body"
}

const calloutParentID = "11111111111111111111111111111111"

func newFakeCallouts(t *testing.T) *fakeCallouts {
	t.Helper()
	f := &fakeCallouts{blocks: map[string]map[string]interface{}{}}
	srv := httptest.NewServer(f)
	prev := notionAPIBaseURL
	notionAPIBaseURL = srv.URL + "/v1"
	t.Cleanup(func() {
		notionAPIBaseURL = prev
		srv.Close()
	})
	return f
}

// seed adds an existing callout, as if made in the Notion UI or by an
// older provider version.
func (f *fakeCallouts) seed(id string, callout map[string]interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blocks[id] = map[string]interface{}{
		"object": "block", "id": id, "type": "callout", "has_children": false,
		"parent":  map[string]string{"type": "page_id", "page_id": calloutParentID},
		"callout": callout,
	}
}

// setIcon changes a callout's icon behind Terraform's back.
func (f *fakeCallouts) setIcon(id string, icon interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blocks[id]["callout"].(map[string]interface{})["icon"] = icon
}

func (f *fakeCallouts) icon(id string) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.blocks[id]["callout"].(map[string]interface{})["icon"]
}

func (f *fakeCallouts) writeLog() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.writes, "\n")
}

// echoIcon turns a request icon into the response shape: a native icon
// without a color reads back gray, and a custom emoji gains a name.
func echoIcon(in interface{}) interface{} {
	icon, ok := in.(map[string]interface{})
	if !ok {
		return in
	}
	switch icon["type"] {
	case "icon":
		inner := icon["icon"].(map[string]interface{})
		if _, ok := inner["color"]; !ok {
			inner["color"] = "gray"
		}
	case "custom_emoji":
		icon["custom_emoji"].(map[string]interface{})["name"] = "bufo"
	}
	return icon
}

func fillPlainText(callout map[string]interface{}) {
	for _, rt := range callout["rich_text"].([]interface{}) {
		rt := rt.(map[string]interface{})
		rt["plain_text"] = rt["text"].(map[string]interface{})["content"]
	}
}

func (f *fakeCallouts) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, _ := json.Marshal(map[string]interface{}{})
	var body map[string]interface{}
	if r.Body != nil {
		dec := json.NewDecoder(r.Body)
		_ = dec.Decode(&body)
		raw, _ = json.Marshal(body)
	}
	if r.Method != http.MethodGet {
		f.writes = append(f.writes, r.Method+" "+r.URL.Path+" "+string(raw))
	}
	w.Header().Set("Content-Type", "application/json")
	reply := func(status int, v interface{}) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/blocks/")
	id, sub, _ := strings.Cut(rest, "/")

	switch {
	case r.Method == http.MethodPatch && sub == "children":
		in := body["children"].([]interface{})[0].(map[string]interface{})
		callout := in["callout"].(map[string]interface{})
		fillPlainText(callout)
		if icon, ok := callout["icon"]; ok {
			callout["icon"] = echoIcon(icon)
		} else {
			callout["icon"] = nil
		}
		f.next++
		newID := fmt.Sprintf("%032d", f.next+100)
		block := map[string]interface{}{
			"object": "block", "id": newID, "type": "callout", "has_children": false,
			"parent":  map[string]string{"type": "page_id", "page_id": id},
			"callout": callout,
		}
		f.blocks[newID] = block
		reply(200, map[string]interface{}{"results": []interface{}{block}})
	case r.Method == http.MethodPatch && f.blocks[id] != nil:
		cur := f.blocks[id]["callout"].(map[string]interface{})
		upd := body["callout"].(map[string]interface{})
		fillPlainText(upd)
		cur["rich_text"] = upd["rich_text"]
		if c, ok := upd["color"]; ok {
			cur["color"] = c
		}
		if icon, ok := upd["icon"]; ok {
			cur["icon"] = echoIcon(icon)
		}
		reply(200, f.blocks[id])
	case r.Method == http.MethodGet && f.blocks[id] != nil:
		reply(200, f.blocks[id])
	case r.Method == http.MethodDelete && f.blocks[id] != nil:
		f.blocks[id]["in_trash"] = true
		reply(200, f.blocks[id])
	default:
		reply(404, map[string]string{"object": "error", "code": "object_not_found", "message": r.Method + " " + r.URL.Path})
	}
}

func calloutConfig(extra string) string {
	return fmt.Sprintf(`
provider "notion" {
  token = "secret_unit_callout"
}

resource "notion_block" "c" {
  parent_id = %q
  type      = "callout"
  rich_text = "Heads up"
%s
}
`, calloutParentID, extra)
}

// The first created callout gets this ID from the fake.
const firstCalloutID = "00000000000000000000000000000101"

// TestUnitBlockCallout_IconLifecycle creates a callout with a native icon,
// switches it to an emoji, checks that removing the icon is refused before
// anything is sent, and imports it.
func TestUnitBlockCallout_IconLifecycle(t *testing.T) {
	f := newFakeCallouts(t)
	const addr = "notion_block.c"
	writesBefore := -1
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: calloutConfig(`
  icon_name  = "pizza"
  icon_color = "blue"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", firstCalloutID),
					resource.TestCheckResourceAttr(addr, "icon_name", "pizza"),
					resource.TestCheckResourceAttr(addr, "icon_color", "blue"),
					resource.TestCheckResourceAttr(addr, "icon", ""),
					resource.TestCheckResourceAttr(addr, "color", ""),
					resource.TestCheckResourceAttr(addr, "rich_text", "Heads up"),
					func(*terraform.State) error {
						log := f.writeLog()
						for _, want := range []string{
							`"icon":{"icon":{"color":"blue","name":"pizza"},"type":"icon"}`,
							`"color":"default"`,
						} {
							if !strings.Contains(log, want) {
								return fmt.Errorf("create request missing %s:\n%s", want, log)
							}
						}
						return nil
					},
				),
			},
			{
				Config: calloutConfig(`  icon = "💡"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(addr, "icon_name"),
					resource.TestCheckNoResourceAttr(addr, "icon_color"),
					resource.TestCheckResourceAttr(addr, "icon", "💡"),
					resource.TestCheckResourceAttr(addr, "id", firstCalloutID),
				),
			},
			{
				PreConfig: func() {
					f.mu.Lock()
					writesBefore = len(f.writes)
					f.mu.Unlock()
				},
				Config:      calloutConfig(``),
				ExpectError: regexp.MustCompile(`can't remove a callout icon`),
			},
			{
				PreConfig: func() {
					f.mu.Lock()
					defer f.mu.Unlock()
					if len(f.writes) != writesBefore {
						t.Errorf("the refused removal sent requests:\n%s", strings.Join(f.writes[writesBefore:], "\n"))
					}
				},
				Config:            calloutConfig(`  icon = "💡"`),
				ResourceName:      addr,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: calloutConfig(`  icon = "💡"`),
				Check:  resource.TestCheckResourceAttr(addr, "icon", "💡"),
			},
		},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(strings.Join(f.writes, "\n"), `"icon":null`) {
		t.Errorf("icon null must never be sent:\n%s", strings.Join(f.writes, "\n"))
	}
	got, _ := json.Marshal(f.blocks[firstCalloutID]["callout"].(map[string]interface{})["icon"])
	if string(got) != `{"emoji":"💡","type":"emoji"}` {
		t.Errorf("icon in Notion changed: %s", got)
	}
}

// TestUnitBlockCallout_URLAndCustomEmoji covers the other two icon kinds,
// including a custom emoji ID written with dashes.
func TestUnitBlockCallout_URLAndCustomEmoji(t *testing.T) {
	f := newFakeCallouts(t)
	const addr = "notion_block.c"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: calloutConfig(`  icon_url = "https://example.com/icon.png"`),
				Check:  resource.TestCheckResourceAttr(addr, "icon_url", "https://example.com/icon.png"),
			},
			{
				Config: calloutConfig(`  icon_custom_emoji_id = "45ce454c-d427-4f53-9489-e5d0f3d1db6b"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(addr, "icon_url"),
					resource.TestCheckResourceAttr(addr, "icon_custom_emoji_id", "45ce454c-d427-4f53-9489-e5d0f3d1db6b"),
					func(*terraform.State) error {
						if !strings.Contains(f.writeLog(), `"custom_emoji":{"id":"45ce454c-d427-4f53-9489-e5d0f3d1db6b"}`) {
							return fmt.Errorf("custom emoji not sent:\n%s", f.writeLog())
						}
						return nil
					},
				),
			},
		},
	})
}

// TestUnitBlockCallout_ImportExistingEmoji imports an emoji callout like
// the ones earlier provider versions made through the SDK, and checks that
// the same config plans no changes afterwards.
func TestUnitBlockCallout_ImportExistingEmoji(t *testing.T) {
	f := newFakeCallouts(t)
	const id = "22222222222222222222222222222222"
	f.seed(id, map[string]interface{}{
		"rich_text": []interface{}{map[string]interface{}{
			"type": "text", "text": map[string]interface{}{"content": "Heads up"}, "plain_text": "Heads up",
		}},
		"color": "gray_background",
		"icon":  map[string]interface{}{"type": "emoji", "emoji": "💡"},
	})
	config := calloutConfig(`
  icon  = "💡"
  color = "gray_background"`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             config,
				ResourceName:       "notion_block.c",
				ImportState:        true,
				ImportStateId:      id,
				ImportStatePersist: true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					s := states[0]
					for k, want := range map[string]string{"icon": "💡", "color": "gray_background", "rich_text": "Heads up", "type": "callout", "parent_id": calloutParentID} {
						if s.Attributes[k] != want {
							return fmt.Errorf("%s = %q, want %q", k, s.Attributes[k], want)
						}
					}
					for _, k := range calloutIconAttrs {
						if v, ok := s.Attributes[k]; ok && v != "" {
							return fmt.Errorf("%s should be unset after import, got %q", k, v)
						}
					}
					return nil
				},
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
	if strings.Contains(f.writeLog(), "PATCH") {
		t.Errorf("an unchanged import should not write:\n%s", f.writeLog())
	}
}

// TestUnitBlockCallout_IconTypeChangedIsDrift changes the icon kind in
// Notion. A configured emoji replaced by a native icon is put back, and so
// is a configured native icon replaced by an emoji. A native icon added in
// the UI to a callout with no icon configured isn't tracked.
func TestUnitBlockCallout_IconTypeChangedIsDrift(t *testing.T) {
	f := newFakeCallouts(t)
	const addr = "notion_block.c"
	emojiConfig := calloutConfig(`  icon = "💡"`)
	nativeConfig := calloutConfig(`  icon_name = "pizza"`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: emojiConfig},
			{
				PreConfig: func() {
					f.setIcon(firstCalloutID, map[string]interface{}{"type": "icon", "icon": map[string]interface{}{"name": "token", "color": "red"}})
				},
				Config: emojiConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				Check: func(*terraform.State) error {
					got, _ := json.Marshal(f.icon(firstCalloutID))
					if string(got) != `{"emoji":"💡","type":"emoji"}` {
						return fmt.Errorf("emoji not restored: %s", got)
					}
					return nil
				},
			},
			{Config: nativeConfig},
			{
				PreConfig: func() {
					f.setIcon(firstCalloutID, map[string]interface{}{"type": "emoji", "emoji": "🔥"})
				},
				Config: nativeConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "icon_name", "pizza"),
					resource.TestCheckResourceAttr(addr, "icon", ""),
				),
			},
		},
	})
}

// TestUnitBlockCallout_UnconfiguredIcon covers a callout created with no
// icon that gets one in the Notion app. A native icon isn't tracked, so it
// plans nothing. An emoji always shows in `icon`, so it plans an update
// that is refused, since the API can't remove it.
func TestUnitBlockCallout_UnconfiguredIcon(t *testing.T) {
	f := newFakeCallouts(t)
	const addr = "notion_block.c"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: calloutConfig(``),
				Check: func(*terraform.State) error {
					if strings.Contains(f.writeLog(), `"icon"`) {
						return fmt.Errorf("no icon should be sent:\n%s", f.writeLog())
					}
					return nil
				},
			},
			{
				PreConfig: func() {
					f.setIcon(firstCalloutID, map[string]interface{}{"type": "icon", "icon": map[string]interface{}{"name": "token", "color": "gray"}})
				},
				Config: calloutConfig(``),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				PreConfig: func() {
					f.setIcon(firstCalloutID, map[string]interface{}{"type": "emoji", "emoji": "🔥"})
				},
				Config:      calloutConfig(``),
				ExpectError: regexp.MustCompile(`can't remove a callout icon`),
			},
			{
				Config: calloutConfig(`  icon = "🔥"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr(addr, "icon", "🔥"),
			},
		},
	})
}

func TestReadRawBlockIntoState_Callout(t *testing.T) {
	b, err := parseRawBlock([]byte(`{"object":"block","id":"aaaa-bbbb","type":"callout","has_children":true,
		"parent":{"type":"page_id","page_id":"pppp-qqqq"},
		"callout":{"rich_text":[{"type":"text","text":{"content":"Kale"},"plain_text":"Kale"}],"color":"gray_background","icon":{"type":"emoji","emoji":"💡"}}}`))
	if err != nil {
		t.Fatal(err)
	}

	// State as an earlier provider version left it: emoji set, the new
	// icon attributes absent (null). Reading must change nothing.
	state := BlockResourceModel{
		Icon: types.StringValue("💡"), Color: types.StringValue("gray_background"), RichText: types.StringValue("Kale"),
		RichTextJSON: types.StringNull(), TabItems: types.ListNull(types.ObjectType{AttrTypes: tabItemAttrTypes}),
	}
	readRawBlockIntoState(b, &state)
	if state.Icon.ValueString() != "💡" || state.Color.ValueString() != "gray_background" || state.RichText.ValueString() != "Kale" || !state.HasChildren.ValueBool() {
		t.Errorf("fields: %+v", state)
	}
	for _, v := range []types.String{state.IconName, state.IconColor, state.IconURL, state.IconCustomEmojiID} {
		if !v.IsNull() {
			t.Errorf("unconfigured icon attribute became %s", v)
		}
	}

	// A stale emoji in state is cleared when the callout has a native icon.
	b.Callout.Icon = json.RawMessage(`{"type":"icon","icon":{"name":"pizza","color":"gray"}}`)
	readRawBlockIntoState(b, &state)
	if state.Icon.ValueString() != "" {
		t.Errorf("stale emoji kept: %q", state.Icon.ValueString())
	}
}

func TestCalloutBody(t *testing.T) {
	got := mustJSON(t, calloutBody(plainToRichText("Hi"), "", nil))
	if !strings.Contains(got, `"color":"default"`) || strings.Contains(got, `"icon"`) {
		t.Errorf("unexpected body: %s", got)
	}
	got = mustJSON(t, calloutBody(nil, "red", nil))
	if strings.Contains(got, `"icon"`) || !strings.Contains(got, `"rich_text":[]`) || !strings.Contains(got, `"color":"red"`) {
		t.Errorf("no-icon body: %s", got)
	}
	icon := iconFields{Emoji: types.StringValue(""), Name: types.StringValue("pizza")}.request()
	got = mustJSON(t, calloutBody(nil, "", icon))
	if !strings.Contains(got, `"icon":{"icon":{"name":"pizza"},"type":"icon"}`) {
		t.Errorf("icon body: %s", got)
	}
}

func TestCalloutIconRemoval(t *testing.T) {
	none := iconFields{Emoji: types.StringValue(""), Name: types.StringNull(), Color: types.StringNull(), URL: types.StringNull(), CustomEmojiID: types.StringNull()}
	emoji, native, url := none, none, none
	emoji.Emoji = types.StringValue("💡")
	native.Name = types.StringValue("pizza")
	url.URL = types.StringValue("https://example.com/i.png")
	newIcon := map[string]interface{}{"type": "emoji", "emoji": "🔥"}

	for _, c := range []struct {
		prior   iconFields
		planned map[string]interface{}
		want    string
	}{
		{none, nil, ""},
		{emoji, nil, "icon"},
		{native, nil, "icon_name"},
		{url, nil, "icon_url"},
		{emoji, newIcon, ""},
		{native, newIcon, ""},
	} {
		if got := calloutIconRemoval(c.prior, c.planned); got != c.want {
			t.Errorf("calloutIconRemoval(%+v, %v) = %q, want %q", c.prior, c.planned, got, c.want)
		}
	}
}

// Plan-time checks through real Terraform for the callout icon attributes.

func TestUnitBlockCalloutIcon_OnlyOnCallout(t *testing.T) {
	for _, attr := range []string{`icon_name = "pizza"`, `icon_color = "blue"`, `icon_url = "https://example.com/i.png"`, `icon_custom_emoji_id = "abc"`} {
		name := strings.SplitN(attr, " ", 2)[0]
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{blockUnitStep(fmt.Sprintf(`
resource "notion_block" "p" {
  parent_id = "00000000000000000000000000000000"
  type      = "paragraph"
  rich_text = "hi"
  %s
}`, attr), regexp.QuoteMeta(name)+` only applies to blocks of type callout`)},
		})
	}
}

func TestUnitBlockCalloutIcon_RejectedOnTab(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{blockUnitStep(`
resource "notion_block" "t" {
  parent_id = "00000000000000000000000000000000"
  type      = "tab"
  icon_name = "pizza"
  tab_items = [{ label = "One" }]
}`, `icon_name only applies to blocks of type callout, not "tab"`)},
	})
}

func TestUnitBlockCalloutIcon_Conflicts(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{blockUnitStep(`
resource "notion_block" "c" {
  parent_id = "00000000000000000000000000000000"
  type      = "callout"
  icon      = "💡"
  icon_name = "pizza"
}`, `Only one of icon, icon_name, icon_url, and icon_custom_emoji_id`)},
	})
}

func TestUnitBlockCalloutIcon_ColorNeedsName(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{blockUnitStep(`
resource "notion_block" "c" {
  parent_id  = "00000000000000000000000000000000"
  type       = "callout"
  icon_color = "blue"
}`, `icon_color only applies to a native icon`)},
	})
}

func TestUnitBlockCalloutIcon_BadColor(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{blockUnitStep(`
resource "notion_block" "c" {
  parent_id  = "00000000000000000000000000000000"
  type       = "callout"
  icon_name  = "pizza"
  icon_color = "teal"
}`, `is not a native icon color`)},
	})
}

func TestUnitBlockCalloutIcon_NotionIconURLRejected(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{blockUnitStep(`
resource "notion_block" "c" {
  parent_id = "00000000000000000000000000000000"
  type      = "callout"
  icon_url  = "https://www.notion.so/icons/pizza_blue.svg"
}`, `Use icon_name for Notion icons`)},
	})
}

func TestUnitBlockCalloutIcon_ValidPlan(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{blockUnitStep(`
resource "notion_block" "native" {
  parent_id  = "00000000000000000000000000000000"
  type       = "callout"
  rich_text  = "Note"
  icon_name  = "pizza"
  icon_color = "blue"
}

resource "notion_block" "emoji" {
  parent_id = "00000000000000000000000000000000"
  type      = "callout"
  rich_text = "Note"
  icon      = "💡"
}`, ``)},
	})
}
