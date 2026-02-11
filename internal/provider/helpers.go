package provider

import (
	"strings"

	"github.com/jomei/notionapi"
)

// normalizeID removes hyphens from a Notion ID to produce the 32-char hex form.
func normalizeID(id string) string {
	return strings.ReplaceAll(id, "-", "")
}

// richTextToPlain extracts plain text from a slice of RichText objects.
func richTextToPlain(rt []notionapi.RichText) string {
	var sb strings.Builder
	for _, r := range rt {
		sb.WriteString(r.PlainText)
	}
	return sb.String()
}

// plainToRichText creates a simple RichText slice from a plain string.
func plainToRichText(text string) []notionapi.RichText {
	return []notionapi.RichText{
		{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: text},
		},
	}
}
