---
page_title: "notion_custom_emojis Data Source - Notion"
subcategory: ""
description: |-
  List the workspace's custom emojis.
---

# notion_custom_emojis (Data Source)

Lists the custom emojis in the integration's workspace through [`GET /v1/custom_emojis`](https://developers.notion.com/reference/list-custom-emojis) (Notion API 2026-03-25), paging through every result. Use `name` to resolve one custom emoji name to its ID. Since 2026-09-25 Notion rejects a `custom_emoji` icon that doesn't name an emoji in the workspace, so looking the ID up here avoids that error.

## Example Usage

```terraform
data "notion_custom_emojis" "parrot" {
  name = "party parrot"
}

output "parrot_id" {
  value = one(data.notion_custom_emojis.parrot.emojis[*].id)
}

# Every custom emoji in the workspace
data "notion_custom_emojis" "all" {}
```

## Schema

### Optional

- `name` (String) Exact name to match. Omit to list every custom emoji.

### Read-Only

- `emojis` (Attributes List) Matching custom emojis. (see [below for nested schema](#nestedatt--emojis))

<a id="nestedatt--emojis"></a>
### Nested Schema for `emojis`

Read-Only:

- `id` (String) The custom emoji ID.
- `name` (String) The custom emoji name.
- `url` (String) The URL of the custom emoji image.
