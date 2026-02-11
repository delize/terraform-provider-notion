---
page_title: "notion_database_property_select Resource - Notion"
subcategory: ""
description: |-
  Manages a select property on a Notion database.
---

# notion_database_property_select (Resource)

Manages a select property on a Notion database. Select properties allow choosing one option from a predefined list.

## Example Usage

```terraform
resource "notion_database_property_select" "status" {
  database = notion_database.tasks.id
  name     = "Status"
  options = {
    "To Do"       = "red"
    "In Progress" = "yellow"
    "Done"        = "green"
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

Select properties can be imported using a composite ID:

```shell
terraform import notion_database_property_select.status <database-id>/<property-name>
```
