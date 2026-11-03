package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

const markdownAPIVersion = "2026-03-11"

// markdownVersionWhenOmitted is the markdown syntax Notion uses on
// markdownAPIVersion when a request leaves markdown_version out. The next
// Notion-Version makes v2 the default, so this constant must change with
// markdownAPIVersion. Change detection compares against it, so an unset
// version and an explicit "v1" count as the same content.
const markdownVersionWhenOmitted = "v1"

// markdownWarning is one entry of the `warnings` array that a synchronous
// PATCH /v1/pages/{id}/markdown can return, for example when the v2 parser
// converted a deprecated tag or saved an unknown tag as text.
type markdownWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PageMarkdownResponse struct {
	Object          string            `json:"object"`
	ID              string            `json:"id"`
	Markdown        string            `json:"markdown"`
	Truncated       bool              `json:"truncated"`
	UnknownBlockIDs []string          `json:"unknown_block_ids"`
	Warnings        []markdownWarning `json:"warnings,omitempty"`
}

// markdownCreateResult is what a markdown page create returns. Notion's
// reference documents `warnings` only on the PATCH markdown endpoint, not on
// POST /v1/pages, so Warnings is filled only if Notion happens to send it.
type markdownCreateResult struct {
	ID       string            `json:"id"`
	URL      string            `json:"url"`
	Warnings []markdownWarning `json:"warnings,omitempty"`
}

type markdownClient struct {
	token string
}

func newMarkdownClient(client *notionapi.Client) *markdownClient {
	return &markdownClient{token: client.Token.String()}
}

// providerMarkdownVersions maps a token to its provider block's
// markdown_version, the same way providerHTTPClients carries
// silent_notifications to the direct-HTTP helpers.
var providerMarkdownVersions sync.Map

func registerMarkdownVersion(token, version string) {
	if version == "" {
		providerMarkdownVersions.Delete(token)
		return
	}
	providerMarkdownVersions.Store(token, version)
}

// providerMarkdownVersion returns the provider-level markdown_version for
// this client's token, or "" when the provider block leaves it unset.
func (mc *markdownClient) providerMarkdownVersion() string {
	if mc == nil {
		return ""
	}
	if v, ok := providerMarkdownVersions.Load(mc.token); ok {
		return v.(string)
	}
	return ""
}

// resolveMarkdownVersion picks the version to send. A resource-level value
// wins over the provider default. "" means omit the parameter.
func (mc *markdownClient) resolveMarkdownVersion(resourceVersion types.String) string {
	if !resourceVersion.IsNull() && !resourceVersion.IsUnknown() && resourceVersion.ValueString() != "" {
		return resourceVersion.ValueString()
	}
	return mc.providerMarkdownVersion()
}

// plannedMarkdownVersion is the markdown_version value a resource stores.
// A configured value is kept as is. Otherwise the provider default is
// recorded, but only when the resource manages markdown, so turning on a
// provider default doesn't show a diff on pages that have no markdown.
// With no default the value stays null, which matches state written before
// the attribute existed.
func plannedMarkdownVersion(configured, markdown types.String, providerDefault string) types.String {
	if !configured.IsNull() {
		return configured
	}
	if markdown.IsNull() || providerDefault == "" {
		return types.StringNull()
	}
	return types.StringValue(providerDefault)
}

// effectiveMarkdownVersion is the syntax a stored markdown_version stands
// for, treating unset as whatever Notion uses when the field is omitted.
func effectiveMarkdownVersion(v types.String) string {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return markdownVersionWhenOmitted
	}
	return v.ValueString()
}

// markdownNeedsReplace reports whether Update must resend the page body.
// It does only when the markdown text changed or the version it is written
// in changed. Resending on every update would replace edits made in the
// Notion UI whenever an unrelated attribute such as the title changed.
func markdownNeedsReplace(planMarkdown, stateMarkdown, planVersion, stateVersion types.String) bool {
	if planMarkdown.IsNull() || planMarkdown.IsUnknown() {
		return false
	}
	if !planMarkdown.Equal(stateMarkdown) {
		return true
	}
	return effectiveMarkdownVersion(planVersion) != effectiveMarkdownVersion(stateVersion)
}

// appendMarkdownWarnings turns Notion's markdown warnings into Terraform
// warning diagnostics.
func appendMarkdownWarnings(diags *diag.Diagnostics, pageID string, warnings []markdownWarning) {
	for _, w := range warnings {
		diags.AddWarning(
			fmt.Sprintf("Notion markdown warning (%s)", w.Code),
			fmt.Sprintf("Notion accepted the markdown for page %s but reported: %s", pageID, w.Message),
		)
	}
}

func withMarkdownVersion(body map[string]interface{}, version string) map[string]interface{} {
	if version != "" {
		body["markdown_version"] = version
	}
	return body
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

func (mc *markdownClient) createPage(ctx context.Context, body map[string]interface{}, version string) (*markdownCreateResult, error) {
	withMarkdownVersion(body, version)
	respBody, err := mc.doRequest(withNoReplay(ctx), http.MethodPost, notionAPIBaseURL+"/pages", body)
	if err != nil {
		return nil, err
	}

	var page markdownCreateResult
	if err := json.Unmarshal(respBody, &page); err != nil {
		return nil, fmt.Errorf("failed to parse page response: %w", err)
	}
	return &page, nil
}

// CreatePageWithMarkdownAndTitle creates a page with both a title property and markdown content.
// position is the optional create-time placement (2026-01-15), nil to omit.
// version is the markdown_version to send, "" to omit it.
func (mc *markdownClient) CreatePageWithMarkdownAndTitle(ctx context.Context, parentPageID, title, markdown, version string, position map[string]interface{}) (*markdownCreateResult, error) {
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
	return mc.createPage(ctx, body, version)
}

// CreateDatabaseEntryWithMarkdown creates a database entry with markdown content and properties.
func (mc *markdownClient) CreateDatabaseEntryWithMarkdown(ctx context.Context, databaseID, markdown, version string, properties map[string]interface{}) (*markdownCreateResult, error) {
	body := map[string]interface{}{
		"parent":     map[string]string{"database_id": databaseID},
		"markdown":   markdown,
		"properties": properties,
	}
	return mc.createPage(ctx, body, version)
}

// GetPageMarkdown retrieves a page's content as markdown.
func (mc *markdownClient) GetPageMarkdown(ctx context.Context, pageID, version string) (*PageMarkdownResponse, error) {
	return mc.GetPageMarkdownWithTranscript(ctx, pageID, false, version)
}

// GetPageMarkdownWithTranscript retrieves a page's markdown, optionally with
// full AI meeting note transcripts (include_transcript, 2026-02-26). Without
// it, Notion renders a placeholder linking to each meeting note.
func (mc *markdownClient) GetPageMarkdownWithTranscript(ctx context.Context, pageID string, includeTranscript bool, version string) (*PageMarkdownResponse, error) {
	respBody, err := mc.doRequest(ctx, http.MethodGet, pageMarkdownURL(pageID, includeTranscript, version), nil)
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

// pageMarkdownURL builds the GET URL. markdown_version is a query
// parameter on this endpoint, unlike the body parameter on writes.
func pageMarkdownURL(pageID string, includeTranscript bool, version string) string {
	u := fmt.Sprintf("%s/pages/%s/markdown", notionAPIBaseURL, pageID)
	q := url.Values{}
	if includeTranscript {
		q.Set("include_transcript", "true")
	}
	if version != "" {
		q.Set("markdown_version", version)
	}
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func pageMarkdownWriteURL(pageID string) string {
	return fmt.Sprintf("%s/pages/%s/markdown", notionAPIBaseURL, pageID)
}

// ReplacePageMarkdown replaces all content in a page with new markdown.
func (mc *markdownClient) ReplacePageMarkdown(ctx context.Context, pageID, markdown, version string) (*PageMarkdownResponse, error) {
	body := withMarkdownVersion(map[string]interface{}{
		"type": "replace_content",
		"replace_content": map[string]interface{}{
			"new_str":                markdown,
			"allow_deleting_content": true,
		},
	}, version)

	respBody, err := mc.doRequest(ctx, http.MethodPatch, pageMarkdownWriteURL(pageID), body)
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
func (mc *markdownClient) InsertPageMarkdown(ctx context.Context, pageID, markdown, position, version string) (*PageMarkdownResponse, error) {
	if position != "start" && position != "end" {
		return nil, fmt.Errorf("InsertPageMarkdown: position must be \"start\" or \"end\", got %q", position)
	}

	body := withMarkdownVersion(insertContentBody(markdown, position), version)

	respBody, err := mc.doRequest(withNoReplay(ctx), http.MethodPatch, pageMarkdownWriteURL(pageID), body)
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
