---
page_title: "notion_view_query Data Source - Notion"
subcategory: ""
description: |-
  Query a Notion view.
---

# notion_view_query (Data Source)

Query a Notion view (2026-03-19 Views API). The view's saved `filter` and `sorts` are applied server-side. Returns one page of results as raw JSON for callers to parse with `jsondecode`.

Notion runs view queries in steps. The first read creates a query with `POST /v1/views/{id}/queries`, which returns the first page and a `query_id`. Notion caches the result set for about 15 minutes. To read a later page, set `query_id` and `start_cursor` from the earlier read, which fetches it with `GET /v1/views/{id}/queries/{query_id}`. Cursors are opaque strings, so pass them through verbatim.

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
- `query_id` (String) ID of a cached query from an earlier read. Leave unset to start a new query. Also read-only output.
- `start_cursor` (String) Opaque cursor returned in a prior response's `next_cursor`. Requires `query_id`.

### Read-Only

- `next_cursor` (String) Cursor to fetch the next page, or empty when
  `has_more` is false.
- `has_more` (Boolean) Whether more results are available.
- `raw_json` (String) Raw JSON response. Use `jsondecode` to access the
  `results` array.
