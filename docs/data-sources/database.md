---
page_title: "notion_database Data Source - Notion"
subcategory: ""
description: |-
  Look up an existing Notion database by title.
---

# notion_database (Data Source)

Use this data source to look up an existing Notion database by its title. Returns the first matching database.

## Example Usage

```terraform
data "notion_database" "existing" {
  query = "My Existing Database"
}

output "database_url" {
  value = data.notion_database.existing.url
}
```

## Schema

### Required

- `query` (String) Search query to find the database by title.

### Read-Only

- `id` (String) The ID of the database.
- `title` (String) The title of the database.
- `url` (String) The URL of the database in Notion.
