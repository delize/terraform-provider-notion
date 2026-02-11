---
page_title: "notion_database_property_last_edited_time Resource - Notion"
subcategory: ""
description: |-
  Manages a last edited time property on a Notion database.
---

# notion_database_property_last_edited_time (Resource)

Manages a last edited time property on a Notion database. This property is automatically populated by Notion with the timestamp of the last edit to each entry.

## Example Usage

```terraform
resource "notion_database_property_last_edited_time" "updated" {
  database = notion_database.tasks.id
  name     = "Last Updated"
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
terraform import notion_database_property_last_edited_time.updated <database-id>/<property-name>
```
