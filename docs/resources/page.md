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

# Native Notion icon, locked in the Notion app, created at the top of its parent
resource "notion_page" "runbook" {
  parent_page_id = "your-parent-page-id"
  title          = "On-call runbook"
  icon_name      = "pizza"
  icon_color     = "blue"
  is_locked      = true
  position       = "page_start"
}
```

## Icons

Set at most one of `icon` (an emoji), `icon_name` with optional `icon_color` (a native Notion icon, API 2026-03-25), `icon_url` (an external image), or `icon_custom_emoji_id` (a custom emoji from the integration's workspace). Removing all of them clears the page icon.

Notion returns native icons by their internal name. Picker names such as `star circle` are accepted on write but read back as the internal name (`token`), which shows as a diff, so configure the internal name. URLs from Notion's own icon set, such as `https://www.notion.so/icons/pizza_blue.svg`, are rejected in `icon_url` because Notion stores them as native icons. Use `icon_name` and `icon_color` instead.

A native icon, image, or custom emoji someone sets in the Notion UI on a page that doesn't configure one is left alone. An emoji works differently, because `icon` defaults to an empty string. Terraform shows a UI-set emoji as a diff and removes it on the next apply, so set `icon` to the emoji if you want to keep it. `notion_database` doesn't have this default and leaves a UI-set emoji alone.

## Schema

### Required

- `parent_page_id` (String) The ID of the parent page. Changing this on an
  existing resource issues a `POST /v1/pages/{id}/move` (2026-01-15 endpoint)
  rather than recreating the resource.
- `title` (String) The title of the page.

### Optional

- `icon` (String) Emoji icon for the page. Defaults to an empty string, so an emoji set in the Notion UI is removed on the next apply unless it's configured here. Conflicts with the other icon attributes.
- `icon_name` (String) Native Notion icon name, such as `pizza`. Use the internal name Notion returns.
- `icon_color` (String) Color of the native icon: `gray` (default), `lightgray`, `brown`, `yellow`, `orange`, `green`, `blue`, `purple`, `pink`, or `red`. Requires `icon_name`.
- `icon_url` (String) External image URL for the icon. Notion icon-set URLs are rejected.
- `icon_custom_emoji_id` (String) ID of a custom emoji in the integration's workspace.
- `is_locked` (Boolean) Whether the page is locked from editing in the Notion app. It does not block API updates, so Terraform can still change a locked page. When unset, the current value is reported and left alone.
- `position` (String) Where to create the page in its parent: `page_start` or `page_end` (API 2026-01-15). Only used at creation. Changing it later stores the new value with a warning and does not move the page.
- `after_block_id` (String) Create the page directly after this block in the parent page. Conflicts with `position`. Only used at creation.
- `markdown` (String) Page content as enhanced markdown. Full-rewrite semantics
  (`replace_content`). Mutually exclusive with managing content via
  `notion_block` resources. Mutually exclusive with `template_id`.
- `template_id` (String) Notion template page ID to apply at creation
  (2026-01-15 API addition). The template is applied asynchronously, so the
  page is initially returned blank. Changing this forces a new resource.
- `template_timezone` (String) Optional IANA timezone (e.g.
  `America/New_York`) used when resolving template variables. Only meaningful
  when `template_id` is set. Changing it forces a new resource.
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
