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

# Notion-flavored Markdown (v2). Separate blocks with a blank line.
resource "notion_page" "handbook" {
  parent_page_id   = "your-parent-page-id"
  title            = "Handbook"
  markdown_version = "v2"
  markdown         = "## Welcome\n\n<toggle>\nRead this first\n\nDetails inside the toggle.\n</toggle>"
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

## Markdown versions

Notion accepts two markdown syntaxes. `v1` is [enhanced markdown](https://developers.notion.com/guides/data-apis/enhanced-markdown), the original format, which Notion has deprecated. `v2` is [Notion-flavored Markdown](https://developers.notion.com/guides/data-apis/notion-flavored-markdown), which Notion recommends. The next Notion API version will make `v2` the default and reject `v1` with a 400 error, and Notion plans to retire `v1` about three months after that version ships.

Set `markdown_version` on the resource, or on the provider block to cover every page and database entry at once. The resource value wins. When neither is set, the provider leaves the field out of its requests and Notion uses `v1`, which is how earlier versions of the provider behaved. The `notion_page_markdown` data source has its own `markdown_version` and otherwise follows the provider block.

The same text can mean different things in the two syntaxes, so moving to `v2` means rewriting the markdown string, not only setting the attribute. Notion's [migration guide](https://developers.notion.com/guides/data-apis/migrate-to-notion-flavored-markdown) lists every change. These are the ones most likely to affect a config.

- In `v2` a single newline doesn't start a new block. Separate blocks with a blank line, or the lines join into one paragraph.
- Children are no longer indented with tabs, except in lists. A block with children uses its tag form, such as `<toggle>` or `<quote>`, with the children inside the tag after a blank line.
- `{color="blue"}` suffixes become a `color` attribute on the block's tag. `v2` saves the old suffix as literal text and doesn't warn about it.
- `<br>` becomes `<br />`, `<details>` becomes `<toggle>`, and several other block tags change name or attributes.
- `v2` rejects input it can't turn into blocks with a 400 error whose message starts with `Markdown errors blocked parsing`. `v1` saved the same input as text.

Change `markdown_version` and the `markdown` string in the same apply. A change to the effective version, whether it comes from the resource or the provider block, replaces the page body once with the current `markdown`, even when the text itself didn't change. Going from unset to an explicit `"v1"`, or back, isn't a change, since both mean `v1`. Setting a provider default shows a plan diff only on resources that set `markdown`.

When Notion accepts the markdown but changes or ignores part of it, for example converting a `v1` tag or saving an unknown tag as text, each of Notion's warnings shows up as a Terraform warning. The apply still succeeds.

### When the body is rewritten

The page body is replaced (Notion's `replace_content`) only when `markdown` changes or the effective `markdown_version` changes. Updating anything else leaves the body alone. Earlier versions of the provider resent the markdown on every update, so a title change also overwrote edits made in the Notion UI.

A replace overwrites everything on the page with the configured markdown, including edits made in the Notion UI. Child pages and databases that aren't in the markdown can be deleted, because the provider allows Notion to delete content during a replace. Replacing content may also drop pending suggested edits on the page. Notion doesn't document this either way.

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
- `markdown` (String) Page content as markdown in the syntax set by `markdown_version`. Full-rewrite semantics (`replace_content`), sent only when the markdown or its version changes. Mutually exclusive with managing content via `notion_block` resources. Mutually exclusive with `template_id`.
- `markdown_version` (String) Markdown syntax for `markdown` and `markdown_insert`: `v2` (Notion-flavored Markdown) or `v1` (enhanced markdown, deprecated). Overrides the provider's `markdown_version`. When neither is set, the field is left out and Notion uses `v1`. Records the provider default when only that is set. Changing the effective version replaces the page body once. See [Markdown versions](#markdown-versions).
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
  not remove the previously inserted content. Other updates, such as a title change, don't insert again (earlier provider versions re-inserted on every update). Fields:
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
