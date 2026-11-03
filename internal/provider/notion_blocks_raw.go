package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/jomei/notionapi"
)

// heading_4 (2026-03-30) and tab (2026-03-25) blocks aren't modeled by the
// jomei/notionapi SDK. Worse, the SDK decodes any block type it doesn't know
// into an empty UnsupportedBlock with no ID, so reads can't go through it
// either. This file creates, reads, updates, and lists those blocks over
// direct HTTP on Notion-Version 2026-03-11, the version the Append block
// children reference documents.
//
// callout blocks go the same way because a callout icon is the full icon
// union (emoji, external, custom_emoji, and the native `icon` type from
// 2026-03-25), and the SDK's Icon only models emoji, external, and file.
//
// A tab block is a container whose direct children are paragraph blocks.
// Each paragraph is one tab item: its rich_text is the tab label, its icon
// is the tab icon, and its own children are the tab's content.
//
// column_list blocks go this way too. Notion only creates a column list
// together with its columns (at least two, each with at least one child),
// and the SDK can't send nested children or a column width_ratio.

const rawBlockAPIVersion = "2026-03-11"

// rawBlock is the subset of a block object these shims need.
type rawBlock struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	HasChildren bool   `json:"has_children"`
	InTrash     bool   `json:"in_trash"`
	Archived    bool   `json:"archived"`
	Parent      struct {
		Type    string `json:"type"`
		PageID  string `json:"page_id"`
		BlockID string `json:"block_id"`
	} `json:"parent"`
	Heading4  *rawTextBlock `json:"heading_4,omitempty"`
	Paragraph *rawTextBlock `json:"paragraph,omitempty"`
	Callout   *rawTextBlock `json:"callout,omitempty"`
	Column    *rawColumn    `json:"column,omitempty"`
}

// rawColumn is the column object. width_ratio is absent when the column
// uses an equal share of the list.
type rawColumn struct {
	WidthRatio *float64 `json:"width_ratio,omitempty"`
}

type rawTextBlock struct {
	RichText     []notionapi.RichText `json:"rich_text"`
	Color        string               `json:"color"`
	IsToggleable bool                 `json:"is_toggleable"`
	// Icon is the icon union as Notion sent it. Decode it with icon().
	Icon json.RawMessage `json:"icon"`
}

// parentID returns the normalized page or block parent ID.
func (b *rawBlock) parentID() string {
	switch b.Parent.Type {
	case "page_id":
		return normalizeID(b.Parent.PageID)
	case "block_id":
		return normalizeID(b.Parent.BlockID)
	}
	return ""
}

// emoji returns the block's emoji icon, or "" for no icon or a non-emoji one.
func (t *rawTextBlock) emoji() string {
	return t.icon().Emoji
}

// icon decodes the block's icon. It's the zero parsedIcon when there's none.
func (t *rawTextBlock) icon() parsedIcon {
	if t == nil {
		return parsedIcon{}
	}
	return parseIcon(t.Icon)
}

// tabItemSpec is one tab item to create or update.
type tabItemSpec struct {
	Label string
	Icon  string // emoji, "" for none
}

// heading4Body builds the heading_4 payload for create and update.
func heading4Body(rt []notionapi.RichText, color string, toggleable bool) map[string]interface{} {
	body := map[string]interface{}{
		"rich_text":     richTextOrEmpty(rt),
		"is_toggleable": toggleable,
	}
	if color != "" {
		body["color"] = color
	}
	return body
}

// calloutBody builds the callout payload for create and update. icon is the
// request object from iconFields.request, or nil to leave the icon alone.
// Update a block only accepts an icon object, never null, so an icon can be
// set or changed through the API but not removed. See calloutIconRemoval.
// An empty color is sent as "default", so removing color from the config
// resets it and a new callout doesn't pick up some other server default.
func calloutBody(rt []notionapi.RichText, color string, icon map[string]interface{}) map[string]interface{} {
	if color == "" {
		color = "default"
	}
	body := map[string]interface{}{
		"rich_text": richTextOrEmpty(rt),
		"color":     color,
	}
	if icon != nil {
		body["icon"] = icon
	}
	return body
}

// tabItemBody builds a tab item paragraph payload. Update a block only
// accepts an icon object, never null, so an icon can be set or changed
// through the API but not removed. See tabIconRemovals.
func tabItemBody(item tabItemSpec) map[string]interface{} {
	body := map[string]interface{}{
		"rich_text": richTextOrEmpty(plainToRichText(item.Label)),
	}
	if item.Icon != "" {
		body["icon"] = map[string]string{"type": "emoji", "emoji": item.Icon}
	}
	return body
}

func tabItemBlock(item tabItemSpec) map[string]interface{} {
	return map[string]interface{}{
		"object":    "block",
		"type":      "paragraph",
		"paragraph": tabItemBody(item),
	}
}

// tabBlock builds a tab container with its items. Notion requires at least
// one item when creating a tab.
func tabBlock(items []tabItemSpec) map[string]interface{} {
	children := make([]map[string]interface{}, 0, len(items))
	for _, it := range items {
		children = append(children, tabItemBlock(it))
	}
	return map[string]interface{}{
		"object": "block",
		"type":   "tab",
		"tab":    map[string]interface{}{"children": children},
	}
}

// columnSpec is one column to create. A nil WidthRatio leaves the width to
// Notion, which splits the list evenly.
type columnSpec struct {
	WidthRatio *float64
}

// columnPlaceholder is the child every new column gets. Notion rejects a
// column with no children, so each column starts with one empty paragraph.
func columnPlaceholder() map[string]interface{} {
	return map[string]interface{}{
		"object":    "block",
		"type":      "paragraph",
		"paragraph": map[string]interface{}{"rich_text": []interface{}{}},
	}
}

// columnBlock builds one column with its placeholder child, for creating a
// column list or appending a column to one.
func columnBlock(c columnSpec) map[string]interface{} {
	body := map[string]interface{}{
		"children": []map[string]interface{}{columnPlaceholder()},
	}
	if c.WidthRatio != nil {
		body["width_ratio"] = *c.WidthRatio
	}
	return map[string]interface{}{
		"object": "block",
		"type":   "column",
		"column": body,
	}
}

// columnListBlock builds a column list with its columns. Notion requires at
// least two columns when the list is created.
func columnListBlock(cols []columnSpec) map[string]interface{} {
	children := make([]map[string]interface{}, 0, len(cols))
	for _, c := range cols {
		children = append(children, columnBlock(c))
	}
	return map[string]interface{}{
		"object":      "block",
		"type":        "column_list",
		"column_list": map[string]interface{}{"children": children},
	}
}

func richTextOrEmpty(rt []notionapi.RichText) []notionapi.RichText {
	if rt == nil {
		return []notionapi.RichText{}
	}
	return rt
}

// appendBlockRequest is the Append block children body. afterID places the
// new blocks after an existing sibling, via the 2026-03-11 position object.
func appendBlockRequest(children []map[string]interface{}, afterID string) map[string]interface{} {
	body := map[string]interface{}{"children": children}
	if afterID != "" {
		body["position"] = map[string]interface{}{
			"type":        "after_block",
			"after_block": map[string]string{"id": afterID},
		}
	}
	return body
}

func appendBlocksRaw(ctx context.Context, token, parentID string, children []map[string]interface{}, afterID string) ([]rawBlock, error) {
	body, err := json.Marshal(appendBlockRequest(children, afterID))
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s/blocks/%s/children", notionAPIBaseURL, parentID)
	respBody, err := doRawBlockRequest(ctx, http.MethodPatch, u, token, body)
	if err != nil {
		return nil, err
	}
	var list struct {
		Results []rawBlock `json:"results"`
	}
	if err := json.Unmarshal(respBody, &list); err != nil {
		return nil, fmt.Errorf("failed to parse append response: %w", err)
	}
	return list.Results, nil
}

func getBlockRaw(ctx context.Context, token, blockID string) (*rawBlock, error) {
	respBody, err := doRawBlockRequest(ctx, http.MethodGet, fmt.Sprintf("%s/blocks/%s", notionAPIBaseURL, blockID), token, nil)
	if err != nil {
		return nil, err
	}
	return parseRawBlock(respBody)
}

// updateBlockRaw PATCHes one block. payload is the type-keyed body, for
// example {"heading_4": {...}}.
func updateBlockRaw(ctx context.Context, token, blockID string, payload map[string]interface{}) (*rawBlock, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	respBody, err := doRawBlockRequest(ctx, http.MethodPatch, fmt.Sprintf("%s/blocks/%s", notionAPIBaseURL, blockID), token, body)
	if err != nil {
		return nil, err
	}
	return parseRawBlock(respBody)
}

// listBlockChildrenRaw returns every direct child of a block, following
// next_cursor verbatim (cursors are opaque since 2026-04-22).
func listBlockChildrenRaw(ctx context.Context, token, blockID string) ([]rawBlock, error) {
	var (
		all    []rawBlock
		cursor string
	)
	for {
		q := url.Values{"page_size": {"100"}}
		if cursor != "" {
			q.Set("start_cursor", cursor)
		}
		u := fmt.Sprintf("%s/blocks/%s/children?%s", notionAPIBaseURL, blockID, q.Encode())
		respBody, err := doRawBlockRequest(ctx, http.MethodGet, u, token, nil)
		if err != nil {
			return nil, err
		}
		var page struct {
			Results    []rawBlock `json:"results"`
			HasMore    bool       `json:"has_more"`
			NextCursor *string    `json:"next_cursor"`
		}
		if err := json.Unmarshal(respBody, &page); err != nil {
			return nil, fmt.Errorf("failed to parse block children: %w", err)
		}
		all = append(all, page.Results...)
		if !page.HasMore || page.NextCursor == nil || *page.NextCursor == "" {
			return all, nil
		}
		cursor = *page.NextCursor
	}
}

func parseRawBlock(body []byte) (*rawBlock, error) {
	var b rawBlock
	if err := json.Unmarshal(body, &b); err != nil {
		return nil, fmt.Errorf("failed to parse block: %w", err)
	}
	return &b, nil
}

func doRawBlockRequest(ctx context.Context, method, u, token string, body []byte) ([]byte, error) {
	resp, err := doNotionRequestWithVersion(ctx, method, u, token, rawBlockAPIVersion, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("notion API %d on %s %s: %s", resp.StatusCode, method, u, string(respBody))
	}
	return respBody, nil
}
