---
page_title: "notion_permission_group_member Resource - Notion"
subcategory: "Admin API"
description: |-
  Manages a direct user membership in a Notion permission group through the Admin API.
---

# notion_permission_group_member (Resource)

Manages one direct user membership in an admin-managed permission group through the [Admin API](https://developers.notion.com/reference/admin/intro). This needs an Enterprise plan and `admin_token` with the `permission-group:read` and `permission-group:write` scopes.

The user must be a workspace member. Guests aren't supported, and restricted members can't be group owners. Memberships of SCIM-managed groups have to be changed in the identity provider. Nested-group memberships aren't exposed by the API, so they aren't managed here.

## Example Usage

```terraform
data "notion_admin_users" "org" {
  workspace_id = notion_permission_group.engineering.workspace_id
}

locals {
  users_by_email = { for u in data.notion_admin_users.org.users : u.email => u.id if u.email != null }
}

resource "notion_permission_group_member" "ada" {
  workspace_id = notion_permission_group.engineering.workspace_id
  group_id     = notion_permission_group.engineering.id
  user_id      = local.users_by_email["ada@example.com"]
  role         = "member"
}
```

## Schema

### Required

- `workspace_id` (String) The ID of the workspace. Changing this forces a new membership.
- `group_id` (String) The ID of the permission group. Changing this forces a new membership.
- `user_id` (String) The ID of the user. Changing this forces a new membership.
- `role` (String) The user's role in the group, `owner` or `member`. Changes in place.

### Read-Only

- `id` (String) Composite ID in the form `<workspace_id>/<group_id>/<user_id>`.

## Import

```shell
terraform import notion_permission_group_member.ada <workspace-id>/<group-id>/<user-id>
```
