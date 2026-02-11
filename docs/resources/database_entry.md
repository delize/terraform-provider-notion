---
page_title: "notion_database_entry Resource - Notion"
subcategory: ""
description: |-
  Manages an entry (row) in a Notion database.
---

# notion_database_entry (Resource)

Manages an entry (row) in a Notion database. Each entry has a title that corresponds to the database's title column.

~> **Note:** Destroying an entry archives it in Notion rather than permanently deleting it.

## Example Usage

```terraform
resource "notion_database_entry" "first_task" {
  database = notion_database.tasks.id
  title    = "Set up Terraform"
}
```

## Schema

### Required

- `database` (String) The ID of the parent database. Changing this forces a new resource.
- `title` (String) The title of the entry (value of the title column).

### Read-Only

- `id` (String) The ID of the entry.
- `url` (String) The URL of the entry in Notion.

## Import

Database entries can be imported using their Notion page ID:

```shell
terraform import notion_database_entry.first_task <entry-id>
```
