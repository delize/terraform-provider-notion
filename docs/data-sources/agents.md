---
page_title: "notion_agents Data Source - Notion"
subcategory: ""
description: |-
  Beta. Lists the Notion Custom Agents the token can access.
---

# notion_agents (Data Source)

~> **Beta:** This data source uses the Notion Agent APIs, which have been in public beta since 2026-08-20.

Lists the Custom Agents the token can access, newest first. A personal access token sees the agents its user can read. A connection token sees only the agents shared with its connection.

## Example Usage

```terraform
data "notion_agents" "disabled" {
  agent_type = "custom_agent"
  statuses   = ["disabled"]
}

# Conditions the shortcuts don't cover go in a raw filter.
data "notion_agents" "auto_model" {
  filter = jsonencode({
    and = [
      { property = "model_mode", string = { equals = "auto" } },
      { property = "created_time", date = { after = "2026-09-01T00:00:00Z" } },
    ]
  })
}
```

## Schema

### Optional

- `query` (String) Case-insensitive substring match on agent names and descriptions.
- `agent_type` (String) Only return `custom_agent` or `autofill_custom_agent` agents. Conflicts with `filter`.
- `statuses` (List of String) Only return agents in these statuses: `active`, `disabled`, `deleted`. Listing deleted agents also needs `include_deleted = true`. Conflicts with `filter`.
- `filter` (String) A raw Query agents filter as JSON, for example from `jsonencode()`. See the [Query agents reference](https://developers.notion.com/reference/notion-agent-apis/query-agents) for the properties and conditions.
- `include_deleted` (Boolean) Include soft-deleted agents. Defaults to `false`.
- `limit` (Number) Maximum number of agents to return. Defaults to all.

### Read-Only

- `agents` (List of Object) Matching agents. Each has:
  - `id` (String)
  - `name` (String)
  - `agent_type` (String)
  - `status` (String)
  - `pause_reason` (String) Null when active.
  - `effective_credit_limit` (Number) Null when uncapped or hidden.
  - `last_run_at` (String) Null when never run or hidden.
  - `created_time` (String)
