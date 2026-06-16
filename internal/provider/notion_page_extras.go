package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// The jomei/notionapi SDK doesn't know about the 2026-01-15 template parameter
// on Create page or the move page endpoint. This file shims both via direct
// HTTP using the shared doNotionRequest helper (notion_trash.go), keeping the
// rest of resource_page.go on the SDK path for the common case.

// createPageResp is the slim subset of the create-page response we need.
type createPageResp struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// createPageWithTemplate POSTs /v1/pages with a `template` field, returning
// (id, url). templateID may be empty to mean "default template" (the API
// distinguishes that from "no template" via the `type` field). The `children`
// param is intentionally not supported here — per Notion docs, children are
// disallowed when applying a template, and the page is returned blank initially
// with the template applied asynchronously.
func createPageWithTemplate(ctx context.Context, token, parentPageID, title, templateID, timezone string) (string, string, error) {
	tpl := map[string]interface{}{}
	if templateID == "" {
		tpl["type"] = "default"
	} else {
		tpl["type"] = "template_id"
		tpl["template_id"] = templateID
	}
	if timezone != "" {
		tpl["timezone"] = timezone
	}

	body := map[string]interface{}{
		"parent":   map[string]string{"type": "page_id", "page_id": parentPageID},
		"template": tpl,
		"properties": map[string]interface{}{
			"title": map[string]interface{}{
				"type": "title",
				"title": []map[string]interface{}{
					{"type": "text", "text": map[string]string{"content": title}},
				},
			},
		},
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return "", "", err
	}

	resp, err := doNotionRequest(ctx, http.MethodPost, notionAPIBaseURL+"/pages", token, bodyJSON)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("notion API %d creating page with template: %s", resp.StatusCode, string(respBody))
	}

	var page createPageResp
	if err := json.Unmarshal(respBody, &page); err != nil {
		return "", "", fmt.Errorf("failed to parse create-page response: %w", err)
	}
	return page.ID, page.URL, nil
}

// movePage POSTs /v1/pages/{id}/move with the new page_id parent. Backs the
// 2026-01-15 move page endpoint, used by resource_page Update when
// parent_page_id changes.
func movePage(ctx context.Context, token, pageID, newParentPageID string) error {
	body := map[string]interface{}{
		"parent": map[string]string{
			"type":    "page_id",
			"page_id": newParentPageID,
		},
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/pages/%s/move", notionAPIBaseURL, pageID)
	resp, err := doNotionRequest(ctx, http.MethodPost, url, token, bodyJSON)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("notion API %d moving page %s: %s", resp.StatusCode, pageID, string(respBody))
	}
	return nil
}
