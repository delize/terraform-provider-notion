package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// The jomei/notionapi SDK models status options as {id, name, color} and has
// no `group` field (2026-06-22) or `default_option` (2026-10-22). This file
// writes and reads status property schemas over direct HTTP instead.
//
// It stays on Notion-Version 2022-06-28 on purpose. That is the version the
// rest of the database and property resources use, where the schema lives on
// the database object. The Update a database reference documents `group`
// for that legacy endpoint.

const legacySchemaAPIVersion = "2022-06-28"

// statusGroupNames are the group names Notion accepts on a status option.
var statusGroupNames = []string{"To-do", "In progress", "Complete"}

type statusOptionRequest struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
	Group string `json:"group,omitempty"`
}

// statusPropertySchema is what the resource needs back from Notion.
type statusPropertySchema struct {
	ID            string
	Name          string
	IsStatus      bool              // false when a same-named property has another type
	Options       map[string]string // option name -> color
	Groups        map[string]string // option name -> group name
	DefaultOption *string
}

// groupFor looks up an option's group without regard to case, matching how
// Notion matches option names.
func (s *statusPropertySchema) groupFor(option string) (string, bool) {
	if g, ok := s.Groups[option]; ok {
		return g, true
	}
	for name, g := range s.Groups {
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(option)) {
			return g, true
		}
	}
	return "", false
}

type rawSchemaDatabase struct {
	Properties map[string]rawSchemaProperty `json:"properties"`
}

type rawSchemaProperty struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Type   string           `json:"type"`
	Status *rawStatusSchema `json:"status,omitempty"`
}

type rawStatusSchema struct {
	Options []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	} `json:"options"`
	Groups []struct {
		ID        string   `json:"id"`
		Name      string   `json:"name"`
		OptionIDs []string `json:"option_ids"`
	} `json:"groups"`
	DefaultOption *string `json:"default_option"`
}

// upsertStatusProperty creates or replaces a status property's option set
// via PATCH /v1/databases/{id}. Options are the complete list to keep, as
// Notion removes any that are omitted. An option without a group keeps its
// current group, or lands in To-do when new.
func upsertStatusProperty(ctx context.Context, token, databaseID, name string, options []statusOptionRequest) (*statusPropertySchema, error) {
	body, err := json.Marshal(map[string]interface{}{
		"properties": map[string]interface{}{
			name: map[string]interface{}{
				"type":   "status",
				"status": map[string]interface{}{"options": options},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/databases/%s", notionAPIBaseURL, databaseID)
	respBody, err := doSchemaRequest(ctx, http.MethodPatch, url, token, body)
	if err != nil {
		return nil, err
	}
	schema, found, err := parseStatusProperty(respBody, "", name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("status property %q missing from update response", name)
	}
	return schema, nil
}

// readStatusProperty fetches a database and returns the status property
// matching propertyID, or name when the ID is empty or not found.
func readStatusProperty(ctx context.Context, token, databaseID, propertyID, name string) (*statusPropertySchema, bool, error) {
	url := fmt.Sprintf("%s/databases/%s", notionAPIBaseURL, databaseID)
	respBody, err := doSchemaRequest(ctx, http.MethodGet, url, token, nil)
	if err != nil {
		return nil, false, err
	}
	return parseStatusProperty(respBody, propertyID, name)
}

func doSchemaRequest(ctx context.Context, method, url, token string, body []byte) ([]byte, error) {
	resp, err := doNotionRequestWithVersion(ctx, method, url, token, legacySchemaAPIVersion, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("notion API %d on %s %s: %s", resp.StatusCode, method, url, string(respBody))
	}
	return respBody, nil
}

// parseStatusProperty finds a status property in a database response. ID
// wins over name so a property renamed in Notion is still found.
func parseStatusProperty(dbJSON []byte, propertyID, name string) (*statusPropertySchema, bool, error) {
	var db rawSchemaDatabase
	if err := json.Unmarshal(dbJSON, &db); err != nil {
		return nil, false, fmt.Errorf("failed to parse database response: %w", err)
	}

	var (
		matchName string
		match     *rawSchemaProperty
	)
	if propertyID != "" {
		for n, p := range db.Properties {
			if p.ID == propertyID {
				p := p
				matchName, match = n, &p
				break
			}
		}
	}
	if match == nil {
		if p, ok := db.Properties[name]; ok {
			matchName, match = name, &p
		}
	}
	if match == nil {
		return nil, false, nil
	}

	out := &statusPropertySchema{
		ID:      match.ID,
		Name:    matchName,
		Options: map[string]string{},
		Groups:  map[string]string{},
	}
	if match.Type != "status" || match.Status == nil {
		return out, true, nil
	}
	out.IsStatus = true

	idToName := make(map[string]string, len(match.Status.Options))
	for _, o := range match.Status.Options {
		out.Options[o.Name] = o.Color
		idToName[o.ID] = o.Name
	}
	for _, g := range match.Status.Groups {
		for _, id := range g.OptionIDs {
			if n, ok := idToName[id]; ok {
				out.Groups[n] = g.Name
			}
		}
	}
	out.DefaultOption = match.Status.DefaultOption
	return out, true, nil
}
