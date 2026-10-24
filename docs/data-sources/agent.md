---
page_title: "notion_agent Data Source - Notion"
subcategory: ""
description: |-
  Beta. Reads one Notion Custom Agent.
---

# notion_agent (Data Source)

~> **Beta:** This data source uses the Notion Agent APIs, which have been in public beta since 2026-08-20.

Reads one Custom Agent, or the personal agent with `agent_id = "notion_ai"`. It works with a personal access token, or with a connection token that has the **View sessions and interact with agents** capability and that the agent is shared with.

Fields the token isn't allowed to read come back from Notion as `hidden`. Here they read as null, and `credit_limit_hidden` tells you when the limit was hidden.

## Example Usage

```terraform
data "notion_agent" "triage" {
  agent_id = "1f0c7c06-781b-4987-9986-5c8dd3028013"
}

output "triage_model" {
  value = jsondecode(data.notion_agent.triage.raw_json).model
}
```

## Schema

### Required

- `agent_id` (String) The agent ID, or `notion_ai` for the personal agent.

### Read-Only

- `id` (String) The agent ID.
- `name` (String) The agent's name.
- `description` (String) The description, or null.
- `agent_type` (String) `notion_ai`, `custom_agent`, `autofill_custom_agent`, or `external`.
- `status` (String) `active`, `disabled`, or `deleted`.
- `pause_reason` (String) Why the agent is paused when `status` is `disabled`, or null.
- `instructions_page_id` (String) The page holding the agent's instructions, or null when they are inline.
- `effective_credit_limit` (Number) The enforced credit limit, folding in any workspace default. Null when uncapped or hidden.
- `credit_limit_hidden` (Boolean) True when the token lacks full access to the agent.
- `last_run_at` (String) ISO 8601 time of the most recent run. Null when the agent never ran or the token lacks edit access.
- `created_time` (String) When the agent was created.
- `last_edited_time` (String) When the agent was last edited.
- `raw_json` (String) The full agent object as JSON, including fields not modeled here such as `model`, `connections`, and `triggers`.
