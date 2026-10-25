---
page_title: "notion_permission_groups Data Source - Notion"
subcategory: "Admin API"
description: |-
  Lists the permission groups in a Notion workspace through the Admin API.
---

# notion_permission_groups (Data Source)

Lists a workspace's permission groups, both admin-managed and SCIM-managed, through the [Admin API](https://developers.notion.com/reference/admin/list-permission-groups). Needs `admin_token` with the `permission-group:read` scope. Legacy groups with duplicate names can appear more than once.

## Example Usage

```terraform
data "notion_permission_groups" "all" {
  workspace_id = data.notion_workspaces.org.workspaces[0].id
}

output "group_ids" {
  value = { for g in data.notion_permission_groups.all.groups : g.name => g.id }
}
```

## Schema

### Required

- `workspace_id` (String) The workspace to list groups in.

### Read-Only

- `groups` (List of Object) Permission groups in the workspace.
  - `id` (String) The group ID.
  - `name` (String) The group name.
