package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// The jomei/notionapi SDK is pinned to Notion-Version 2022-06-28 and
// implements only the original Comment.Create (rich_text) and Comment.Get
// (list). Notion shipped Update and Delete comment endpoints in April 2026
// and a `markdown` body parameter for Create/Update at the same time. This
// file shims those operations using the shared doNotionRequest helper from
// notion_trash.go (which already handles 429 retry on Notion-Version
// 2026-03-11).

// commentBody is the create/update request body. Exactly one of RichText or
// Markdown must be non-empty per Notion's schema; the resource enforces this
// before calling.
type commentBody struct {
	// Used only on create.
	Parent       *commentParent `json:"parent,omitempty"`
	DiscussionID string         `json:"discussion_id,omitempty"`

	// Body — exactly one of these.
	RichText []notionRichText `json:"rich_text,omitempty"`
	Markdown string           `json:"markdown,omitempty"`
}

type commentParent struct {
	Type   string `json:"type"`
	PageID string `json:"page_id,omitempty"`
}

// notionRichText is the minimal rich-text shape the comment endpoints need
// for both request and response. The SDK's RichText type works too, but we
// keep a local type so the shim can decode the markdown-server-rendered form
// without pulling in the full SDK schema.
type notionRichText struct {
	Type      string                 `json:"type"`
	Text      *notionRichTextText    `json:"text,omitempty"`
	PlainText string                 `json:"plain_text,omitempty"`
	Href      string                 `json:"href,omitempty"`
	Mention   map[string]interface{} `json:"mention,omitempty"`
}

type notionRichTextText struct {
	Content string                 `json:"content"`
	Link    *notionRichTextLink    `json:"link,omitempty"`
}

type notionRichTextLink struct {
	URL string `json:"url"`
}

// commentResponse is the relevant subset of /v1/comments responses we surface
// to Terraform.
type commentResponse struct {
	ID             string           `json:"id"`
	DiscussionID   string           `json:"discussion_id"`
	CreatedTime    string           `json:"created_time"`
	LastEditedTime string           `json:"last_edited_time"`
	CreatedBy      struct {
		ID string `json:"id"`
	} `json:"created_by"`
	RichText []notionRichText `json:"rich_text"`
}

// createComment POSTs /v1/comments. parentPageID and discussionID are
// mutually exclusive. richTextContent and markdown are also mutually
// exclusive — pass exactly one as a non-empty string (richText is sent as a
// single text segment).
func createComment(ctx context.Context, token, parentPageID, discussionID, richTextContent, markdown string) (*commentResponse, error) {
	body := commentBody{}
	switch {
	case parentPageID != "":
		body.Parent = &commentParent{Type: "page_id", PageID: parentPageID}
	case discussionID != "":
		body.DiscussionID = discussionID
	default:
		return nil, fmt.Errorf("createComment: parent_page_id or discussion_id is required")
	}
	switch {
	case richTextContent != "":
		body.RichText = []notionRichText{{Type: "text", Text: &notionRichTextText{Content: richTextContent}}}
	case markdown != "":
		body.Markdown = markdown
	default:
		return nil, fmt.Errorf("createComment: rich_text or markdown is required")
	}

	return commentRequest(ctx, http.MethodPost, notionAPIBaseURL+"/comments", token, &body)
}

// updateComment PATCHes /v1/comments/{id}. Comment update is restricted to
// comments the integration created (Notion returns 404 otherwise).
func updateComment(ctx context.Context, token, commentID, richTextContent, markdown string) (*commentResponse, error) {
	body := commentBody{}
	switch {
	case richTextContent != "":
		body.RichText = []notionRichText{{Type: "text", Text: &notionRichTextText{Content: richTextContent}}}
	case markdown != "":
		body.Markdown = markdown
	default:
		return nil, fmt.Errorf("updateComment: rich_text or markdown is required")
	}

	return commentRequest(ctx, http.MethodPatch, fmt.Sprintf("%s/comments/%s", notionAPIBaseURL, commentID), token, &body)
}

// deleteComment DELETEs /v1/comments/{id}. Returns the deleted comment per
// the Notion API contract.
func deleteComment(ctx context.Context, token, commentID string) error {
	resp, err := doNotionRequest(ctx, http.MethodDelete, fmt.Sprintf("%s/comments/%s", notionAPIBaseURL, commentID), token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("notion API %d deleting comment %s: %s", resp.StatusCode, commentID, string(respBody))
	}
	return nil
}

// commentRequest centralizes the JSON-encode + decode glue around
// doNotionRequest for create/update.
func commentRequest(ctx context.Context, method, url, token string, body *commentBody) (*commentResponse, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	resp, err := doNotionRequest(ctx, method, url, token, bodyBytes)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("notion API %d on %s %s: %s", resp.StatusCode, method, url, string(respBody))
	}

	var out commentResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// richTextPlainFromShim flattens a notionRichText slice to plain text.
func richTextPlainFromShim(rts []notionRichText) string {
	out := ""
	for _, rt := range rts {
		if rt.PlainText != "" {
			out += rt.PlainText
			continue
		}
		if rt.Text != nil {
			out += rt.Text.Content
		}
	}
	return out
}
