---
page_title: "notion_page Data Source - Notion"
subcategory: ""
description: |-
  Look up an existing Notion page by title.
---

# notion_page (Data Source)

Use this data source to look up an existing Notion page by its title. Returns the first matching page.

## Example Usage

```terraform
data "notion_page" "existing" {
  query = "My Existing Page"
}

resource "notion_database" "tasks" {
  parent             = data.notion_page.existing.id
  title              = "Tasks"
  title_column_title = "Name"
}
```

## Schema

### Required

- `query` (String) Search query to find the page by title.

### Read-Only

- `id` (String) The ID of the page.
- `parent_page_id` (String) The ID of the parent page, if applicable.
- `title` (String) The title of the page.
- `url` (String) The URL of the page in Notion.
