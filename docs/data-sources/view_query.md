---
page_title: "notion_view_query Data Source - Notion"
subcategory: ""
description: |-
  Query a Notion view.
---

# notion_view_query (Data Source)

Query a Notion view via `POST /v1/views/{id}/query` (2026-03-19 Views API).
The view's saved `filter` and `sorts` are applied server-side. Returns one
page of results as raw JSON for callers to parse with `jsondecode`.

Iterate via `next_cursor` and `has_more`. Cursors are opaque strings (per
the 2026-04-22 pagination change) — pass them through verbatim, don't parse.

## Example Usage

```terraform
data "notion_view_query" "first_page" {
  view_id   = notion_view.by_status.id
  page_size = 100
}

locals {
  results = jsondecode(data.notion_view_query.first_page.raw_json).results
}
```

## Schema

### Required

- `view_id` (String) ID of the view to query.

### Optional

- `page_size` (Number) Maximum number of results per page (1-100).
- `start_cursor` (String) Opaque cursor returned in a prior response's
  `next_cursor`.

### Read-Only

- `next_cursor` (String) Cursor to fetch the next page, or empty when
  `has_more` is false.
- `has_more` (Boolean) Whether more results are available.
- `raw_json` (String) Raw JSON response. Use `jsondecode` to access the
  `results` array.
