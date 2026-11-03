package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/jomei/notionapi"
)

const markdownAPIVersion = "2026-03-11"

type PageMarkdownResponse struct {
	Object          string   `json:"object"`
	ID              string   `json:"id"`
	Markdown        string   `json:"markdown"`
	Truncated       bool     `json:"truncated"`
	UnknownBlockIDs []string `json:"unknown_block_ids"`
}

type markdownClient struct {
	token string
}

func newMarkdownClient(client *notionapi.Client) *markdownClient {
	return &markdownClient{token: client.Token.String()}
}

func (mc *markdownClient) doRequest(ctx context.Context, method, url string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", mc.token))
	req.Header.Set("Notion-Version", markdownAPIVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Notion API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// CreatePageWithMarkdown creates a new page using the markdown parameter.
func (mc *markdownClient) CreatePageWithMarkdown(ctx context.Context, parentPageID, markdown string) (*PageMarkdownResponse, error) {
	body := map[string]interface{}{
		"parent":   map[string]string{"page_id": parentPageID},
		"markdown": markdown,
	}

	respBody, err := mc.doRequest(ctx, http.MethodPost, "https://api.notion.com/v1/pages", body)
	if err != nil {
		return nil, err
	}

	// The create endpoint returns a Page object, not a PageMarkdownResponse.
	// We need to parse the page ID and then fetch markdown separately.
	var page struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(respBody, &page); err != nil {
		return nil, fmt.Errorf("failed to parse page response: %w", err)
	}

	return &PageMarkdownResponse{
		Object:   "page_markdown",
		ID:       page.ID,
		Markdown: markdown,
	}, nil
}

// CreatePageWithMarkdownAndTitle creates a page with both a title property and markdown content.
func (mc *markdownClient) CreatePageWithMarkdownAndTitle(ctx context.Context, parentPageID, title, markdown string) (string, string, error) {
	body := map[string]interface{}{
		"parent":   map[string]string{"page_id": parentPageID},
		"markdown": markdown,
		"properties": map[string]interface{}{
			"title": map[string]interface{}{
				"type": "title",
				"title": []map[string]interface{}{
					{"type": "text", "text": map[string]string{"content": title}},
				},
			},
		},
	}

	respBody, err := mc.doRequest(ctx, http.MethodPost, "https://api.notion.com/v1/pages", body)
	if err != nil {
		return "", "", err
	}

	var page struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(respBody, &page); err != nil {
		return "", "", fmt.Errorf("failed to parse page response: %w", err)
	}

	return page.ID, page.URL, nil
}

// CreateDatabaseEntryWithMarkdown creates a database entry with markdown content and properties.
func (mc *markdownClient) CreateDatabaseEntryWithMarkdown(ctx context.Context, databaseID, markdown string, properties map[string]interface{}) (string, string, error) {
	body := map[string]interface{}{
		"parent":     map[string]string{"database_id": databaseID},
		"markdown":   markdown,
		"properties": properties,
	}

	respBody, err := mc.doRequest(ctx, http.MethodPost, "https://api.notion.com/v1/pages", body)
	if err != nil {
		return "", "", err
	}

	var page struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(respBody, &page); err != nil {
		return "", "", fmt.Errorf("failed to parse page response: %w", err)
	}

	return page.ID, page.URL, nil
}

// GetPageMarkdown retrieves a page's content as markdown.
func (mc *markdownClient) GetPageMarkdown(ctx context.Context, pageID string) (*PageMarkdownResponse, error) {
	url := fmt.Sprintf("https://api.notion.com/v1/pages/%s/markdown", pageID)

	respBody, err := mc.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var result PageMarkdownResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse markdown response: %w", err)
	}

	if result.Truncated {
		return nil, fmt.Errorf("page content is truncated (exceeds ~20,000 blocks); this provider does not support truncated pages")
	}

	return &result, nil
}

// ReplacePageMarkdown replaces all content in a page with new markdown.
func (mc *markdownClient) ReplacePageMarkdown(ctx context.Context, pageID, markdown string) (*PageMarkdownResponse, error) {
	url := fmt.Sprintf("https://api.notion.com/v1/pages/%s/markdown", pageID)

	body := map[string]interface{}{
		"type": "replace_content",
		"replace_content": map[string]interface{}{
			"new_str":                markdown,
			"allow_deleting_content": true,
		},
	}

	respBody, err := mc.doRequest(ctx, http.MethodPatch, url, body)
	if err != nil {
		return nil, err
	}

	var result PageMarkdownResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse markdown update response: %w", err)
	}

	if result.Truncated {
		return nil, fmt.Errorf("page content is truncated after update; this provider does not support truncated pages")
	}

	return &result, nil
}

// InsertPageMarkdown prepends or appends markdown to a page without rewriting
// the existing content. position must be "start" or "end". Backs the
// 2026-05-15 insert_content.position addition to PATCH /v1/pages/{id}/markdown.
func (mc *markdownClient) InsertPageMarkdown(ctx context.Context, pageID, markdown, position string) (*PageMarkdownResponse, error) {
	if position != "start" && position != "end" {
		return nil, fmt.Errorf("InsertPageMarkdown: position must be \"start\" or \"end\", got %q", position)
	}

	url := fmt.Sprintf("https://api.notion.com/v1/pages/%s/markdown", pageID)
	body := map[string]interface{}{
		"type": "insert_content",
		"insert_content": map[string]interface{}{
			"markdown": markdown,
			"position": position,
		},
	}

	respBody, err := mc.doRequest(ctx, http.MethodPatch, url, body)
	if err != nil {
		return nil, err
	}

	var result PageMarkdownResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse markdown insert response: %w", err)
	}

	if result.Truncated {
		return nil, fmt.Errorf("page content is truncated after insert; this provider does not support truncated pages")
	}

	return &result, nil
}
