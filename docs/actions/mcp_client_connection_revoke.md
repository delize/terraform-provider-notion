---
page_title: "notion_mcp_client_connection_revoke Action - Notion"
subcategory: "Admin API"
description: |-
  Revokes a member's MCP client tokens in a workspace.
---

# notion_mcp_client_connection_revoke (Action)

Revokes one member's active OAuth tokens for one MCP client in one workspace, through the [Admin API](https://developers.notion.com/reference/admin/revoke-mcp-client-connection). The member is disconnected but can connect again if workspace policy allows it. To block reconnection through enterprise-managed authentication, use `notion_mcp_enterprise_access`. Needs Terraform 1.14 or later and `admin_token` with the `mcp-client-connection:write-high-impact` scope. It runs only when invoked, and reports the number of revoked tokens as a progress message.

## Example Usage

```terraform
action "notion_mcp_client_connection_revoke" "ada_cursor" {
  config {
    client_key   = "cursor"
    user_id      = "00000000-0000-0000-0000-000000000000"
    workspace_id = data.notion_workspaces.org.workspaces[0].id
  }
}
```

```shell
terraform apply -invoke=action.notion_mcp_client_connection_revoke.ada_cursor
```

## Schema

### Required

- `client_key` (String) The client key from `notion_mcp_client_connections`. Use the key, not the client name.
- `user_id` (String) The member.
- `workspace_id` (String) The workspace.
