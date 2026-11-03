---
page_title: "notion_page_markdown Data Source - Notion"
subcategory: ""
description: |-
  Retrieve a Notion page's content as enhanced markdown.
---

# notion_page_markdown (Data Source)

Retrieve a Notion page's content as [enhanced markdown](https://developers.notion.com/guides/data-apis/enhanced-markdown) through [`GET /v1/pages/{page_id}/markdown`](https://developers.notion.com/reference/retrieve-page-markdown). Pages larger than about 20,000 blocks come back truncated, and this data source returns an error for those rather than partial content.

## Example Usage

```terraform
data "notion_page_markdown" "runbook" {
  page_id = "1234567890abcdef1234567890abcdef"
}

# Include full AI meeting note transcripts instead of placeholders
data "notion_page_markdown" "standup" {
  page_id            = "abcdef1234567890abcdef1234567890"
  include_transcript = true
}

output "runbook_markdown" {
  value = data.notion_page_markdown.runbook.markdown
}
```

## Schema

### Required

- `page_id` (String) The ID of the page to retrieve markdown for.

### Optional

- `include_transcript` (Boolean) Include full AI meeting note transcripts in the markdown (Notion API 2026-02-26). Without it, each meeting note renders as a placeholder with its URL. Defaults to `false`.

### Read-Only

- `markdown` (String) The page content rendered as enhanced markdown.
