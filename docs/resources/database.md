---
page_title: "notion_database Resource - Notion"
subcategory: ""
description: |-
  Manages a Notion database.
---

# notion_database (Resource)

Manages a Notion database. Databases are created under a parent page and include a title column by default.

~> **Note:** Destroying a database archives it in Notion rather than permanently deleting it.

## Example Usage

```terraform
resource "notion_database" "tasks" {
  parent             = notion_page.example.id
  title              = "Tasks"
  title_column_title = "Task Name"
}
```

## Schema

### Required

- `parent` (String) The ID of the parent page. Changing this forces a new resource.
- `title` (String) The title of the database.
- `title_column_title` (String) The name of the title column (every Notion database has one).

### Read-Only

- `id` (String) The ID of the database.
- `title_column_id` (String) The ID of the title column.
- `url` (String) The URL of the database in Notion.

## Import

Databases can be imported using their Notion database ID:

```shell
terraform import notion_database.tasks <database-id>
```
