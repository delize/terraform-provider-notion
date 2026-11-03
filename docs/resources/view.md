---
page_title: "notion_view Resource - Notion"
subcategory: ""
description: |-
  Manages a Notion database view.
---

# notion_view (Resource)

Manages a Notion database view via the 2026-03-19 `/v1/views` endpoints. View
types include `table`, `board`, `list`, `calendar`, `timeline`, `gallery`,
`form`, `chart`, `map`, and `dashboard`.

`filter`, `sorts`, `quick_filters`, and `configuration` accept raw JSON
strings — these mirror the same shape as the data source query API and would
balloon if modeled as nested attributes. Use `jsonencode` to build them.

## Example Usage

```terraform
resource "notion_view" "by_status" {
  database_id    = notion_database.tasks.id
  data_source_id = notion_database.tasks.default_data_source_id
  name           = "By Status"
  type           = "board"

  sorts = jsonencode([
    { property = "Status", direction = "ascending" }
  ])

  configuration = jsonencode({
    type = "board"
    board = {
      group_by = "Status"
    }
  })
}
```

## Schema

### Required

- `data_source_id` (String) ID of the data source this view is scoped to. Under
  the v2025-09-03 multi-source databases model, the data source is distinct
  from the database. Changing this forces a new resource.
- `name` (String) Display name of the view.
- `type` (String) View type. One of `table`, `board`, `list`, `calendar`,
  `timeline`, `gallery`, `form`, `chart`, `map`, `dashboard`. Changing this
  forces a new resource.

### Optional

- `database_id` (String) ID of the database to create the view in. Mutually
  exclusive with `parent_view_id`. Exactly one is required at create.
  Changing this forces a new resource.
- `parent_view_id` (String) ID of a dashboard view to add this view to as a
  widget. Mutually exclusive with `database_id`. Changing this forces a new
  resource.
- `filter` (String) Filter as a JSON object string.
- `sorts` (String) Sorts as a JSON array string (max 100 entries).
- `quick_filters` (String) Quick filters pinned in the view's filter bar, as a
  JSON object string.
- `configuration` (String) View presentation configuration as a JSON object
  string. The inner `type` field must match the view's `type` attribute.

### Read-Only

- `id` (String) The view ID.
- `url` (String) Deep link to the view in Notion.

## Import

```shell
terraform import notion_view.by_status <view-id>
```
