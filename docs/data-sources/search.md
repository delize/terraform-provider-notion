---
page_title: "notion_search Data Source - Notion"
subcategory: ""
description: |-
  Search the Notion workspace for pages and databases.
---

# notion_search (Data Source)

Search the Notion workspace for pages and databases the integration has access to. Wraps the [`/v1/search`](https://developers.notion.com/reference/post-search) endpoint and paginates through all results.

## Example Usage

```terraform
# Find every page whose title contains "Roadmap"
data "notion_search" "roadmaps" {
  query         = "Roadmap"
  filter_object = "page"
}

# List every database the integration can see
data "notion_search" "all_databases" {
  filter_object = "database"
}

# List pages in the trash
data "notion_search" "trashed_pages" {
  filter_object = "page"
  in_trash      = true
}

output "roadmap_urls" {
  value = [for r in data.notion_search.roadmaps.results : r.url]
}
```

## Schema

### Optional

- `query` (String) Substring to match against page/database titles. Omit to list everything accessible to the integration.
- `filter_object` (String) Restrict results to either `page` or `database`. Omit to return both.
- `in_trash` (Boolean) When `true`, list pages and databases in the trash instead of live ones (Notion API 2026-07-15). Works alone or with `query` and `filter_object`. Defaults to `false`.

### Read-Only

- `results` (Attributes List) All matching pages and databases. (see [below for nested schema](#nestedatt--results))

<a id="nestedatt--results"></a>
### Nested Schema for `results`

Read-Only:

- `id` (String) The Notion ID of the page or database.
- `object` (String) Either `page` or `database`.
- `title` (String) The plain-text title.
- `url` (String) The Notion URL.
- `parent_type` (String) The parent kind (`workspace`, `page_id`, `database_id`, or `block_id`).
- `parent_id` (String) The parent ID. Empty when `parent_type` is `workspace`.
- `archived` (Boolean) Whether the result is archived.
