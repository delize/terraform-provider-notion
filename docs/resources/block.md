---
page_title: "notion_block Resource - Notion"
subcategory: ""
description: |-
  Manages a content block on a Notion page.
---

# notion_block (Resource)

Manages a content block on a Notion page or inside another block. Supports paragraphs, headings, lists, code blocks, callouts, equations, synced blocks, columns, and more.

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

## Schema

### Required

- `parent_id` (String) The ID of the parent page or block. Changing this forces a new resource.
- `type` (String) The block type. Changing this forces a new resource. Valid values: `paragraph`, `heading_1`, `heading_2`, `heading_3`, `bulleted_list_item`, `numbered_list_item`, `to_do`, `toggle`, `quote`, `callout`, `code`, `equation`, `divider`, `table_of_contents`, `bookmark`, `embed`, `image`, `synced_block`, `column_list`, `column`.

### Optional

- `after` (String) Insert after this block ID. Changing this forces a new resource.
- `rich_text` (String) Plain text content of the block.
- `color` (String) Block color (e.g. `default`, `red`, `blue_background`).
- `is_toggleable` (Boolean) Whether a heading block is toggleable.
- `checked` (Boolean) Whether a to-do block is checked.
- `icon` (String) Emoji icon for callout blocks.
- `language` (String) Programming language for code blocks.
- `caption` (String) Caption text for code, bookmark, and image blocks.
- `url` (String) URL for bookmark, embed, and image blocks.
- `expression` (String) LaTeX expression for equation blocks.
- `synced_from` (String) Source block ID for synced block copies. Changing this forces a new resource.

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

## Import

Blocks can be imported using their Notion block ID:

```shell
terraform import notion_block.example <block-id>
```
