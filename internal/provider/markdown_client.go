package provider

import (
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

// Creates and inserts add content each time they run and have no
// idempotency key, so those methods pass withNoReplay. A replace is safe
// to resend and keeps the normal retries.
func (mc *markdownClient) doRequest(ctx context.Context, method, url string, body interface{}) ([]byte, error) {
	var reqBody []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = b
	}

	// Shared helper: 429 retry with Retry-After, plus 5xx retry in
	// retryTransport. Previously this used http.DefaultClient and surfaced
	// every rate limit straight to the user.
	resp, err := doNotionRequestWithVersion(ctx, method, url, mc.token, markdownAPIVersion, reqBody)
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

	respBody, err := mc.doRequest(withNoReplay(ctx), http.MethodPost, "https://api.notion.com/v1/pages", body)
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
// position is the optional create-time placement (2026-01-15), nil to omit.
func (mc *markdownClient) CreatePageWithMarkdownAndTitle(ctx context.Context, parentPageID, title, markdown string, position map[string]interface{}) (string, string, error) {
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
	if position != nil {
		body["position"] = position
	}

	respBody, err := mc.doRequest(withNoReplay(ctx), http.MethodPost, "https://api.notion.com/v1/pages", body)
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

	respBody, err := mc.doRequest(withNoReplay(ctx), http.MethodPost, "https://api.notion.com/v1/pages", body)
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
	return mc.GetPageMarkdownWithTranscript(ctx, pageID, false)
}

// GetPageMarkdownWithTranscript retrieves a page's markdown, optionally with
// full AI meeting note transcripts (include_transcript, 2026-02-26). Without
// it, Notion renders a placeholder linking to each meeting note.
func (mc *markdownClient) GetPageMarkdownWithTranscript(ctx context.Context, pageID string, includeTranscript bool) (*PageMarkdownResponse, error) {
	url := pageMarkdownURL(pageID, includeTranscript)

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

func pageMarkdownURL(pageID string, includeTranscript bool) string {
	u := fmt.Sprintf("%s/pages/%s/markdown", notionAPIBaseURL, pageID)
	if includeTranscript {
		u += "?include_transcript=true"
	}
	return u
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
	body := insertContentBody(markdown, position)

	respBody, err := mc.doRequest(withNoReplay(ctx), http.MethodPatch, url, body)
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

// insertContentBody builds the insert_content request. Notion expects the
// text in `content` and the position as an object such as {"type": "end"}.
func insertContentBody(markdown, position string) map[string]interface{} {
	return map[string]interface{}{
		"type": "insert_content",
		"insert_content": map[string]interface{}{
			"content":  markdown,
			"position": map[string]string{"type": position},
		},
	}
}
