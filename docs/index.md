---
page_title: "Notion Provider"
subcategory: ""
description: |-
  The Notion provider allows you to manage Notion pages, blocks, databases, and database properties using Terraform.
---

# Notion Provider

The Notion provider allows you to manage [Notion](https://www.notion.so/) resources using Terraform. It supports creating and managing pages, content blocks, databases, database properties, and database entries through the [Notion API](https://developers.notion.com/).

## Authentication

The provider requires a Notion API token (integration token). You can obtain one by creating an integration at [https://www.notion.so/my-integrations](https://www.notion.so/my-integrations).

The token can be provided in two ways:

1. Set the `token` attribute in the provider configuration
2. Set the `NOTION_TOKEN` environment variable (recommended)

~> **Note:** Make sure to share the relevant pages/databases with your integration in Notion, otherwise the API will not be able to access them.

~> **OAuth public connections:** If you obtain your token via the OAuth flow on a public connection, Notion mints a fresh `access_token` and `refresh_token` for each successful authorization (2026-06-08 change). Always store and use the latest pair returned — including from re-authorizations of the same connection — and feed that `access_token` to this provider via `NOTION_TOKEN` or the `token` attribute. The provider itself does not perform the OAuth flow.

~> **Personal access tokens expire:** Since 2026-07-02 a personal access token can expire after 7, 30, 90, or 180 days, or 1 year (the default). An expired token fails with an `unauthorized` error, so rotate the value behind `NOTION_TOKEN` before it lapses.

## Silent writes

Set `silent_notifications = true` to create and update pages, and append blocks, without Notion notifications (Notion API 2026-10-08). That skips page update notifications, @mentions, and notifications for people added to a Person property. Database automations, reminders, and connection webhooks still run. This is useful when Terraform manages pages that people are mentioned or assigned in, since every apply would otherwise notify them.

```terraform
provider "notion" {
  silent_notifications = true
}
```

## Admin API

Enterprise organizations can manage permission groups, legal holds, custom agent governance, MCP client access, personal access tokens, and workspace exports through Notion's [Admin API](https://developers.notion.com/reference/admin/intro) (reference added 2026-08-24). It uses an organization bot token, which an organization owner creates in the organization console with per-resource scopes. Set it as `admin_token` or `NOTION_ADMIN_TOKEN`. A provider block can set only `admin_token` when it manages nothing but Admin API resources.

```terraform
provider "notion" {
  admin_token = var.notion_admin_token
}

data "notion_workspaces" "org" {}

resource "notion_permission_group" "engineering" {
  workspace_id = data.notion_workspaces.org.workspaces[0].id
  name         = "Engineering"
}
```

| Resource, data source, or action | Scopes |
|---|---|
| `notion_permission_group`, `notion_permission_group_member` | `permission-group:read`, `permission-group:write` |
| `notion_permission_groups` | `permission-group:read` |
| `notion_admin_users` | `user:read` |
| `notion_workspaces` | `workspace:read` |
| `notion_legal_hold` | `legal-hold:read`, `legal-hold:write`, plus `legal-hold:write-high-impact` with `release_on_destroy` |
| `notion_legal_holds`, `notion_legal_hold_pages` | `legal-hold:read` |
| `notion_legal_hold_export` (action) | `legal-hold:export` |
| `notion_revoke_user_sessions` (action) | `managed-user-session:write` |
| `notion_admin_agent_settings` | `workflows:read`, `workflows:write` |
| `notion_admin_workspace_agent_settings`, `notion_admin_agent_delete` (action) | `workflows:write` |
| `notion_admin_agents`, `notion_admin_agent_credit_usage`, `notion_admin_agents_credit_usage` | `workflows:read` |
| `notion_mcp_client_connections` | `mcp-client-connection:read` |
| `notion_mcp_enterprise_access`, `notion_mcp_client_connection_revoke` (action) | `mcp-client-connection:write-high-impact` |
| `notion_personal_access_tokens` | `personal-access-token:read` |
| `notion_personal_access_token_revoke` (action) | `personal-access-token:write-high-impact` |
| `notion_enqueue_workspace_export` (action), `notion_workspace_export_status` | `workspace:export` |

Scope capabilities are independent, so `permission-group:write` doesn't include `permission-group:read`.

### Destroy never makes org-level changes implicitly

Destroying an Admin API resource doesn't release, restore, or delete anything unless you opt in. `notion_legal_hold` leaves the hold active (opt in with `release_on_destroy`), `notion_mcp_enterprise_access` leaves a denial in place (opt in with `restore_on_destroy`), and `notion_admin_agent_settings` and `notion_admin_workspace_agent_settings` leave the agent and settings untouched.

### Actions

One-shot operations are [Terraform actions](https://developer.hashicorp.com/terraform/language/invoke-actions), which need Terraform 1.14 or later. They run only when invoked, with `terraform apply -invoke=action.<type>.<name>` or from a resource's `lifecycle.action_trigger`, never during plan or destroy. Actions have no outputs, so values such as an export job ID appear as progress messages.

- `notion_legal_hold_export` - Start or continue a legal hold export
- `notion_revoke_user_sessions` - Sign a managed user out of every session
- `notion_admin_agent_delete` - Delete a custom agent
- `notion_mcp_client_connection_revoke` - Revoke a member's MCP client tokens in a workspace
- `notion_personal_access_token_revoke` - Permanently revoke a personal access token
- `notion_enqueue_workspace_export` - Start a workspace export

## Rate limits and retries

Notion limits requests per connection (180 per minute, or 600 on Business and Enterprise, since 2026-09-09) and per workspace across all connections (since 2026-06-16). The provider retries `429` responses using Notion's `Retry-After` value and retries transient `5xx` responses with backoff. A `504 gateway_timeout` on a write is not retried, because Notion says the write may already have saved, so check the page before re-running the apply. On Free workspaces, writes past the workspace block limit fail with `403` (since 2026-09-01).

## Example Usage

```terraform
terraform {
  required_providers {
    notion = {
      source = "delize/notion"
    }
  }
}

# Configure via environment variable (recommended)
provider "notion" {}

# Or configure with token directly
provider "notion" {
  token = "secret_..."
}
```

## Resources

### Core Resources
- `notion_page` - Manage Notion pages
- `notion_block` - Manage content blocks on pages (paragraphs, headings, lists, code, etc.)
- `notion_database` - Manage Notion databases
- `notion_database_entry` - Manage entries (rows) in Notion databases
- `notion_view` - Manage database views (2026-03-19 Views API)

### Database Property Resources
- `notion_database_property_select` - Select property with options
- `notion_database_property_multi_select` - Multi-select property with options
- `notion_database_property_status` - Status property with options (writable as of 2026-03-19)
- `notion_database_property_number` - Number property with format
- `notion_database_property_relation` - Relation property linking databases
- `notion_database_property_rollup` - Rollup property for aggregations
- `notion_database_property_rich_text` - Rich text property
- `notion_database_property_date` - Date property
- `notion_database_property_people` - People property
- `notion_database_property_checkbox` - Checkbox property
- `notion_database_property_url` - URL property
- `notion_database_property_email` - Email property
- `notion_database_property_created_time` - Created time property (automatic)
- `notion_database_property_created_by` - Created by property (automatic)
- `notion_database_property_last_edited_time` - Last edited time property (automatic)
- `notion_database_property_last_edited_by` - Last edited by property (automatic)

### Custom Agent Resources (beta)
- `notion_agent_settings` - Enabled status and credit limit of an existing Custom Agent (Agent APIs public beta, 2026-08-20)

## Data Sources

- `notion_database` - Look up an existing database by title
- `notion_page` - Look up an existing page by title
- `notion_user` - Look up a Notion user by email
- `notion_database_entries` - List entries in a database, with optional filter and sorts
- `notion_meeting_notes` - Query AI meeting notes for the integration's user
- `notion_view_query` - Query a Notion view
- `notion_page_markdown` - Read a page's content as enhanced markdown
- `notion_search` - Search pages and databases, including the trash
- `notion_users` - List every user the integration can see
- `notion_blocks` - List a page's child blocks
- `notion_custom_emojis` - List the workspace's custom emojis
- `notion_database_templates` - List a database's page templates, to find a `template_id`
- `notion_agent` - Read one Custom Agent (beta)
- `notion_agents` - List the Custom Agents the token can access (beta)
- `notion_agent_insights` - Credit usage and run counts for a Custom Agent (beta)

### Admin API (Enterprise, needs `admin_token`)
- `notion_permission_group` (resource) - Admin-managed permission group
- `notion_permission_group_member` (resource) - Direct user membership in a permission group
- `notion_permission_groups` - List a workspace's permission groups
- `notion_admin_users` - List a workspace's members and guest bots
- `notion_workspaces` - List the organization's workspaces
- `notion_legal_hold` (resource) - Legal hold and its users. Destroy keeps the hold unless `release_on_destroy`
- `notion_legal_holds` - List legal holds
- `notion_legal_hold_pages` - Pages a hold preserves in a workspace
- `notion_admin_agent_settings` (resource) - An agent's admin status, credit limit, and sharing
- `notion_admin_workspace_agent_settings` (resource) - Agent creation policy and default credit limit
- `notion_admin_agents` - List agents
- `notion_admin_agent_credit_usage` - One agent's credit usage
- `notion_admin_agents_credit_usage` - Credit usage for every agent
- `notion_mcp_enterprise_access` (resource) - Deny or restore a member's enterprise-managed MCP access
- `notion_mcp_client_connections` - List members' MCP client connections
- `notion_personal_access_tokens` - List personal access token records
- `notion_workspace_export_status` - Status and download URLs of a workspace export

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `admin_token` (String, Sensitive) Notion Admin API organization bot token, used only by the Admin API resources, data sources, and actions. Requires an Enterprise plan. Can also be set via the `NOTION_ADMIN_TOKEN` environment variable.
- `silent_notifications` (Boolean) Create and update pages without sending Notion notifications. Defaults to `false`.
- `token` (String, Sensitive) Notion API token. Can also be set via the `NOTION_TOKEN` environment variable.
