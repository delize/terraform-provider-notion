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

Each option belongs to one of the **To-do**, **In progress**, or **Complete** groups. Use `option_groups` to place options in a group (Notion API 2026-06-22). Options you leave out of `option_groups` keep their current group, and new ones go to To-do. Renaming or reordering the groups themselves still happens in Notion.

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

  option_groups = {
    "Blocked" = "In progress"
    "Done"    = "Complete"
  }
}
```

## Schema

### Required

- `database` (String) The ID of the parent database. Changing this forces a new resource.
- `name` (String) The name of the property. Changing this forces a new resource.
- `options` (Map of String) Map of option label to color. Valid colors:
  `default`, `gray`, `brown`, `orange`, `yellow`, `green`, `blue`, `purple`,
  `pink`, `red`. Changing an existing option's color updates it in place (Notion API 2026-10-09). Notion matches option names without regard to case, so `high` and `High` are the same option.

### Optional

- `option_groups` (Map of String) Map of option label to group: `To-do`, `In progress`, or `Complete`. Every key must also be in `options`. Only the listed options are tracked, so groups changed in Notion for other options don't show as drift.

### Read-Only

- `id` (String) The ID of the property.
- `default_option` (String) Name of the option a page gets when it has no explicit status, or null when none is configured (Notion API 2026-10-22). The default isn't necessarily the first option. It is set in Notion.

## Import

```shell
terraform import notion_database_property_status.task_status <database-id>/<property-name>
```
