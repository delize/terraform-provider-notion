---
page_title: "notion_mcp_client_connections Data Source - Notion"
subcategory: "Admin API"
description: |-
  Lists members' MCP client connections to Notion through the Admin API.
---

# notion_mcp_client_connections (Data Source)

Lists the OAuth connections members use to connect MCP clients, such as Claude Code or Codex, to Notion MCP, through the [Admin API](https://developers.notion.com/reference/admin/list-mcp-client-connections). Workspace-owned internal connections aren't included. Needs `admin_token` with the `mcp-client-connection:read` scope.

Each result groups one member and one client across workspaces. Enterprise-managed and self-authorized connections for the same pair come back as separate results.

## Example Usage

```terraform
data "notion_mcp_client_connections" "acme" {
  workspace_ids = [data.notion_workspaces.org.workspaces[0].id]
}

output "unmanaged_mcp_clients" {
  value = [for c in data.notion_mcp_client_connections.acme.connections : "${c.user_id} ${c.client_name}" if !c.is_enterprise_managed]
}
```

## Schema

### Optional

- `user_ids` (List of String) Only these members.
- `workspace_ids` (List of String) Only these workspaces. Also limits each result's `workspace_ids`.

### Read-Only

- `connections` (List of Object) Each with `client_key` (pass it to `notion_mcp_client_connection_revoke`), `client_name`, `client_type`, `user_id`, `workspace_ids`, `is_enterprise_managed`, `idp_issuer`, `created_at`, and `last_active_at`. Times are epoch milliseconds.
