package provider

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// rich_text_json can't store what Notion returns byte for byte. Notion adds
// plain_text, href, and the full annotations object to every rich text item,
// so the stored string would differ from the configured one right after
// apply, and Terraform would fail the apply with "Provider produced
// inconsistent result". The helpers here keep the prior string (the plan on
// create and update, the state on refresh) when Notion's rich text means the
// same thing, and otherwise store Notion's rich text in request shape so
// real drift still shows. notion_view's JSON attributes do the same with
// viewJSONReadback.

// richTextJSONReadback returns the rich_text_json value to store once Notion
// has returned rt for a block.
func richTextJSONReadback(prior types.String, rt []notionapi.RichText) types.String {
	if !prior.IsNull() && !prior.IsUnknown() && richTextMatchesJSON(prior.ValueString(), rt) {
		return prior
	}
	s, err := richTextToRequestJSON(rt)
	if err != nil {
		return prior
	}
	return types.StringValue(s)
}

// richTextMatchesJSON reports whether the rich text Notion returned is what
// the JSON a user wrote asks for. Only the request side of the rich text
// object is compared. plain_text and href are response-only and ignored.
// Annotations the JSON leaves out must still be at Notion's defaults (false,
// and "default" for color), so styling added in the Notion app is drift.
func richTextMatchesJSON(prior string, rt []notionapi.RichText) bool {
	var want []map[string]interface{}
	if err := json.Unmarshal([]byte(prior), &want); err != nil || len(want) != len(rt) {
		return false
	}
	for i := range want {
		if !richTextItemMatches(want[i], rt[i]) {
			return false
		}
	}
	return true
}

func richTextItemMatches(want map[string]interface{}, got notionapi.RichText) bool {
	typ, _ := want["type"].(string)
	if typ == "" {
		// The rich text docs make type optional on requests when the
		// matching object is present.
		for _, k := range []string{"text", "mention", "equation"} {
			if _, ok := want[k]; ok {
				typ = k
				break
			}
		}
	}
	if typ != richTextType(got) {
		return false
	}
	switch typ {
	case "text":
		w, _ := want["text"].(map[string]interface{})
		if w == nil || got.Text == nil {
			return false
		}
		if c, _ := w["content"].(string); c != got.Text.Content {
			return false
		}
		wantURL := ""
		if l, ok := w["link"].(map[string]interface{}); ok {
			wantURL, _ = l["url"].(string)
		}
		gotURL := ""
		if got.Text.Link != nil {
			gotURL = got.Text.Link.Url
		}
		if wantURL != gotURL {
			return false
		}
	case "equation":
		w, _ := want["equation"].(map[string]interface{})
		if w == nil || got.Equation == nil {
			return false
		}
		if e, _ := w["expression"].(string); e != got.Equation.Expression {
			return false
		}
	case "mention":
		if got.Mention == nil {
			return false
		}
		// A mention response carries more than the request (a user
		// mention comes back with the full user, for one). Compare only
		// the keys the JSON sets.
		gotMention, err := toGenericJSON(got.Mention)
		if err != nil || !jsonSubset(want["mention"], gotMention) {
			return false
		}
	default:
		return false
	}
	return annotationsMatch(want["annotations"], got.Annotations)
}

func richTextType(rt notionapi.RichText) string {
	if rt.Type != "" {
		return string(rt.Type)
	}
	switch {
	case rt.Text != nil:
		return "text"
	case rt.Mention != nil:
		return "mention"
	case rt.Equation != nil:
		return "equation"
	}
	return ""
}

// annotationDefaults are the values Notion reports for annotations nobody
// set, per the rich text reference.
var annotationDefaults = map[string]interface{}{
	"bold":          false,
	"italic":        false,
	"strikethrough": false,
	"underline":     false,
	"code":          false,
	"color":         "default",
}

func annotationsToMap(a *notionapi.Annotations) map[string]interface{} {
	if a == nil {
		return annotationDefaults
	}
	color := string(a.Color)
	if color == "" {
		color = "default"
	}
	return map[string]interface{}{
		"bold":          a.Bold,
		"italic":        a.Italic,
		"strikethrough": a.Strikethrough,
		"underline":     a.Underline,
		"code":          a.Code,
		"color":         color,
	}
}

func annotationsMatch(want interface{}, got *notionapi.Annotations) bool {
	w, _ := want.(map[string]interface{})
	gotMap := annotationsToMap(got)
	for k, def := range annotationDefaults {
		expected := def
		if v, ok := w[k]; ok && v != nil {
			expected = v
		}
		if k == "color" && expected == "" {
			expected = "default"
		}
		if !reflect.DeepEqual(expected, gotMap[k]) {
			return false
		}
	}
	return true
}

// richTextToRequestJSON renders rich text in the shape a request takes,
// without plain_text, href, or annotations at their defaults. This is what
// an import stores and what drift shows, so it reads like something a user
// would write in jsonencode.
func richTextToRequestJSON(rt []notionapi.RichText) (string, error) {
	out := make([]map[string]interface{}, 0, len(rt))
	for _, r := range rt {
		item := map[string]interface{}{"type": richTextType(r)}
		switch {
		case r.Text != nil:
			text := map[string]interface{}{"content": r.Text.Content}
			if r.Text.Link != nil && r.Text.Link.Url != "" {
				text["link"] = map[string]interface{}{"url": r.Text.Link.Url}
			}
			item["text"] = text
		case r.Mention != nil:
			item["mention"] = mentionRequestShape(r.Mention)
		case r.Equation != nil:
			item["equation"] = map[string]interface{}{"expression": r.Equation.Expression}
		}
		ann := map[string]interface{}{}
		for k, v := range annotationsToMap(r.Annotations) {
			if !reflect.DeepEqual(v, annotationDefaults[k]) {
				ann[k] = v
			}
		}
		if len(ann) > 0 {
			item["annotations"] = ann
		}
		out = append(out, item)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// mentionRequestShape trims user, page, and database mentions to the ID a
// request needs. Other mentions are kept as the SDK models them.
func mentionRequestShape(m *notionapi.Mention) interface{} {
	switch {
	case m.User != nil:
		return map[string]interface{}{"type": "user", "user": map[string]interface{}{"id": string(m.User.ID)}}
	case m.Page != nil:
		return map[string]interface{}{"type": "page", "page": map[string]interface{}{"id": string(m.Page.ID)}}
	case m.Database != nil:
		return map[string]interface{}{"type": "database", "database": map[string]interface{}{"id": string(m.Database.ID)}}
	}
	// Round-trip through a map so keys sort like the rest of the output.
	if g, err := toGenericJSON(m); err == nil {
		return g
	}
	return m
}

func toGenericJSON(v interface{}) (interface{}, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out interface{}
	err = json.Unmarshal(b, &out)
	return out, err
}

// jsonSubset reports whether every value set in want is also in got. Keys
// only got has are ignored. Strings also match when they're the same Notion
// ID with and without hyphens, or the same date in two formats.
func jsonSubset(want, got interface{}) bool {
	switch w := want.(type) {
	case map[string]interface{}:
		g, ok := got.(map[string]interface{})
		if !ok {
			return false
		}
		for k, wv := range w {
			gv, present := g[k]
			if wv == nil {
				if present && gv != nil {
					return false
				}
				continue
			}
			if !present || !jsonSubset(wv, gv) {
				return false
			}
		}
		return true
	case []interface{}:
		g, ok := got.([]interface{})
		if !ok || len(g) != len(w) {
			return false
		}
		for i := range w {
			if !jsonSubset(w[i], g[i]) {
				return false
			}
		}
		return true
	case string:
		g, ok := got.(string)
		return ok && sameJSONString(w, g)
	}
	return reflect.DeepEqual(want, got)
}

func sameJSONString(a, b string) bool {
	if a == b {
		return true
	}
	if na, nb := normalizeID(a), normalizeID(b); len(na) == 32 && na == nb {
		return true
	}
	ta, okA := parseNotionDate(a)
	tb, okB := parseNotionDate(b)
	return okA && okB && ta.Equal(tb)
}

func parseNotionDate(s string) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, true
	}
	return time.Time{}, false
}
