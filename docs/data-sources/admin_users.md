---
page_title: "notion_admin_users Data Source - Notion"
subcategory: "Admin API"
description: |-
  Lists a Notion workspace's members and guest bots through the Admin API.
---

# notion_admin_users (Data Source)

Lists a workspace's active members, including restricted members, and guest bots through the [Admin API](https://developers.notion.com/reference/admin/list-users). Unlike `notion_users`, it covers the whole workspace rather than only the users the integration can see. Page guests and expired temporary memberships aren't included. Needs `admin_token` with the `user:read` scope.

## Example Usage

```terraform
data "notion_admin_users" "org" {
  workspace_id = data.notion_workspaces.org.workspaces[0].id
}

output "workspace_owners" {
  value = [for u in data.notion_admin_users.org.users : u.email if u.membership_type == "owner"]
}
```

## Schema

### Required

- `workspace_id` (String) The workspace to list users in.

### Read-Only

- `users` (List of Object) People and bots in the workspace.
  - `id` (String) The user ID.
  - `name` (String) The user's name, or null.
  - `type` (String) `person` or `bot`.
  - `membership_type` (String) For people, the effective workspace membership: `none`, `page_guest`, `restricted_member`, `member`, `membership_admin`, or `owner`. Null for bots.
  - `email` (String) For people, the email address when Notion has one. Null for bots.
