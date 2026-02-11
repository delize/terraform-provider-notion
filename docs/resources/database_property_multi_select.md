---
page_title: "notion_database_property_multi_select Resource - Notion"
subcategory: ""
description: |-
  Manages a multi-select property on a Notion database.
---

# notion_database_property_multi_select (Resource)

Manages a multi-select property on a Notion database. Multi-select properties allow choosing multiple options from a predefined list.

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
- `options` (Map of String) A map of option labels to colors. Valid colors: `default`, `gray`, `brown`, `orange`, `yellow`, `green`, `blue`, `purple`, `pink`, `red`.

### Read-Only

- `id` (String) The ID of the property.

## Import

Multi-select properties can be imported using a composite ID:

```shell
terraform import notion_database_property_multi_select.tags <database-id>/<property-name>
```
