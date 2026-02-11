---
page_title: "notion_database_property_people Resource - Notion"
subcategory: ""
description: |-
  Manages a people property on a Notion database.
---

# notion_database_property_people (Resource)

Manages a people property on a Notion database. People properties allow assigning Notion users to entries.

## Example Usage

```terraform
resource "notion_database_property_people" "assignee" {
  database = notion_database.tasks.id
  name     = "Assignee"
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
terraform import notion_database_property_people.assignee <database-id>/<property-name>
```
