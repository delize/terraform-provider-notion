---
page_title: "notion_database_templates Data Source - Notion"
subcategory: ""
description: |-
  List the page templates of a database's data source.
---

# notion_database_templates (Data Source)

Lists the page templates available in a database through [List data source templates](https://developers.notion.com/reference/list-data-source-templates) (Notion API 2026-01-15). Use it to find the `template_id` for [`notion_page`](../resources/page.md) instead of copying IDs out of Notion.

The endpoint works on data sources, which Notion added in API version 2025-09-03. Set `database_id` and the data source is looked up for you, which works when the database has exactly one data source. For a database with several, the error lists them so you can set `data_source_id` directly.

## Example Usage

```terraform
data "notion_database_templates" "meetings" {
  database_id = notion_database.meetings.id
  name        = "weekly"
}

resource "notion_page" "this_week" {
  parent_page_id = notion_page.team.id
  title          = "Weekly sync"
  template_id    = one([for t in data.notion_database_templates.meetings.templates : t.id if t.name == "Weekly sync"])
}

# The default template, if the data source has one
output "default_template" {
  value = data.notion_database_templates.meetings.default_template_id
}
```

## Schema

### Optional

- `database_id` (String) The database to list templates for. Set this or `data_source_id`.
- `data_source_id` (String) The data source to list templates for. Set this or `database_id`. When `database_id` is set, this reports the data source that was used.
- `name` (String) Filter on template name. Notion matches it as a case-insensitive substring.

### Read-Only

- `default_template_id` (String) ID of the default template, or null when none of the returned templates is the default.
- `templates` (Attributes List) Templates the integration can access. Sharing the database with the integration generally grants access to its templates. (see [below for nested schema](#nestedatt--templates))

<a id="nestedatt--templates"></a>
### Nested Schema for `templates`

Read-Only:

- `id` (String) The template page ID.
- `name` (String) The template name.
- `is_default` (Boolean) Whether this is the data source's default template.
