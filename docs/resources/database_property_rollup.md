---
page_title: "notion_database_property_rollup Resource - Notion"
subcategory: ""
description: |-
  Manages a rollup property on a Notion database.
---

# notion_database_property_rollup (Resource)

Manages a rollup property on a Notion database. Rollup properties aggregate values from a related database through a relation property.

## Example Usage

```terraform
resource "notion_database_property_rollup" "task_count" {
  database          = notion_database.projects.id
  name              = "Task Count"
  function          = "count_all"
  relation_property = "Tasks"
  rollup_property   = "Name"
}
```

## Schema

### Required

- `database` (String) The ID of the parent database. Changing this forces a new resource.
- `name` (String) The name of the property. Changing this forces a new resource.
- `function` (String) The rollup aggregation function. Valid values: `count_all`, `count_values`, `count_unique_values`, `count_empty`, `count_not_empty`, `percent_empty`, `percent_not_empty`, `sum`, `average`, `median`, `min`, `max`, `range`.
- `relation_property` (String) The name of the relation property to roll up through.
- `rollup_property` (String) The name of the property in the related database to aggregate.

### Read-Only

- `id` (String) The ID of the property.

## Import

Rollup properties can be imported using a composite ID:

```shell
terraform import notion_database_property_rollup.task_count <database-id>/<property-name>
```
