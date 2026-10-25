---
page_title: "notion_permission_group Resource - Notion"
subcategory: "Admin API"
description: |-
  Manages an admin-managed permission group in a Notion workspace through the Admin API.
---

# notion_permission_group (Resource)

Manages an admin-managed permission group in a Notion workspace through the [Admin API](https://developers.notion.com/reference/admin/intro). This needs an Enterprise plan and `admin_token` in the provider block, with the `permission-group:read` and `permission-group:write` scopes. Scope capabilities are independent, so the token needs both.

Group names must be unique among active groups, compared exactly and case-sensitively. Creating or renaming to a name that's taken fails with a `409 conflict_error`.

SCIM-managed groups can be imported and read, but Notion refuses to rename or delete them. Change those in the identity provider.

## Example Usage

```terraform
data "notion_workspaces" "org" {}

resource "notion_permission_group" "engineering" {
  workspace_id = data.notion_workspaces.org.workspaces[0].id
  name         = "Engineering"
}
```

## Schema

### Required

- `workspace_id` (String) The ID of the workspace the group belongs to. See the `notion_workspaces` data source. Changing this forces a new group.
- `name` (String) The name of the group, 1 to 200 characters. Renaming happens in place.

### Optional

- `transfer_to_owner_id` (String) An active workspace owner who receives access that deleting the group would otherwise orphan. Only used on destroy. When Notion refuses a delete with a conflict, set this, apply, then destroy again.

### Read-Only

- `id` (String) The ID of the group.

## Import

```shell
terraform import notion_permission_group.engineering <workspace-id>/<group-id>
```
