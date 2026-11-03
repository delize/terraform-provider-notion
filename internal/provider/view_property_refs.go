package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
)

// viewPropertyRefs maps the property names and IDs of a view's data source
// to one form, so a config that names a property and Notion's copy that
// uses its ID compare as the same value.
//
// Notion accepts a property name or ID anywhere a view refers to a property
// (filter and sorts "property", quick filter keys, configuration
// "property_id"), but answers with the ID. The title property's ID is
// always "title".
type viewPropertyRefs struct {
	canon map[string]string
}

// canonical returns the unescaped property ID for a name or ID the data
// source has, and ref unchanged otherwise. A nil receiver changes nothing.
func (p *viewPropertyRefs) canonical(ref string) string {
	if p == nil {
		return ref
	}
	if id, ok := p.canon[ref]; ok {
		return id
	}
	if un, err := url.PathUnescape(ref); err == nil {
		if id, ok := p.canon[un]; ok {
			return id
		}
	}
	return ref
}

// newViewPropertyRefs builds the lookup from a data source's properties,
// keyed by name as Notion returns them.
func newViewPropertyRefs(props map[string]struct {
	ID string `json:"id"`
}) *viewPropertyRefs {
	p := &viewPropertyRefs{canon: map[string]string{}}
	for name, prop := range props {
		id := prop.ID
		if un, err := url.PathUnescape(id); err == nil {
			id = un
		}
		p.canon[id] = id
		p.canon[prop.ID] = id
		// An ID wins over a name if a property is named like another's ID.
		if _, taken := p.canon[name]; !taken {
			p.canon[name] = id
		}
	}
	p.canon["title"] = "title"
	return p
}

// fetchViewPropertyRefs reads the data source schema with
// GET /v1/data_sources/{id}.
func fetchViewPropertyRefs(ctx context.Context, token, dataSourceID string) (*viewPropertyRefs, error) {
	body, err := doNotionGet(ctx, fmt.Sprintf("%s/data_sources/%s", notionAPIBaseURL, normalizeID(dataSourceID)), token, notionTrashAPIVersion)
	if err != nil {
		return nil, fmt.Errorf("reading data source %s to match property names to IDs: %w", dataSourceID, err)
	}
	var ds struct {
		Properties map[string]struct {
			ID string `json:"id"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(body, &ds); err != nil {
		return nil, fmt.Errorf("decoding data source %s: %w", dataSourceID, err)
	}
	return newViewPropertyRefs(ds.Properties), nil
}

// canonicalViewRefs returns v with every property reference in canonical
// form: the value of "property" and "property_id" keys at any depth, and
// with keysAreProps (quick_filters) the top-level keys too.
func canonicalViewRefs(v interface{}, refs *viewPropertyRefs, keysAreProps bool) interface{} {
	if refs == nil {
		return v
	}
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			if s, ok := val.(string); ok && (k == "property" || k == "property_id") {
				out[k] = refs.canonical(s)
				continue
			}
			key := k
			if keysAreProps {
				key = refs.canonical(k)
			}
			out[key] = canonicalViewRefs(val, refs, false)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, e := range t {
			out[i] = canonicalViewRefs(e, refs, false)
		}
		return out
	}
	return v
}

// viewJSONSubset reports whether every value want sets matches got, for a
// view's configuration. Notion fills configuration in: updates are a
// shallow merge that keeps omitted fields, and a table's properties list
// comes back with every property of the data source plus read-only fields
// such as property_name. So got may have keys want doesn't, and a list of
// objects with property_id may have more entries, matched by property_id in
// any order. A key want sets to null must be absent or null in got. Other
// lists must have the same length and match entry by entry.
func viewJSONSubset(want, got interface{}) bool {
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
			if !present || !viewJSONSubset(wv, gv) {
				return false
			}
		}
		return true
	case []interface{}:
		g, ok := got.([]interface{})
		if !ok {
			return false
		}
		if byID, ok := indexByPropertyID(g); ok && allHavePropertyID(w) {
			for _, we := range w {
				ge, found := byID[we.(map[string]interface{})["property_id"].(string)]
				if !found || !viewJSONSubset(we, ge) {
					return false
				}
			}
			return true
		}
		if len(w) != len(g) {
			return false
		}
		for i := range w {
			if !viewJSONSubset(w[i], g[i]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(want, got)
}

func allHavePropertyID(list []interface{}) bool {
	for _, e := range list {
		m, ok := e.(map[string]interface{})
		if !ok {
			return false
		}
		if _, ok := m["property_id"].(string); !ok {
			return false
		}
	}
	return true
}

func indexByPropertyID(list []interface{}) (map[string]interface{}, bool) {
	if !allHavePropertyID(list) {
		return nil, false
	}
	out := make(map[string]interface{}, len(list))
	for _, e := range list {
		m := e.(map[string]interface{})
		out[m["property_id"].(string)] = m
	}
	return out, true
}

// viewJSONHasContent reports whether any of a view's JSON attributes holds
// a value that refers to properties, so the data source schema is worth
// reading.
func viewJSONHasContent(vals ...string) bool {
	for _, s := range vals {
		if s != "" && !isEmptyJSONValue(s) {
			return true
		}
	}
	return false
}

// timestampSortError returns an error when sorts holds a timestamp sort.
// Notion accepts one on a view and then leaves it out of the view, so the
// apply would fail with an inconsistent result.
func timestampSortError(sorts string) error {
	if strings.TrimSpace(sorts) == "" {
		return nil
	}
	var list []map[string]interface{}
	if json.Unmarshal([]byte(sorts), &list) != nil {
		return nil
	}
	for i, s := range list {
		if _, ok := s["timestamp"]; ok {
			return fmt.Errorf("sorts[%d] sorts by timestamp. Notion accepts a timestamp sort on a view but doesn't keep it, so sort by a created time or last edited time property instead", i)
		}
	}
	return nil
}
