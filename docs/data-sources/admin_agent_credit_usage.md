---
page_title: "notion_admin_agent_credit_usage Data Source - Notion"
subcategory: "Admin API"
description: |-
  Reads one Notion agent's credit usage through the Admin API.
---

# notion_admin_agent_credit_usage (Data Source)

Reads one agent's premium AI credit usage, with every monthly billing period, through the [Admin API](https://developers.notion.com/reference/admin/get-agent-credit-usage). Works for custom and database autofill agents, including deleted ones. Needs `admin_token` with the `workflows:read` scope.

## Example Usage

```terraform
data "notion_admin_agent_credit_usage" "triage" {
  workspace_id = data.notion_workspaces.org.workspaces[0].id
  agent_id     = "00000000-0000-0000-0000-000000000000"
}

output "triage_credits_this_period" {
  value = data.notion_admin_agent_credit_usage.triage.periods[0].credits_used
}
```

## Schema

### Required

- `workspace_id` (String) The workspace.
- `agent_id` (String) The agent.

### Read-Only

- `name`, `type`, `status` (String)
- `credit_limit` (Number) The enforced per-agent limit, or null.
- `total_credits_used`, `runs_completed` (Number) All-time totals.
- `last_credit_usage_time` (Number) Epoch milliseconds, or null.
- `periods` (List of Object) Newest first, each with `start_ms`, `end_ms`, and `credits_used`.

Credits are fractional since 2026-10-30. `credits_used` has up to two decimal places. `total_credits_used` is rounded down to a whole number, or to two significant figures when it is below 1.
