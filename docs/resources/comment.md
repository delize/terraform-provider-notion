---
page_title: "notion_comment Resource - Notion"
subcategory: ""
description: |-
  Manages a comment on a Notion page or within an existing discussion thread.
---

# notion_comment (Resource)

Manages a comment on a Notion page or as a reply within an existing discussion thread.

## Notion API constraints worth knowing

- **Integration capability:** the integration token must have **"Insert comments"** capability enabled, and to use update/delete it also needs **"Update comments"** / **"Delete comments"**. Configure these in your integration's settings.
- **Update/Delete are owner-restricted:** the [Notion docs](https://developers.notion.com/reference/update-a-comment) state that a connection can only update or delete comments it created. Attempting to update or delete a comment created by another user/integration returns 404.
- **Markdown is inline-only:** when using `markdown`, only inline formatting renders — bold, italic, strikethrough, code, links, mentions, inline equations. Fenced code blocks, headings, lists, tables, and blockquotes are *not* rendered as structured blocks in comments per the API docs.
- **Endpoints not yet in the upstream Go SDK:** `Update` (`PATCH /v1/comments/{id}`), `Delete` (`DELETE /v1/comments/{id}`), and the `markdown` body parameter shipped in April 2026, after the `jomei/notionapi` SDK's last release. This provider implements them via a small in-tree HTTP shim using `Notion-Version: 2026-03-11`.

## Example Usage

```terraform
# A new top-level comment on a page, with markdown body
resource "notion_comment" "review" {
  parent_page_id = "abcd1234abcd1234abcd1234abcd1234"
  markdown       = "Looks great! Two small **nits**: see [doc](https://example.com)."
}

# A plain-text comment using rich_text instead
resource "notion_comment" "ack" {
  parent_page_id = notion_page.deploy_runbook.id
  rich_text      = "Acknowledged, deploying now."
}

# A reply within an existing discussion thread
resource "notion_comment" "followup" {
  discussion_id = "ef01ef01ef01ef01ef01ef01ef01ef01"
  markdown      = "Following up — fixed in `v1.2.3`."
}
```

## Schema

### Required

One of `parent_page_id` or `discussion_id` must be set; the other must be omitted. Likewise one of `rich_text` or `markdown` must be set.

### Optional

- `parent_page_id` (String, ForceNew) ID of the page to comment on. Creates a new discussion thread on that page.
- `discussion_id` (String, ForceNew) ID of an existing discussion thread to reply to.
- `rich_text` (String) Plain-text comment body. Mutually exclusive with `markdown`.
- `markdown` (String) Markdown comment body. Inline formatting only — see constraints above.

### Read-Only

- `id` (String) The comment ID.
- `plain_text` (String) Server-rendered plain-text representation of the body.
- `anchor_block_id` (String) Block (page) the discussion is anchored to. Used internally for refresh.
- `created_time` (String) RFC3339 timestamp of when the comment was created.
- `last_edited_time` (String) RFC3339 timestamp of the most recent edit.
- `created_by` (String) ID of the user/bot that created the comment.

## Import

```shell
terraform import notion_comment.example <comment-id>
```

Note: imported comments without a known anchor (page or block) won't refresh on subsequent `terraform plan` runs until `anchor_block_id` is populated. Importing comments created on pages: also set `parent_page_id` in your config.
