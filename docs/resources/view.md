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
    { property = "Priority", direction = "descending" }
  ])

  # Keys name a property by name or ID. The title property's ID is "title".
  quick_filters = jsonencode({ title = {} })

  configuration = jsonencode({
    type       = "table"
    properties = [{ property_id = "title", visible = true, width = 320 }]
    wrap_cells = true
  })
}
```

## How the JSON attributes read back

Notion returns these values with its own key order and whitespace, and with more than you sent. The provider keeps the string you configured as long as Notion's value means the same thing, so none of that shows as a diff. When the value really differs, for example because someone changed the filter in the Notion app, state holds Notion's value as compact JSON with sorted keys (the same form `jsonencode` produces) and the next plan shows the change.

- **Property names and IDs.** You can refer to a property by name or by ID in a filter or sort `property`, a `quick_filters` key, and a `property_id` inside `configuration`. Notion stores the ID, so the provider reads the data source's properties (`GET /v1/data_sources/{id}`, one extra request when any of these attributes is set) and compares names and IDs as the same property. The title property's ID is always `title`.
- **Configuration.** Notion's configuration contains more than the config sets. Updates are a shallow merge, so top-level fields you leave out keep their value, and a table's `properties` list comes back with every property of the data source, each with a read-only `property_name`. The provider compares only what the config sets. Entries in a list with `property_id` are matched by property in any order, and properties you don't list are ignored. Removing a field such as `wrap_cells` from the config doesn't reset it in Notion, so set it explicitly instead.
- **Timestamp sorts.** Notion accepts a sort on `created_time` or `last_edited_time` for a view but doesn't keep it, so the provider rejects one at plan time. Sort on a created time or last edited time property instead.
- **Quick filters.** Removing a key from `quick_filters` removes that quick filter from the view. Notion keeps quick filters an update doesn't name, so the provider sends `null` for keys the previous state had and the config dropped, matching names and IDs through the data source.
- **Import.** An import has no config to compare with, so state holds Notion's values with property IDs. If the config names properties, the first plan after an import shows an update that sends the same filter and sorts again, and plans are clean after that.
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
