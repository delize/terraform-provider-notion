---
page_title: "notion_agent_insights Data Source - Notion"
subcategory: ""
description: |-
  Reads credit usage and run counts for a Notion Custom Agent.
---

# notion_agent_insights (Data Source)

Reads premium AI credit usage and completed runs for a Custom Agent over a time window, by default the current billing period.

Pass `agent_id = "notion_ai"` for your personal agent. That only works with a personal access token, and the totals cover only the user the token acts as. In that case `credit_limit` is the workspace's per-member default.

## Example Usage

```terraform
data "notion_agent_insights" "triage" {
  agent_id = "1f0c7c06-781b-4987-9986-5c8dd3028013"
}

output "triage_credits" {
  value = data.notion_agent_insights.triage.total_credits_used
}
```

## Schema

### Required

- `agent_id` (String) The agent ID, or `notion_ai`.

### Optional

- `start_time` (Number) Start of the window as epoch seconds. Set together with `end_time`.
- `end_time` (Number) End of the window as epoch seconds. Set together with `start_time`.

### Read-Only

- `name` (String) The agent's name.
- `agent_type` (String) `notion_ai`, `custom_agent`, or `autofill_custom_agent`.
- `status` (String) `active`, `disabled`, or `deleted`.
- `total_credits_used` (Number) Premium AI credits used in the window.
- `runs_completed` (Number) Runs recorded in the window.
- `credit_limit` (Number) The enforced limit. Null when none is set or when it is hidden.
- `credit_limit_hidden` (Boolean) True when the token lacks full access to the agent.
