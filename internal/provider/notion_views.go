package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Notion shipped 8 view endpoints in the 2026-03-19 changelog under /v1/views.
// The jomei/notionapi SDK doesn't model them, so this file shims them over
// doNotionRequest. We keep the request/response shapes loose (json.RawMessage)
// for filter / sorts / configuration so the resource doesn't have to model
// every nested field — users pass JSON strings via jsonencode/jsondecode.

// viewObject is the slim subset of the response we need on read.
type viewObject struct {
	Object         string          `json:"object"`
	ID             string          `json:"id"`
	DataSourceID   string          `json:"data_source_id"`
	Parent         struct {
		Type       string `json:"type"`
		DatabaseID string `json:"database_id"`
	} `json:"parent"`
	Name           string          `json:"name"`
	Type           string          `json:"type"`
	URL            string          `json:"url"`
	CreatedTime    string          `json:"created_time"`
	LastEditedTime string          `json:"last_edited_time"`
	Filter         json.RawMessage `json:"filter,omitempty"`
	Sorts          json.RawMessage `json:"sorts,omitempty"`
	QuickFilters   json.RawMessage `json:"quick_filters,omitempty"`
	Configuration  json.RawMessage `json:"configuration,omitempty"`
}

// viewCreate accepts the most common parent options (database_id and view_id);
// create_database is intentionally not exposed — creating a brand new database
// as a side effect of creating a view would be surprising for a TF resource.
type viewCreate struct {
	DatabaseID    string          `json:"database_id,omitempty"`
	ViewID        string          `json:"view_id,omitempty"`
	DataSourceID  string          `json:"data_source_id"`
	Name          string          `json:"name"`
	Type          string          `json:"type"`
	Filter        json.RawMessage `json:"filter,omitempty"`
	Sorts         json.RawMessage `json:"sorts,omitempty"`
	QuickFilters  json.RawMessage `json:"quick_filters,omitempty"`
	Configuration json.RawMessage `json:"configuration,omitempty"`
}

type viewUpdate struct {
	Name          *string         `json:"name,omitempty"`
	Filter        json.RawMessage `json:"filter,omitempty"`
	Sorts         json.RawMessage `json:"sorts,omitempty"`
	QuickFilters  json.RawMessage `json:"quick_filters,omitempty"`
	Configuration json.RawMessage `json:"configuration,omitempty"`
}

func viewsBaseURL() string {
	return notionAPIBaseURL + "/views"
}

func createView(ctx context.Context, token string, payload viewCreate) (*viewObject, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	resp, err := doNotionRequest(ctx, http.MethodPost, viewsBaseURL(), token, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return decodeViewResponse(resp, "create")
}

func getView(ctx context.Context, token, viewID string) (*viewObject, error) {
	resp, err := doNotionRequest(ctx, http.MethodGet, viewsBaseURL()+"/"+viewID, token, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return decodeViewResponse(resp, "get")
}

func updateView(ctx context.Context, token, viewID string, payload viewUpdate) (*viewObject, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	resp, err := doNotionRequest(ctx, http.MethodPatch, viewsBaseURL()+"/"+viewID, token, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return decodeViewResponse(resp, "update")
}

func deleteView(ctx context.Context, token, viewID string) error {
	resp, err := doNotionRequest(ctx, http.MethodDelete, viewsBaseURL()+"/"+viewID, token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("notion API %d deleting view %s: %s", resp.StatusCode, viewID, string(respBody))
	}
	return nil
}

// queryView runs a view query. Without queryID it creates one with
// POST /v1/views/{id}/queries, which returns the first page and a query ID.
// With queryID it fetches a later page of that cached query with GET
// /v1/views/{id}/queries/{query_id}. Notion keeps a query for about 15
// minutes. The response is returned verbatim.
func queryView(ctx context.Context, token, viewID, queryID, startCursor string, pageSize int64) ([]byte, error) {
	method, u, body := http.MethodPost, viewsBaseURL()+"/"+viewID+"/queries", []byte(nil)
	if queryID == "" {
		req := map[string]interface{}{}
		if pageSize > 0 {
			req["page_size"] = pageSize
		}
		b, err := json.Marshal(req)
		if err != nil {
			return nil, err
		}
		body = b
	} else {
		q := url.Values{}
		if startCursor != "" {
			q.Set("start_cursor", startCursor)
		}
		if pageSize > 0 {
			q.Set("page_size", fmt.Sprint(pageSize))
		}
		method, u = http.MethodGet, viewsBaseURL()+"/"+viewID+"/queries/"+queryID
		if len(q) > 0 {
			u += "?" + q.Encode()
		}
	}
	resp, err := doNotionRequest(ctx, method, u, token, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("notion API %d querying view %s: %s", resp.StatusCode, viewID, string(respBody))
	}
	return respBody, nil
}

func decodeViewResponse(resp *http.Response, op string) (*viewObject, error) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("notion API %d on %s view: %s", resp.StatusCode, op, string(respBody))
	}
	var v viewObject
	if err := json.Unmarshal(respBody, &v); err != nil {
		return nil, fmt.Errorf("failed to parse %s-view response: %w", op, err)
	}
	return &v, nil
}
