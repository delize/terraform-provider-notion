---
page_title: "notion_database_property_status Resource - Notion"
subcategory: ""
description: |-
  Manages a status property on a Notion database.
---

# notion_database_property_status (Resource)

Manages a status property on a Notion database. Status properties became
writable via the Notion API in the 2026-03-19 change; prior to that they could
only be created through the Notion UI.

Notion assigns each option to one of the **To-do**, **In progress**, or
**Complete** groups server-side based on the option set; this resource does
not model group membership directly.

## Example Usage

```terraform
resource "notion_database_property_status" "task_status" {
  database = notion_database.tasks.id
  name     = "Status"
  options = {
    "Not started" = "default"
    "In progress" = "blue"
    "Blocked"     = "red"
    "Done"        = "green"
  }
}
```

## Schema

### Required

- `database` (String) The ID of the parent database. Changing this forces a new resource.
- `name` (String) The name of the property. Changing this forces a new resource.
- `options` (Map of String) Map of option label to color. Valid colors:
  `default`, `gray`, `brown`, `orange`, `yellow`, `green`, `blue`, `purple`,
  `pink`, `red`.

### Read-Only

- `id` (String) The ID of the property.

## Import

```shell
terraform import notion_database_property_status.task_status <database-id>/<property-name>
```
