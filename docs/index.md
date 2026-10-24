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

Set `silent_notifications = true` to create and update pages without Notion notifications (Notion API 2026-10-08). That skips page update notifications, @mentions, and notifications for people added to a Person property. Database automations, reminders, and connection webhooks still run. This is useful when Terraform manages pages that people are mentioned or assigned in, since every apply would otherwise notify them.

```terraform
provider "notion" {
  silent_notifications = true
}
```

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
- `notion_database_entries` - List all entries in a database
- `notion_meeting_notes` - Query AI meeting notes for the integration's user
- `notion_view_query` - Query a Notion view
- `notion_agent` - Read one Custom Agent (beta)
- `notion_agents` - List the Custom Agents the token can access (beta)
- `notion_agent_insights` - Credit usage and run counts for a Custom Agent (beta)

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `silent_notifications` (Boolean) Create and update pages without sending Notion notifications. Defaults to `false`.
- `token` (String, Sensitive) Notion API token. Can also be set via the `NOTION_TOKEN` environment variable.
