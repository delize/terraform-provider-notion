---
page_title: "notion_revoke_user_sessions Action - Notion"
subcategory: "Admin API"
description: |-
  Revokes all active sessions of a managed Notion user.
---

# notion_revoke_user_sessions (Action)

Revokes every active session of a managed user, signing them out of Notion, through the [Admin API](https://developers.notion.com/reference/admin/revoke-user-session). Useful in offboarding. Needs Terraform 1.14 or later and `admin_token` with the `managed-user-session:write` scope. It runs only when invoked.

Only managed users are covered, meaning accounts on the organization's verified domains. The [Help Center](https://www.notion.com/help/admin-apis-for-enterprise-organizations) notes that a logout call for any other account does nothing, so a successful run against an outside account signs no one out.

## Example Usage

```terraform
action "notion_revoke_user_sessions" "ada" {
  config {
    email = "ada@example.com"
  }
}
```

```shell
terraform apply -invoke=action.notion_revoke_user_sessions.ada
```

## Schema

### Optional

Set exactly one of these.

- `user_id` (String) The managed user's Notion user ID.
- `email` (String) The managed user's primary email address.
