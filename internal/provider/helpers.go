package provider

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jomei/notionapi"
)

// normalizeID removes hyphens from a Notion ID to produce the 32-char hex form.
func normalizeID(id string) string {
	return strings.ReplaceAll(id, "-", "")
}

// mdLinkRe matches markdown links: [display text](url)
var mdLinkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

// richTextToPlain extracts plain text from a slice of RichText objects,
// reconstructing markdown link syntax for RichText elements that have a link.
func richTextToPlain(rt []notionapi.RichText) string {
	var sb strings.Builder
	for _, r := range rt {
		if r.Text != nil && r.Text.Link != nil && r.Text.Link.Url != "" {
			sb.WriteString("[")
			sb.WriteString(r.PlainText)
			sb.WriteString("](")
			sb.WriteString(r.Text.Link.Url)
			sb.WriteString(")")
		} else {
			sb.WriteString(r.PlainText)
		}
	}
	return sb.String()
}

// plainToRichText parses a string for markdown links [text](url) and creates
// a RichText slice with appropriate link annotations. Plain text without links
// produces a single RichText element (backward compatible).
func plainToRichText(text string) []notionapi.RichText {
	matches := mdLinkRe.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return []notionapi.RichText{
			{
				Type: notionapi.ObjectTypeText,
				Text: &notionapi.Text{Content: text},
			},
		}
	}

	var result []notionapi.RichText
	cursor := 0

	for _, m := range matches {
		// m[0]:m[1] = full match, m[2]:m[3] = display text, m[4]:m[5] = url
		if m[0] > cursor {
			plain := text[cursor:m[0]]
			result = append(result, notionapi.RichText{
				Type: notionapi.ObjectTypeText,
				Text: &notionapi.Text{Content: plain},
			})
		}

		display := text[m[2]:m[3]]
		url := text[m[4]:m[5]]
		result = append(result, notionapi.RichText{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{
				Content: display,
				Link:    &notionapi.Link{Url: url},
			},
		})

		cursor = m[1]
	}

	if cursor < len(text) {
		plain := text[cursor:]
		result = append(result, notionapi.RichText{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: plain},
		})
	}

	return result
}

// jsonToRichText parses a JSON-encoded array of Notion RichText objects.
func jsonToRichText(jsonStr string) ([]notionapi.RichText, error) {
	var rt []notionapi.RichText
	if err := json.Unmarshal([]byte(jsonStr), &rt); err != nil {
		return nil, fmt.Errorf("invalid rich_text_json: %w", err)
	}
	return rt, nil
}

// richTextToJSON serializes a slice of RichText objects to a JSON string.
func richTextToJSON(rt []notionapi.RichText) (string, error) {
	b, err := json.Marshal(rt)
	if err != nil {
		return "", fmt.Errorf("error serializing rich text to JSON: %w", err)
	}
	return string(b), nil
}
