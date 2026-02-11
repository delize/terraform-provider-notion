---
page_title: "notion_database_property_relation Resource - Notion"
subcategory: ""
description: |-
  Manages a relation property on a Notion database.
---

# notion_database_property_relation (Resource)

Manages a relation property on a Notion database. Relation properties create links between entries in different databases.

## Example Usage

```terraform
resource "notion_database_property_relation" "project" {
  database         = notion_database.tasks.id
  name             = "Project"
  related_database = notion_database.projects.id
}
```

## Schema

### Required

- `database` (String) The ID of the parent database. Changing this forces a new resource.
- `name` (String) The name of the property. Changing this forces a new resource.
- `related_database` (String) The ID of the database to relate to.

### Read-Only

- `id` (String) The ID of the property.

## Import

Relation properties can be imported using a composite ID:

```shell
terraform import notion_database_property_relation.project <database-id>/<property-name>
```
