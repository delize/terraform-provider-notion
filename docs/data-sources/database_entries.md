---
page_title: "notion_database_entries Data Source - Notion"
subcategory: ""
description: |-
  List all entries in a Notion database.
---

# notion_database_entries (Data Source)

Use this data source to list all entries (rows) in a Notion database.

## Example Usage

```terraform
data "notion_database_entries" "all_tasks" {
  database = notion_database.tasks.id
}

output "task_count" {
  value = length(data.notion_database_entries.all_tasks.entries)
}
```

## Schema

### Required

- `database` (String) The ID of the database to query.

### Read-Only

- `entries` (List of Object) List of database entries. Each entry has the following attributes:
  - `id` (String) The ID of the entry.
  - `title` (String) The title of the entry.
  - `url` (String) The URL of the entry in Notion.
