---
page_title: "notion_admin_agents_credit_usage Data Source - Notion"
subcategory: "Admin API"
description: |-
  Reads credit usage for every agent in a Notion workspace.
---

# notion_admin_agents_credit_usage (Data Source)

Reads premium AI credit usage for every agent in a workspace through the [Admin API](https://developers.notion.com/reference/admin/get-agents-credit-usage). Needs `admin_token` with the `workflows:read` scope. Each agent's `periods` holds only the first page Notion returns, starting with the current period. Use `notion_admin_agent_credit_usage` for one agent's full history.

## Example Usage

```terraform
data "notion_admin_agents_credit_usage" "acme" {
  workspace_id = data.notion_workspaces.org.workspaces[0].id
}

output "top_spenders" {
  value = [for a in data.notion_admin_agents_credit_usage.acme.agents : a.name if a.total_credits_used > 1000]
}
```

## Schema

### Required

- `workspace_id` (String) The workspace.

### Optional

- `include_deleted` (Boolean) Include deleted agents that still have usage. Defaults to `false`.

### Read-Only

- `agents` (List of Object) Each with `id`, `name`, `type`, `status`, `credit_limit`, `total_credits_used`, `runs_completed`, `last_credit_usage_time`, and `periods`. Credit values are fractional, as described on `notion_admin_agent_credit_usage`.
