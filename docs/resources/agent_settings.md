---
page_title: "notion_agent_settings Resource - Notion"
subcategory: ""
description: |-
  Manages the enabled status and credit limit of an existing Notion Custom Agent.
---

# notion_agent_settings (Resource)

Manages the enabled status and credit limit of an existing Custom Agent. The API can't create agents, so this resource adopts one by ID. Create it in Notion first.

The Agent APIs have been generally available since 2026-11-02. A connection token needs the "Edit selected agents" capability to change an agent. Notion doesn't let a connection change a credit limit, so `credit_limit` and `unlimited_credits` need a personal access token.

Each setting is managed only when you set it. Leave `enabled` out to keep Terraform away from the agent's status, and leave out both `credit_limit` and `unlimited_credits` to keep it away from the limit.

## Destroying this resource

Destroying `notion_agent_settings` only removes it from Terraform state. It does not delete the agent, re-enable it, or restore an earlier credit limit. The agent keeps whatever settings it had at that point.

## Example Usage

```terraform
data "notion_agents" "triage" {
  query      = "triage"
  agent_type = "custom_agent"
}

resource "notion_agent_settings" "triage" {
  agent_id     = data.notion_agents.triage.agents[0].id
  enabled      = true
  credit_limit = 500
}
```

Clear the agent's own limit:

```terraform
resource "notion_agent_settings" "triage" {
  agent_id          = "1f0c7c06-781b-4987-9986-5c8dd3028013"
  unlimited_credits = true
}
```

## Access

A personal access token acts as you and uses your access to the agent. A connection token needs the **View sessions and interact with agents** capability, and the agent must be shared with the connection. Changing `enabled` needs edit access to the agent. Reading or changing the credit limit needs full access.

## How enabled behaves

- `enabled = false` disables the agent through the API. Notion then reports `status = "disabled"` with `pause_reason = "disabled_from_api"`.
- `enabled = true` re-enables an agent that was turned off through the API or the agent's settings in Notion.
- Notion also pauses agents on its own, for example when a credit or run limit is reached. Those pauses show up in `status` and `pause_reason` but are not treated as drift on `enabled`, and the resource doesn't try to lift them. If you apply `enabled = true` while such a pause is active, the apply succeeds with a warning and the agent stays paused until the cause is fixed in Notion.
- Someone re-enabling the agent in Notion while the config says `enabled = false` does show up as drift, and the next apply disables it again.

## How credit_limit behaves

Notion's API only reports the effective limit, which folds in any workspace default. It never returns the agent's own limit on a read. So `credit_limit` is trusted as written, and a change made to the agent's own limit outside Terraform is not detected. Compare against `effective_credit_limit` if you need to see what Notion enforces.

## Schema

### Required

- `agent_id` (String) ID of the Custom Agent. Changing this forces a new resource. The personal agent (`notion_ai`) has no status or credit limit controls and is rejected.

### Optional

- `enabled` (Boolean) Whether the agent may run. Omit to leave the status unmanaged.
- `credit_limit` (Number) The agent's own credit limit, zero or more. Omit to leave it unmanaged. Conflicts with `unlimited_credits`.
- `unlimited_credits` (Boolean) Set to `true` to clear the agent's own credit limit. A workspace default limit can still apply.

### Read-Only

- `id` (String) Same as `agent_id`.
- `name` (String) The agent's name.
- `agent_type` (String) `custom_agent` or `autofill_custom_agent`.
- `status` (String) `active`, `disabled`, or `deleted`, as reported by Notion.
- `pause_reason` (String) Why the agent is paused when `status` is `disabled`, for example `disabled_from_api` or `credit_limit`. Null when active.
- `effective_credit_limit` (Number) The limit Notion enforces. Null when uncapped or when the token lacks full access.

If the agent is deleted in Notion, the next refresh removes the resource from state.

## Import

Import by agent ID. `enabled` and the credit limit start unmanaged until you set them in config.

```shell
terraform import notion_agent_settings.triage 1f0c7c06-781b-4987-9986-5c8dd3028013
```
