---
page_title: "notion_page Resource - Notion"
subcategory: ""
description: |-
  Manages a Notion page.
---

# notion_page (Resource)

Manages a Notion page. Pages can be created under an existing parent page.

~> **Note:** Destroying a page archives it in Notion rather than permanently deleting it, as the Notion API does not support hard deletes.

## Example Usage

```terraform
resource "notion_page" "example" {
  parent_page_id = "your-parent-page-id"
  title          = "My Terraform Page"
}

# Append a chunk of markdown without rewriting existing content
resource "notion_page" "with_appended_note" {
  parent_page_id = "your-parent-page-id"
  title          = "Release Notes"
  markdown       = "# Release Notes\n\nInitial body."

  markdown_insert = {
    content  = "\n## 2026-06-15\n\n- Shipped feature X."
    position = "end"
  }
}
```

## Schema

### Required

- `parent_page_id` (String) The ID of the parent page. Changing this forces a new resource.
- `title` (String) The title of the page.

### Optional

- `icon` (String) Emoji icon for the page.
- `markdown` (String) Page content as enhanced markdown. Full-rewrite semantics
  (`replace_content`). Mutually exclusive with managing content via
  `notion_block` resources.
- `markdown_insert` (Object) Append or prepend markdown to the page without
  rewriting the existing content (uses the 2026-05-15 `insert_content.position`
  endpoint). Each change to `content` or `position` triggers another insert —
  this is an imperative trigger, not declarative state. Removing the block does
  not remove the previously inserted content. Fields:
  - `content` (String, required) Markdown to insert.
  - `position` (String, required) `"start"` (prepend) or `"end"` (append).

### Read-Only

- `id` (String) The ID of the page.
- `url` (String) The URL of the page in Notion.

## Import

Pages can be imported using their Notion page ID:

```shell
terraform import notion_page.example <page-id>
```
