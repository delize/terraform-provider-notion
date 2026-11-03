package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// Entry property maps whose values are lists: multi_select option names,
// related page IDs, and people (user IDs). They follow the same rules as
// the scalar maps in resource_database_entry.go. Each key is written in
// full on create and update, read back only when it's in state, and cleared
// with an empty array when removed from config.

const (
	// maxEntryListItems is Notion's per-request limit for relation, people,
	// and multi_select arrays (request limits page, "100 related pages",
	// "100 users", "100 options").
	maxEntryListItems = 100
	// pageResponseListLimit is how many relation or people references a
	// page object carries. Notion documents that a relation with more than
	// 25 references comes back with has_more and that more than 25 people
	// may be omitted. The SDK drops has_more, so a list of this length or
	// longer is treated as possibly truncated and read in full from the
	// page property item endpoint.
	pageResponseListLimit = 25
)

// entryListElemType is the element type of every list-valued entry map.
var entryListElemType = types.ListType{ElemType: types.StringType}

// listMapValues decodes a map of string lists from a plan. It returns nil
// for a null or unknown map.
func listMapValues(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string][]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	var vals map[string][]string
	diags.Append(m.ElementsAs(ctx, &vals, false)...)
	return vals
}

// buildEntryListProperties adds the list-valued maps to props. Arrays are
// always non-nil so an empty list is sent as [] and clears the value.
func buildEntryListProperties(ctx context.Context, plan *DatabaseEntryResourceModel, diags *diag.Diagnostics, props notionapi.Properties) {
	for name, vals := range listMapValues(ctx, plan.MultiSelectProperties, diags) {
		props[name] = multiSelectValue(vals)
	}
	for name, vals := range listMapValues(ctx, plan.RelationProperties, diags) {
		props[name] = relationValue(vals)
	}
	for name, vals := range listMapValues(ctx, plan.PeopleProperties, diags) {
		props[name] = peopleValue(vals)
	}
}

func multiSelectValue(names []string) notionapi.MultiSelectProperty {
	opts := make([]notionapi.Option, 0, len(names))
	for _, n := range names {
		opts = append(opts, notionapi.Option{Name: n})
	}
	return notionapi.MultiSelectProperty{Type: notionapi.PropertyTypeMultiSelect, MultiSelect: opts}
}

func relationValue(ids []string) notionapi.RelationProperty {
	rels := make([]notionapi.Relation, 0, len(ids))
	for _, id := range ids {
		rels = append(rels, notionapi.Relation{ID: notionapi.PageID(id)})
	}
	return notionapi.RelationProperty{Type: notionapi.PropertyTypeRelation, Relation: rels}
}

func peopleValue(ids []string) notionapi.PeopleProperty {
	users := make([]notionapi.User, 0, len(ids))
	for _, id := range ids {
		users = append(users, notionapi.User{ID: notionapi.UserID(id)})
	}
	return notionapi.PeopleProperty{Type: notionapi.PropertyTypePeople, People: users}
}

// clearRemovedListProperties sends an empty array for list keys that were
// in state but are gone from the plan.
func clearRemovedListProperties(state, plan *DatabaseEntryResourceModel, props notionapi.Properties) {
	for _, name := range removedKeys(state.MultiSelectProperties, plan.MultiSelectProperties) {
		props[name] = multiSelectValue(nil)
	}
	for _, name := range removedKeys(state.RelationProperties, plan.RelationProperties) {
		props[name] = relationValue(nil)
	}
	for _, name := range removedKeys(state.PeopleProperties, plan.PeopleProperties) {
		props[name] = peopleValue(nil)
	}
}

func multiSelectNames(p notionapi.Property) ([]string, bool) {
	ms, ok := p.(*notionapi.MultiSelectProperty)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(ms.MultiSelect))
	for _, o := range ms.MultiSelect {
		out = append(out, o.Name)
	}
	return out, true
}

func relationIDs(p notionapi.Property) ([]string, bool) {
	rp, ok := p.(*notionapi.RelationProperty)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(rp.Relation))
	for _, r := range rp.Relation {
		out = append(out, string(r.ID))
	}
	return out, true
}

func peopleIDs(p notionapi.Property) ([]string, bool) {
	pp, ok := p.(*notionapi.PeopleProperty)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(pp.People))
	for _, u := range pp.People {
		out = append(out, string(u.ID))
	}
	return out, true
}

func identity(s string) string { return s }

// readEntryListProperties reads the list-valued maps back for the keys
// already in state, the same rule the scalar maps follow.
func readEntryListProperties(page *notionapi.Page, state *DatabaseEntryResourceModel, diags *diag.Diagnostics) {
	state.MultiSelectProperties = readListMap(page, state.MultiSelectProperties, multiSelectNames, identity, diags)
	state.RelationProperties = readListMap(page, state.RelationProperties, relationIDs, normalizeID, diags)
	state.PeopleProperties = readListMap(page, state.PeopleProperties, peopleIDs, normalizeID, diags)
}

func readListMap(page *notionapi.Page, prior types.Map, extract func(notionapi.Property) ([]string, bool), norm func(string) string, diags *diag.Diagnostics) types.Map {
	if prior.IsNull() || prior.IsUnknown() {
		return prior
	}
	vals := make(map[string]attr.Value)
	for name, v := range prior.Elements() {
		prop, ok := page.Properties[name]
		if !ok {
			continue
		}
		api, ok := extract(prop)
		if !ok {
			continue
		}
		merged := reconcileListValues(stringsOfList(v), api, norm)
		elems := make([]attr.Value, 0, len(merged))
		for _, s := range merged {
			elems = append(elems, types.StringValue(s))
		}
		l, d := types.ListValue(types.StringType, elems)
		diags.Append(d...)
		vals[name] = l
	}
	m, d := types.MapValue(entryListElemType, vals)
	diags.Append(d...)
	return m
}

// stringsOfList returns the known string elements of a list value.
func stringsOfList(v attr.Value) []string {
	l, ok := v.(types.List)
	if !ok || l.IsNull() || l.IsUnknown() {
		return nil
	}
	out := make([]string, 0, len(l.Elements()))
	for _, e := range l.Elements() {
		if s, ok := e.(types.String); ok && !s.IsNull() && !s.IsUnknown() {
			out = append(out, s.ValueString())
		}
	}
	return out
}

// reconcileListValues decides what to store for one list. When Notion
// holds the same members as state (compared after norm), state is kept as
// is, so neither order nor the configured ID form causes a diff. Otherwise
// Notion's list is returned in Notion's order, with each member in its
// state form when state had it and normalized when it didn't.
func reconcileListValues(prior, api []string, norm func(string) string) []string {
	if sameMembers(prior, api, norm) {
		return append([]string{}, prior...)
	}
	form := make(map[string]string, len(prior))
	for _, p := range prior {
		form[norm(p)] = p
	}
	out := make([]string, 0, len(api))
	for _, a := range api {
		if f, ok := form[norm(a)]; ok {
			out = append(out, f)
		} else {
			out = append(out, norm(a))
		}
	}
	return out
}

func sameMembers(a, b []string, norm func(string) string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, s := range a {
		counts[norm(s)]++
	}
	for _, s := range b {
		k := norm(s)
		if counts[k] == 0 {
			return false
		}
		counts[k]--
	}
	return true
}

// needsListExpansion reports whether a configured relation or people value
// is long enough that the page response may have cut it off.
func needsListExpansion(page *notionapi.Page, state *DatabaseEntryResourceModel) bool {
	if !state.RelationProperties.IsNull() && !state.RelationProperties.IsUnknown() {
		for name := range state.RelationProperties.Elements() {
			if rp, ok := page.Properties[name].(*notionapi.RelationProperty); ok && len(rp.Relation) >= pageResponseListLimit {
				return true
			}
		}
	}
	if !state.PeopleProperties.IsNull() && !state.PeopleProperties.IsUnknown() {
		for name := range state.PeopleProperties.Elements() {
			if pp, ok := page.Properties[name].(*notionapi.PeopleProperty); ok && len(pp.People) >= pageResponseListLimit {
				return true
			}
		}
	}
	return false
}

// expandTruncatedLists replaces relation and people values that may have
// been cut off in the page response with the full list from
// GET /v1/pages/{page_id}/properties/{property_id}. Only keys in state are
// checked, so this costs nothing for entries that don't manage them.
func expandTruncatedLists(ctx context.Context, token string, page *notionapi.Page, state *DatabaseEntryResourceModel) error {
	pageID := normalizeID(string(page.ID))
	if !state.RelationProperties.IsNull() && !state.RelationProperties.IsUnknown() {
		for name := range state.RelationProperties.Elements() {
			rp, ok := page.Properties[name].(*notionapi.RelationProperty)
			if !ok || len(rp.Relation) < pageResponseListLimit {
				continue
			}
			ids, err := listPropertyItemIDs(ctx, token, pageID, string(rp.ID), "relation")
			if err != nil {
				return fmt.Errorf("reading all relations of %q: %w", name, err)
			}
			full := relationValue(ids)
			rp.Relation = full.Relation
		}
	}
	if !state.PeopleProperties.IsNull() && !state.PeopleProperties.IsUnknown() {
		for name := range state.PeopleProperties.Elements() {
			pp, ok := page.Properties[name].(*notionapi.PeopleProperty)
			if !ok || len(pp.People) < pageResponseListLimit {
				continue
			}
			ids, err := listPropertyItemIDs(ctx, token, pageID, string(pp.ID), "people")
			if err != nil {
				return fmt.Errorf("reading all people of %q: %w", name, err)
			}
			full := peopleValue(ids)
			pp.People = full.People
		}
	}
	return nil
}

// listPropertyItemIDs pages through the property item endpoint for a
// relation or people property and returns the referenced page or user IDs.
// Property IDs in page responses are already URL-encoded (for example
// "%3AbC"), so the ID goes into the path as Notion returned it.
func listPropertyItemIDs(ctx context.Context, token, pageID, propertyID, kind string) ([]string, error) {
	var (
		ids    []string
		cursor string
	)
	for {
		q := url.Values{"page_size": {"100"}}
		if cursor != "" {
			q.Set("start_cursor", cursor)
		}
		u := fmt.Sprintf("%s/pages/%s/properties/%s?%s", notionAPIBaseURL, pageID, propertyID, q.Encode())
		body, err := doRawBlockRequest(ctx, http.MethodGet, u, token, nil)
		if err != nil {
			return nil, err
		}
		var list struct {
			Results []struct {
				Type     string `json:"type"`
				Relation *struct {
					ID string `json:"id"`
				} `json:"relation"`
				People *struct {
					ID string `json:"id"`
				} `json:"people"`
			} `json:"results"`
			HasMore    bool    `json:"has_more"`
			NextCursor *string `json:"next_cursor"`
		}
		if err := json.Unmarshal(body, &list); err != nil {
			return nil, fmt.Errorf("failed to parse property items: %w", err)
		}
		for _, item := range list.Results {
			switch {
			case kind == "relation" && item.Relation != nil:
				ids = append(ids, item.Relation.ID)
			case kind == "people" && item.People != nil:
				ids = append(ids, item.People.ID)
			}
		}
		if !list.HasMore || list.NextCursor == nil || *list.NextCursor == "" {
			return ids, nil
		}
		cursor = *list.NextCursor
	}
}

// validateEntryListMaps checks the list maps at plan time. Unknown values
// are skipped and checked again once they're known.
func validateEntryListMaps(cfg *DatabaseEntryResourceModel, diags *diag.Diagnostics) {
	check := func(attrName string, m types.Map, norm func(string) string, noCommas bool) {
		if m.IsNull() || m.IsUnknown() {
			return
		}
		for key, v := range m.Elements() {
			l, ok := v.(types.List)
			if !ok || l.IsNull() || l.IsUnknown() {
				continue
			}
			p := path.Root(attrName).AtMapKey(key)
			if n := len(l.Elements()); n > maxEntryListItems {
				diags.AddAttributeError(p, "Too many values",
					fmt.Sprintf("Notion accepts at most %d values for one property in a request, got %d.", maxEntryListItems, n))
			}
			seen := map[string]bool{}
			for _, e := range l.Elements() {
				s, ok := e.(types.String)
				if !ok || s.IsNull() || s.IsUnknown() {
					continue
				}
				val := s.ValueString()
				if noCommas && strings.Contains(val, ",") {
					diags.AddAttributeError(p, "Invalid option name",
						fmt.Sprintf("Multi-select option names can't contain commas, got %q.", val))
				}
				if seen[norm(val)] {
					diags.AddAttributeError(p, "Duplicate value",
						fmt.Sprintf("%q is listed more than once. Notion stores each value once, so the list would never match.", val))
				}
				seen[norm(val)] = true
			}
		}
	}
	check("multi_select_properties", cfg.MultiSelectProperties, identity, true)
	check("relation_properties", cfg.RelationProperties, normalizeID, false)
	check("people_properties", cfg.PeopleProperties, normalizeID, false)
}

// ValidateConfig checks the list-valued property maps.
func (r *DatabaseEntryResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg DatabaseEntryResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateEntryListMaps(&cfg, &resp.Diagnostics)
}
