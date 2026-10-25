---
page_title: "notion_personal_access_tokens Data Source - Notion"
subcategory: "Admin API"
description: |-
  Lists personal access token records in a Notion workspace.
---

# notion_personal_access_tokens (Data Source)

Lists personal access token (PAT) records in a workspace through the [Admin API](https://developers.notion.com/reference/admin/list-personal-access-tokens). Token values are never returned. Needs `admin_token` with the `personal-access-token:read` scope.

## Example Usage

```terraform
data "notion_personal_access_tokens" "active" {
  workspace_id = data.notion_workspaces.org.workspaces[0].id
  statuses     = ["active"]
}

output "pats_never_used" {
  value = [for t in data.notion_personal_access_tokens.active.tokens : "${t.name} (${t.creator_email})" if t.last_used_at == null]
}
```

## Schema

### Required

- `workspace_id` (String) The workspace.

### Optional

- `statuses` (List of String) Only these statuses: `active`, `expired`, `revoked`.
- `creator_ids` (List of String) Only tokens created by these members.
- `search` (String) Case-insensitive match on the token name.

### Read-Only

- `tokens` (List of Object) Each with `id` (pass it as `bot_id` to `notion_personal_access_token_revoke`), `name`, `status`, `creator_id`, `creator_name`, `creator_email`, `created_at`, `expires_at`, `last_used_at` (updated at most hourly), `revoked_at`, and `revoked_by_id`. Times are epoch milliseconds.
