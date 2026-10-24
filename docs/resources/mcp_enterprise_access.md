---
page_title: "notion_mcp_enterprise_access Resource - Notion"
subcategory: "Admin API"
description: |-
  Denies or restores one member's enterprise-managed MCP client access in a workspace.
---

# notion_mcp_enterprise_access (Resource)

Denies or restores one member's enterprise-managed MCP client access in a workspace, through the [Admin API](https://developers.notion.com/reference/admin/update-mcp-client-connection-enterprise-managed-access). Denying blocks future connections through enterprise-managed authentication and revokes the member's current tokens. It covers every client for that member and workspace. It does not apply to MCP connections members authorize themselves. Needs `admin_token` with the `mcp-client-connection:write-high-impact` scope.

The API can set access but not read it, so changes made in Notion aren't detected.

~> **Destroying keeps the current access.** Removing the resource leaves a denial in place, so deleting a block never silently re-opens access. Set `restore_on_destroy = true` to restore access on destroy.

## Example Usage

```terraform
resource "notion_mcp_enterprise_access" "contractor" {
  workspace_id = data.notion_workspaces.org.workspaces[0].id
  user_id      = "00000000-0000-0000-0000-000000000000"
  access       = "denied"
}
```

## Schema

### Required

- `workspace_id` (String) The workspace. Changing this forces a new resource.
- `user_id` (String) The member. Changing this forces a new resource.
- `access` (String) `denied` or `allowed`. Changes in place.

### Optional

- `restore_on_destroy` (Boolean) Set access back to `allowed` on destroy. Defaults to `false`.

### Read-Only

- `id` (String) `<workspace_id>/<user_id>`.

## Import

```shell
terraform import notion_mcp_enterprise_access.contractor <workspace-id>/<user-id>
```

The API can't read the current access, so after import the next apply sets `access` from the configuration.
