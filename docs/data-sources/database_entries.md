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

### Filtering and sorting

`filter` and `sorts` take Notion's own [filter](https://developers.notion.com/reference/post-database-query-filter) and [sort](https://developers.notion.com/reference/post-database-query-sort) JSON, usually built with `jsonencode`. They're passed to the query unchanged, so newer conditions work as soon as Notion supports them. Examples include relative dates and `"me"` for people (2026-03-30), and arrays of values for select, status, and multi-select (2026-04-17).

```terraform
# Open tasks due by the end of today, soonest first
data "notion_database_entries" "due_today" {
  database = notion_database.tasks.id

  filter = jsonencode({
    and = [
      { property = "Due", date = { on_or_before = "today" } },
      { property = "Status", status = { does_not_equal = ["Done", "Cancelled"] } },
    ]
  })

  sorts = jsonencode([
    { property = "Due", direction = "ascending" },
  ])
}

# Entries assigned to the user who authorized a public connection. With an
# internal integration "me" matches nobody, see Notion's people filter docs.
data "notion_database_entries" "mine" {
  database = notion_database.tasks.id
  filter = jsonencode({
    property = "Assignee"
    people   = { contains = "me" }
  })
}
```

A query that matches more than 10,000 rows returns a partial result with a warning (2026-04-20). A filter is the way to stay under that cap.

## Schema

### Required

- `database` (String) The ID of the database to query.

### Optional

- `filter` (String) Notion filter object as JSON. Must be a JSON object.
- `sorts` (String) Notion sorts array as JSON. Must be a JSON array.

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
    - **Formula** - computed result as string. Empty when Notion returns `type: "unsupported"` because the formula depends on too many related pages or nested formulas (2026-08-05).
    - **Rollup** - aggregated result as string. Empty when Notion returns `type: "unsupported"` for the same reason.
    - **Unique ID** - prefixed ID (e.g. `"PROJ-123"`)
    - **Created time / Last edited time** - RFC3339 timestamp
    - **Created by / Last edited by** - user name
