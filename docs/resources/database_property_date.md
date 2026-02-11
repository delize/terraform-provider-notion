---
page_title: "notion_database_property_date Resource - Notion"
subcategory: ""
description: |-
  Manages a date property on a Notion database.
---

# notion_database_property_date (Resource)

Manages a date property on a Notion database.

## Example Usage

```terraform
resource "notion_database_property_date" "due_date" {
  database = notion_database.tasks.id
  name     = "Due Date"
}
```

## Schema

### Required

- `database` (String) The ID of the parent database. Changing this forces a new resource.
- `name` (String) The name of the property. Changing this forces a new resource.

### Read-Only

- `id` (String) The ID of the property.

## Import

```shell
terraform import notion_database_property_date.due_date <database-id>/<property-name>
```
