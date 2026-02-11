---
page_title: "notion_database_property_rich_text Resource - Notion"
subcategory: ""
description: |-
  Manages a rich text property on a Notion database.
---

# notion_database_property_rich_text (Resource)

Manages a rich text property on a Notion database.

## Example Usage

```terraform
resource "notion_database_property_rich_text" "notes" {
  database = notion_database.tasks.id
  name     = "Notes"
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
terraform import notion_database_property_rich_text.notes <database-id>/<property-name>
```
