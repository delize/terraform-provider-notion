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
- `color` (String) Block color (e.g. `default`, `red`, `blue_background`).
- `is_toggleable` (Boolean) Whether a heading block (`heading_1` to `heading_4`) is toggleable.
- `checked` (Boolean) Whether a to-do block is checked.
- `icon` (String) Emoji icon for callout blocks.
- `language` (String) Programming language for code blocks.
- `caption` (String) Caption text for code, bookmark, and image blocks.
- `url` (String) URL for bookmark, embed, and image blocks.
- `expression` (String) LaTeX expression for equation blocks.
- `rich_text_json` (String) JSON-encoded array of Notion rich text objects. When set, takes precedence over `rich_text`. See [Notion Rich Text API](https://developers.notion.com/reference/rich-text) for the object format.
- `synced_from` (String) Source block ID for synced block copies. Changing this forces a new resource.
- `tab_items` (Attributes List) Tabs of a `tab` block, in order. Required for `tab` and not allowed on other types. Each entry has:
  - `label` (String, Required) Tab label. Supports markdown links.
  - `icon` (String, Optional) Emoji shown next to the label. A native Notion icon set in the app reads back as `""`.
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
| `callout` | Callout | rich_text, icon, color |
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
