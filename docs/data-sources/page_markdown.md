---
page_title: "notion_page_markdown Data Source - Notion"
subcategory: ""
description: |-
  Retrieve a Notion page's content as markdown.
---

# notion_page_markdown (Data Source)

Retrieve a Notion page's content as markdown through [`GET /v1/pages/{page_id}/markdown`](https://developers.notion.com/reference/retrieve-page-markdown). Pages larger than about 20,000 blocks come back truncated, and this data source returns an error for those rather than partial content.

The syntax follows `markdown_version`. `v2` returns [Notion-flavored Markdown](https://developers.notion.com/guides/data-apis/notion-flavored-markdown), which Notion recommends. `v1` returns [enhanced markdown](https://developers.notion.com/guides/data-apis/enhanced-markdown), which Notion has deprecated. When neither this data source nor the provider block sets it, Notion returns `v1`. The next Notion API version will make `v2` the default and reject `v1`.

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

# Read as Notion-flavored Markdown
data "notion_page_markdown" "runbook_v2" {
  page_id          = "1234567890abcdef1234567890abcdef"
  markdown_version = "v2"
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
- `markdown_version` (String) `v2` (Notion-flavored Markdown) or `v1` (enhanced markdown, deprecated). Overrides the provider's `markdown_version`. When neither is set, Notion returns `v1`.

### Read-Only

- `markdown` (String) The page content as markdown in the requested syntax.
