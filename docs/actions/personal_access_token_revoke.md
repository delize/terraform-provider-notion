---
page_title: "notion_personal_access_token_revoke Action - Notion"
subcategory: "Admin API"
description: |-
  Permanently revokes a personal access token in a Notion workspace.
---

# notion_personal_access_token_revoke (Action)

Permanently revokes a personal access token through the [Admin API](https://developers.notion.com/reference/admin/revoke-personal-access-token). The token can no longer authenticate API requests or access Notion Workers. Revoking an already revoked token is a no-op. Needs Terraform 1.14 or later and `admin_token` with the `personal-access-token:write-high-impact` scope. It runs only when invoked.

## Example Usage

```terraform
action "notion_personal_access_token_revoke" "leaked" {
  config {
    workspace_id = data.notion_workspaces.org.workspaces[0].id
    bot_id       = "00000000-0000-0000-0000-000000000000"
  }
}
```

```shell
terraform apply -invoke=action.notion_personal_access_token_revoke.leaked
```

## Schema

### Required

- `workspace_id` (String) The workspace.
- `bot_id` (String) The token's `id` from `notion_personal_access_tokens`.
