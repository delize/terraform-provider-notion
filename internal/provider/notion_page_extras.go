package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/jomei/notionapi"
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
func createPageWithTemplate(ctx context.Context, token, parentPageID, title, templateID, timezone string, position map[string]interface{}) (string, string, error) {
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
	if position != nil {
		body["position"] = position
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

// createPagePlain POSTs /v1/pages with a title, an optional emoji icon, and a
// create-time position (2026-01-15). The SDK's PageCreateRequest has no
// position field, so notion_page uses this instead of the SDK only when a
// position is configured.
func createPagePlain(ctx context.Context, token, parentPageID, title string, icon, position map[string]interface{}) (string, string, error) {
	body := map[string]interface{}{
		"parent": map[string]string{"type": "page_id", "page_id": parentPageID},
		"properties": map[string]interface{}{
			"title": map[string]interface{}{
				"type": "title",
				"title": []map[string]interface{}{
					{"type": "text", "text": map[string]string{"content": title}},
				},
			},
		},
	}
	if icon != nil {
		body["icon"] = icon
	}
	if position != nil {
		body["position"] = position
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
		return "", "", fmt.Errorf("notion API %d creating page: %s", resp.StatusCode, string(respBody))
	}

	var page createPageResp
	if err := json.Unmarshal(respBody, &page); err != nil {
		return "", "", fmt.Errorf("failed to parse create-page response: %w", err)
	}
	return page.ID, page.URL, nil
}

// pagePositionRequest builds the create-time position object, or nil when
// neither attribute is set.
func pagePositionRequest(position, afterBlockID string) map[string]interface{} {
	switch {
	case afterBlockID != "":
		return map[string]interface{}{"type": "after_block", "after_block": map[string]string{"id": afterBlockID}}
	case position != "":
		return map[string]interface{}{"type": position}
	}
	return nil
}

// pageObject is the slice of a page object notion_page reads. It is decoded
// from a Notion-Version 2026-03-11 response, which carries native icons,
// custom emoji, and is_locked that the SDK's 2022-06-28 Page type drops.
type pageObject struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	InTrash  bool   `json:"in_trash"`
	IsLocked bool   `json:"is_locked"`
	Parent   struct {
		Type   string `json:"type"`
		PageID string `json:"page_id"`
	} `json:"parent"`
	Properties map[string]json.RawMessage `json:"properties"`
	Icon       json.RawMessage            `json:"icon"`
}

// title returns the page title with links rendered the same way the SDK
// path did, and false when the page has no title property.
func (p pageObject) title() (string, bool) {
	raw, ok := p.Properties["title"]
	if !ok {
		return "", false
	}
	var tp struct {
		Title []notionapi.RichText `json:"title"`
	}
	if json.Unmarshal(raw, &tp) != nil {
		return "", false
	}
	return richTextToPlain(tp.Title), true
}

func decodePageObject(body []byte) (*pageObject, error) {
	var p pageObject
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("failed to parse page: %w", err)
	}
	return &p, nil
}

// getPageRaw fetches a page on Notion-Version 2026-03-11.
func getPageRaw(ctx context.Context, token, pageID string) (*pageObject, error) {
	body, err := containerRequest(ctx, token, http.MethodGet, "pages", pageID, nil)
	if err != nil {
		return nil, err
	}
	return decodePageObject(body)
}

// updatePageRaw PATCHes /v1/pages/{id} on Notion-Version 2026-03-11. body
// holds any of properties, icon (an object, or nil to clear), and
// is_locked.
func updatePageRaw(ctx context.Context, token, pageID string, body map[string]interface{}) (*pageObject, error) {
	respBody, err := containerRequest(ctx, token, http.MethodPatch, "pages", pageID, body)
	if err != nil {
		return nil, err
	}
	return decodePageObject(respBody)
}
