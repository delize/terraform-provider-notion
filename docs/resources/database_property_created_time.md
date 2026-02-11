---
page_title: "notion_database_property_created_time Resource - Notion"
subcategory: ""
description: |-
  Manages a created time property on a Notion database.
---

# notion_database_property_created_time (Resource)

Manages a created time property on a Notion database. This property is automatically populated by Notion with the timestamp when each entry was created.

## Example Usage

```terraform
resource "notion_database_property_created_time" "created" {
  database = notion_database.tasks.id
  name     = "Created"
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
terraform import notion_database_property_created_time.created <database-id>/<property-name>
```
