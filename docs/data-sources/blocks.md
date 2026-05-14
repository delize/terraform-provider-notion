---
page_title: "notion_blocks Data Source - Notion"
subcategory: ""
description: |-
  List the immediate child blocks of a Notion page or block.
---

# notion_blocks (Data Source)

List the immediate child blocks of a Notion page or block. Wraps the [`/v1/blocks/{id}/children`](https://developers.notion.com/reference/get-block-children) endpoint and paginates through all results.

To traverse nested blocks, use a separate `notion_blocks` data source for each block whose `has_children` is `true`.

## Example Usage

```terraform
data "notion_blocks" "page_contents" {
  parent_id = "abcd1234abcd1234abcd1234abcd1234"
}

output "headings" {
  value = [
    for b in data.notion_blocks.page_contents.blocks :
    b.plain_text if startswith(b.type, "heading_")
  ]
}
```

## Schema

### Required

- `parent_id` (String) The ID of the page or block whose immediate children should be listed.

### Read-Only

- `blocks` (Attributes List) Immediate children of `parent_id`, in document order. (see [below for nested schema](#nestedatt--blocks))

<a id="nestedatt--blocks"></a>
### Nested Schema for `blocks`

Read-Only:

- `id` (String) The block ID.
- `type` (String) The block type (e.g. `paragraph`, `heading_1`, `code`, `image`).
- `has_children` (Boolean) Whether this block has nested children.
- `plain_text` (String) Best-effort plain-text representation. Empty for blocks without textual content (dividers, images, etc.).
- `archived` (Boolean) Whether the block is archived.
