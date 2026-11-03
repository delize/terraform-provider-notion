---
page_title: "notion_database_property_multi_select Resource - Notion"
subcategory: ""
description: |-
  Manages a multi-select property on a Notion database.
---

# notion_database_property_multi_select (Resource)

Manages a multi-select property on a Notion database. Multi-select properties allow choosing multiple options from a predefined list.

Entries set values with `multi_select_properties` on `notion_database_entry`. Notion adds any option name an entry writes that isn't in the list yet. List every option an entry uses in `options` here, or this resource will show the added option as drift on its next plan.

## Example Usage

```terraform
resource "notion_database_property_multi_select" "tags" {
  database = notion_database.tasks.id
  name     = "Tags"
  options = {
    "Bug"     = "red"
    "Feature" = "blue"
    "Docs"    = "green"
  }
}
```

## Schema

### Required

- `database` (String) The ID of the parent database. Changing this forces a new resource.
- `name` (String) The name of the property. Changing this forces a new resource.
- `options` (Map of String) A map of option labels to colors. Valid colors: `default`, `gray`, `brown`, `orange`, `yellow`, `green`, `blue`, `purple`, `pink`, `red`. Changing an existing option's color updates it in place (Notion API 2026-10-09). Notion matches option names without regard to case, so `high` and `High` are the same option. Removing an option from the map removes it from the property.

### Read-Only

- `id` (String) The ID of the property.

## Import

Multi-select properties can be imported using a composite ID:

```shell
terraform import notion_database_property_multi_select.tags <database-id>/<property-name>
```
