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
  icon_name          = "token"
  is_locked          = true
}

resource "notion_view" "board" {
  database_id    = notion_database.tasks.id
  data_source_id = notion_database.tasks.data_source_ids[0]
  name           = "Board"
  type           = "board"
}
```

## Icons

The icon attributes work like the ones on `notion_page`. The difference is `icon`, which reports the database's current emoji when it isn't configured and leaves it alone, so existing configs keep working. Set at most one of `icon`, `icon_name` (with optional `icon_color`), `icon_url`, or `icon_custom_emoji_id`.

Icon, `is_locked`, and `show_data_source_title` are written and read on the database itself over Notion API version 2026-03-11, and `database_type` and `data_source_ids` come from the same response. Schema changes stay on the 2022-06-28 path, so a refresh makes two requests per database.

## Data source title

`show_data_source_title` is the **Show data source title** setting in the Notion app, which shows or hides the title above the database's views. Notion turns it on for new databases. A full-page database with one data source never shows the title, whatever the setting says, but the setting is still stored and read back, so setting it there doesn't cause a diff. When the attribute is unset, the provider reports the current value and leaves it alone.

## Schema

### Required

- `parent` (String) The ID of the parent page. Changing this forces a new resource.
- `title` (String) The title of the database.
- `title_column_title` (String) The name of the title column (every Notion database has one). Can be renamed on existing databases.

### Optional

- `icon` (String) Emoji icon. When unset, the current emoji is reported and left alone. Conflicts with the other icon attributes.
- `icon_name` (String) Native Notion icon name (API 2026-03-25). Use the internal name Notion returns.
- `icon_color` (String) Color of the native icon: `gray` (default), `lightgray`, `brown`, `yellow`, `orange`, `green`, `blue`, `purple`, `pink`, or `red`. Requires `icon_name`.
- `icon_url` (String) External image URL for the icon. Notion icon-set URLs are rejected.
- `icon_custom_emoji_id` (String) ID of a custom emoji in the integration's workspace.
- `is_locked` (Boolean) Whether the database is locked from editing in the Notion app. It does not block API updates. When unset, the current value is reported and left alone.
- `show_data_source_title` (Boolean) Whether the data source title is shown above the database's views (API 2026-10-20). Notion defaults it to `true` on create. Full-page databases with one data source never show the title. When unset, the current value is reported and left alone.

### Read-Only

- `id` (String) The ID of the database.
- `title_column_id` (String) The ID of the title column.
- `url` (String) The URL of the database in Notion.
- `database_type` (String) The typed database kind Notion reports: `tasks`, `projects`, `sprints`, `docs`, `wiki`, `meetings`, `meeting_notes`, `skills`, `github_prs`, or another kind Notion adds later. Null for a regular database. The provider doesn't create typed databases.
- `data_source_ids` (List of String) IDs of the database's data sources, without hyphens, in the order Notion returns them. A new database has one. Use `data_source_ids[0]` as `data_source_id` on `notion_view`.

## Import

Databases can be imported using their Notion database ID:

```shell
terraform import notion_database.tasks <database-id>
```
