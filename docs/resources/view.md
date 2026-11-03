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

`filter`, `sorts`, `quick_filters`, and `configuration` accept JSON strings in the shapes the [Working with views](https://developers.notion.com/guides/data-apis/working-with-views) guide describes. Filter and sorts use the same shape as a data source query. Use `jsonencode` to build them.

## Example Usage

```terraform
resource "notion_view" "open_work" {
  database_id    = notion_database.tasks.id
  data_source_id = notion_database.tasks.data_source_ids[0]
  name           = "Open work"
  type           = "table"

  filter = jsonencode({
    property = "Priority"
    select   = { equals = "High" }
  })

  sorts = jsonencode([
    { timestamp = "created_time", direction = "descending" }
  ])

  # Quick filter keys are property IDs. The title property's ID is "title".
  quick_filters = jsonencode({ title = {} })

  configuration = jsonencode({
    type       = "table"
    properties = [{ property_id = "title", visible = true, width = 320 }]
    wrap_cells = true
  })
}
```

## How the JSON attributes read back

Notion returns these values with its own key order and whitespace, and sometimes with more than you sent. The provider keeps the string you configured as long as Notion's value decodes to the same JSON, so formatting never shows as a diff. When the value really differs, for example because someone changed the filter in the Notion app, state holds Notion's value as compact JSON with sorted keys (the same form `jsonencode` produces) and the next plan shows the change. An import stores the same normalized form.

- `configuration` updates are a shallow merge in Notion, so top-level fields you leave out keep whatever value the view had. The provider ignores those extra top-level fields when it compares. Removing a field such as `wrap_cells` from the config doesn't reset it in Notion, so set it explicitly instead. Fields inside a configured value, such as the entries of `properties`, must match exactly.
- Removing a key from `quick_filters` removes that quick filter from the view. Notion keeps quick filters an update doesn't name, so the provider sends `null` for keys the previous state had and the config dropped. It skips that when the config also adds a key the state doesn't have, in case the two name the same property by name and by ID.
- Notion resolves property names to property IDs in `quick_filters` keys and in `property_id` fields inside `configuration`. Use property IDs there, or the value Notion returns won't match the config and the apply fails with an inconsistent result. The title property's ID is always `title`.
- To clear the filter, sorts, or all quick filters, set the attribute to `jsonencode(null)`, which the views guide documents for updates. Removing the attribute from the config leaves Notion's value in place, because an attribute that isn't configured follows whatever Notion has.

## Schema

### Required

- `data_source_id` (String) ID of the data source this view is scoped to. Under
  the v2025-09-03 multi-source databases model, the data source is distinct
  from the database. Changing this forces a new resource. For a database in the same configuration, use `notion_database.<name>.data_source_ids[0]`.
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
