---
page_title: "notion_database_property_created_by Resource - Notion"
subcategory: ""
description: |-
  Manages a created by property on a Notion database.
---

# notion_database_property_created_by (Resource)

Manages a created by property on a Notion database. This property is automatically populated by Notion with the user who created each entry.

## Example Usage

```terraform
resource "notion_database_property_created_by" "creator" {
  database = notion_database.tasks.id
  name     = "Created By"
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
terraform import notion_database_property_created_by.creator <database-id>/<property-name>
```
