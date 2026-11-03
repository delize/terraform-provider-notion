---
page_title: "notion_block Resource - Notion"
subcategory: ""
description: |-
  Manages a content block on a Notion page.
---

# notion_block (Resource)

Manages a content block on a Notion page or inside another block. Supports paragraphs, headings (including `heading_4`), lists, code blocks, callouts, equations, synced blocks, columns, tabs, and more.

~> **Note:** Destroying a block archives it in Notion. Blocks cannot be moved after creation — changing `parent_id` or `after` forces replacement.

## Example Usage

### Basic Text Blocks

```terraform
resource "notion_block" "heading" {
  parent_id = notion_page.my_page.id
  type      = "heading_1"
  rich_text = "Welcome"
}

resource "notion_block" "intro" {
  parent_id = notion_page.my_page.id
  type      = "paragraph"
  rich_text = "Hello, world!"
  after     = notion_block.heading.id
}
```

### To-Do List

```terraform
resource "notion_block" "task" {
  parent_id = notion_page.my_page.id
  type      = "to_do"
  rich_text = "Buy groceries"
  checked   = true
}
```

### Code Block

```terraform
resource "notion_block" "snippet" {
  parent_id = notion_page.my_page.id
  type      = "code"
  rich_text = "print('hello')"
  language  = "python"
}
```

### Callout

```terraform
resource "notion_block" "note" {
  parent_id = notion_page.my_page.id
  type      = "callout"
  rich_text = "Important information"
  icon      = "💡"
  color     = "yellow_background"
}
```

A callout icon can also be a native Notion icon (Notion API 2026-03-25), an external image, or a custom emoji. Set at most one of `icon`, `icon_name`, `icon_url`, and `icon_custom_emoji_id`. These work the same way as on `notion_page`.

```terraform
resource "notion_block" "tip" {
  parent_id  = notion_page.my_page.id
  type       = "callout"
  rich_text  = "Pizza Friday"
  icon_name  = "pizza"
  icon_color = "blue"
}
```

Each icon kind is only tracked once it's configured. A callout with no icon attribute in the config ignores whatever icon Notion shows, such as the 💡 the Notion app gives new callouts, so it never plans a change because of its icon. Once an icon is configured, a change made in the Notion app shows as drift and the next apply puts the configured icon back.

Notion's API can set or change a callout icon but can't remove it. Removing a configured icon from the config fails before anything is sent. Remove the icon in the Notion app first, then from the config, or configure a different icon.

Importing a callout takes the emoji Notion has into `icon`, so write that emoji in the config. If the imported callout has an emoji and the config leaves `icon` out, the next apply counts it as removing the icon and fails as above.

An empty `color` is sent as `default`.

### Markdown Links

The `rich_text` attribute supports markdown link syntax. Links render as clickable hyperlinks in Notion.

```terraform
resource "notion_block" "with_link" {
  parent_id = notion_page.my_page.id
  type      = "paragraph"
  rich_text = "See the [Notion API docs](https://developers.notion.com) for details."
}
```

### JSON Rich Text (Advanced)

For full control over text formatting (bold, italic, color, links), use `rich_text_json` instead of `rich_text`. It accepts a JSON-encoded array of Notion rich text objects and takes precedence over `rich_text` when set.

```terraform
resource "notion_block" "fancy" {
  parent_id = notion_page.my_page.id
  type      = "paragraph"
  rich_text_json = jsonencode([
    { type = "text", text = { content = "Click " } },
    {
      type = "text",
      text = { content = "here", link = { url = "https://example.com" } },
      annotations = { bold = true }
    },
    { type = "text", text = { content = " for details." } }
  ])
}
```

Notion answers with more than the request holds. Each item comes back with `plain_text`, `href`, and the full `annotations` object. The provider compares only what a request can set (the type, `text.content`, `text.link.url`, an equation's expression, the mention fields you write, and the annotations), so the string you configured stays in state as written, key order and whitespace included. Annotations you leave out must still be at Notion's defaults, so bold or color added in the Notion app shows as a diff. When the text really differs, state holds Notion's version in request shape, without `plain_text`, `href`, or default annotations. Mention IDs match with or without hyphens, and a date mention matches the same date in another format.

While `rich_text_json` is set, `rich_text` isn't read from Notion and keeps its configured value (`""` unless set). An import can't tell which of the two the config uses, so it fills `rich_text` and leaves `rich_text_json` empty. The first plan after importing a block managed with `rich_text_json` then shows an update that rewrites the same text.

### Columns

```terraform
resource "notion_block" "cols" {
  parent_id = notion_page.my_page.id
  type      = "column_list"
}

resource "notion_block" "col1" {
  parent_id = notion_block.cols.id
  type      = "column"
}

resource "notion_block" "col1_text" {
  parent_id = notion_block.col1.id
  type      = "paragraph"
  rich_text = "Left column"
}
```

### Heading 4

`heading_4` works like the other heading levels (Notion API 2026-03-30).

```terraform
resource "notion_block" "subsection" {
  parent_id     = notion_page.my_page.id
  type          = "heading_4"
  rich_text     = "Details"
  is_toggleable = true
}
```

### Tabs

A `tab` block is a tabbed container (Notion API 2026-03-25). List its tabs in `tab_items`, each with a label and an optional emoji icon. Notion stores each tab as a paragraph block, and its `id` is exported, so add a tab's content with `notion_block` resources whose `parent_id` is that tab's `id`.

```terraform
resource "notion_block" "tabs" {
  parent_id = notion_page.my_page.id
  type      = "tab"
  tab_items = [
    { label = "Overview", icon = "📋" },
    { label = "Details" },
  ]
}

resource "notion_block" "overview_text" {
  parent_id = notion_block.tabs.tab_items[0].id
  type      = "paragraph"
  rich_text = "Shown in the Overview tab."
}
```

Tabs are matched by position. Changing a label or icon updates that tab in place, and new entries are added at the end. Removing entries from the end deletes those tabs along with their content. Reordering is a relabel, so the content stays with the position, not the label. A tab block needs at least one tab, and `rich_text`, `rich_text_json`, `color`, and `icon` don't apply to the container itself.

## Schema

### Required

- `parent_id` (String) The ID of the parent page or block. Changing this forces a new resource.
- `type` (String) The block type. Changing this forces a new resource. Valid values: `paragraph`, `heading_1`, `heading_2`, `heading_3`, `heading_4`, `bulleted_list_item`, `numbered_list_item`, `to_do`, `toggle`, `quote`, `callout`, `code`, `equation`, `divider`, `table_of_contents`, `bookmark`, `embed`, `image`, `synced_block`, `column_list`, `column`, `tab`.

### Optional

- `after` (String) Insert after this block ID. Changing this forces a new resource.
- `rich_text` (String) Text content of the block. Supports markdown links: `[text](url)`.
- `color` (String) Block color: `default`, `gray`, `brown`, `orange`, `yellow`, `green`, `blue`, `purple`, `pink`, `red`, or one of the `_background` variants such as `default_background` or `blue_background`. Leave it unset for Notion's default color. Notion reports an unset color as `default`, which reads back as `""` so it doesn't show as a diff, and an import of an uncolored block reads `""` too. Removing `color` from the config resets the block to `default`.
- `is_toggleable` (Boolean) Whether a heading block (`heading_1` to `heading_4`) is toggleable.
- `checked` (Boolean) Whether a to-do block is checked.
- `icon` (String) Emoji icon for callout blocks. Only tracked once configured, and removing a configured icon is an error because the API can't remove it. Conflicts with `icon_name`, `icon_url`, and `icon_custom_emoji_id`.
- `icon_name` (String) Native Notion icon name for a callout, such as `pizza` (2026-03-25). Notion returns its internal icon name, so use that rather than a picker alias to avoid a diff. Only valid when `type` is `callout`.
- `icon_color` (String) Color of the native icon set with `icon_name`: `gray`, `lightgray`, `brown`, `yellow`, `orange`, `green`, `blue`, `purple`, `pink`, or `red`. Notion defaults to `gray`. Only valid when `type` is `callout`.
- `icon_url` (String) External image URL for a callout icon. URLs from Notion's own icon set are rejected, use `icon_name` for those. Only valid when `type` is `callout`.
- `icon_custom_emoji_id` (String) ID of a custom emoji in the integration's workspace to use as a callout icon. See the `notion_custom_emojis` data source. Only valid when `type` is `callout`.
- `language` (String) Programming language for code blocks.
- `caption` (String) Caption text for code, bookmark, and image blocks.
- `url` (String) URL for bookmark, embed, and image blocks.
- `expression` (String) LaTeX expression for equation blocks.
- `rich_text_json` (String) JSON-encoded array of Notion rich text objects. When set, takes precedence over `rich_text`, which then keeps its configured value. Read back by comparing the fields a request sets, so Notion's added `plain_text`, `href`, and default annotations don't show as a diff. See [JSON Rich Text](#json-rich-text-advanced) and [Notion Rich Text API](https://developers.notion.com/reference/rich-text) for the object format.
- `synced_from` (String) Source block ID for synced block copies. Changing this forces a new resource.
- `tab_items` (Attributes List) Tabs of a `tab` block, in order. Required for `tab` and not allowed on other types. Each entry has:
  - `label` (String, Required) Tab label. Supports markdown links.
  - `icon` (String, Optional) Emoji shown next to the label. A native Notion icon set in the app reads back as `""`. Notion's API can set or change a tab icon but can't remove it, so removing `icon` from a tab that has one fails before anything is sent. Remove the icon in the Notion app first, then from the config.
  - `id` (String, Read-Only) Block ID of the tab. Use it as `parent_id` for the tab's content.

### Read-Only

- `id` (String) The ID of the block.
- `has_children` (Boolean) Whether this block has child blocks.

## Supported Block Types

| Block Type | Notion UI Name | Relevant Fields |
|-----------|---------------|-----------------|
| `paragraph` | Text | rich_text, color |
| `heading_1` | Heading 1 | rich_text, color, is_toggleable |
| `heading_2` | Heading 2 | rich_text, color, is_toggleable |
| `heading_3` | Heading 3 | rich_text, color, is_toggleable |
| `heading_4` | Heading 4 | rich_text, color, is_toggleable |
| `bulleted_list_item` | Bulleted list | rich_text, color |
| `numbered_list_item` | Numbered list | rich_text, color |
| `to_do` | To-do list | rich_text, checked, color |
| `toggle` | Toggle list | rich_text, color |
| `quote` | Quote | rich_text, color |
| `callout` | Callout | rich_text, icon, icon_name, icon_color, icon_url, icon_custom_emoji_id, color |
| `code` | Code | rich_text, language, caption |
| `equation` | Block equation | expression |
| `divider` | Divider | (none) |
| `table_of_contents` | Table of contents | color |
| `bookmark` | Bookmark | url, caption |
| `embed` | Embed | url |
| `image` | Image | url, caption |
| `synced_block` | Synced block | synced_from |
| `column_list` | Columns container | (none) |
| `column` | Column | (none) |
| `tab` | Tabs | tab_items |

## Import

Blocks can be imported using their Notion block ID:

```shell
terraform import notion_block.example <block-id>
```
