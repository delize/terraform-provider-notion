---
page_title: "notion_database_entries Data Source - Notion"
subcategory: ""
description: |-
  List all entries in a Notion database with their property values.
---

# notion_database_entries (Data Source)

Use this data source to list all entries (rows) in a Notion database. Each entry includes a `properties` map containing all column values as strings, allowing you to read any database property (select, rich text, number, date, etc.).

## Example Usage

```terraform
data "notion_database_entries" "all_tasks" {
  database = notion_database.tasks.id
}

# Access a specific property value from the first entry
output "first_task_status" {
  value = data.notion_database_entries.all_tasks.entries[0].properties["Status"]
}

# Loop through entries
output "task_titles" {
  value = [for entry in data.notion_database_entries.all_tasks.entries : entry.title]
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
  - `properties` (Map of String) A map of property names to their string values. All property types are converted to strings:
    - **Title / Rich Text** - plain text content
    - **Number** - numeric string (e.g. `"42"`)
    - **Select / Status** - option name (e.g. `"Done"`)
    - **Multi-select** - comma-separated names (e.g. `"Bug, Feature"`)
    - **Date** - RFC3339 format (e.g. `"2026-01-15T00:00:00Z"`)
    - **Checkbox** - `"true"` or `"false"`
    - **URL / Email / Phone** - raw string value
    - **People** - comma-separated user names
    - **Relation** - comma-separated page IDs
    - **Formula** - computed result as string
    - **Rollup** - aggregated result as string
    - **Unique ID** - prefixed ID (e.g. `"PROJ-123"`)
    - **Created time / Last edited time** - RFC3339 timestamp
    - **Created by / Last edited by** - user name
