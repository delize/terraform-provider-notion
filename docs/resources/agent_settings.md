---
page_title: "notion_agent_settings Resource - Notion"
subcategory: ""
description: |-
  Manages the name, description, model, enabled status, and credit limit of an existing Notion Custom Agent.
---

# notion_agent_settings (Resource)

Manages the name, description, model, enabled status, and credit limit of an existing Custom Agent. The API can't create agents, so this resource adopts one by ID. Create it in Notion first.

The Agent APIs have been generally available since 2026-11-02. A connection token needs the "Edit selected agents" capability to change an agent. Notion doesn't let a connection change a credit limit, so `credit_limit` and `unlimited_credits` need a personal access token.

Each setting is managed only when you set it. Leave `name`, `description`, `model_mode`, or `enabled` out and Terraform reads the value from Notion but never changes it. Leave out both `credit_limit` and `unlimited_credits` to keep Terraform away from the limit.

## Destroying this resource

Destroying `notion_agent_settings` only removes it from Terraform state. It does not delete the agent, re-enable it, or restore an earlier name, description, model, or credit limit. The agent keeps whatever settings it had at that point.

## Example Usage

```terraform
data "notion_agents" "triage" {
  query      = "triage"
  agent_type = "custom_agent"
}

resource "notion_agent_settings" "triage" {
  agent_id     = data.notion_agents.triage.agents[0].id
  name         = "Ticket triage"
  description  = "Sorts new support tickets by product area"
  enabled      = true
  credit_limit = 500
}
```

Pin a model:

```terraform
resource "notion_agent_settings" "triage" {
  agent_id   = "1f0c7c06-781b-4987-9986-5c8dd3028013"
  model_mode = "pinned"
  model_id   = "claude-sonnet-5"
}
```

Let Notion choose the model, and clear the agent's own credit limit:

```terraform
resource "notion_agent_settings" "triage" {
  agent_id          = "1f0c7c06-781b-4987-9986-5c8dd3028013"
  model_mode        = "auto"
  unlimited_credits = true
}
```

## Access

A personal access token acts as you and uses your access to the agent. A connection token needs the **View sessions and interact with agents** capability to read the agent and **Edit selected agents** to change it, and the agent must be shared with the connection. Changing `name`, `description`, the model, or `enabled` needs edit access to the agent. Reading or changing the credit limit needs full access, and only a personal access token can change it.

## How updates are sent

Name, description, model, and status go to Notion in a single `PATCH /v1/agents/{agent_id}` per apply, and Notion saves them together or not at all. Only settings that changed since the last apply and differ from what the agent has now are included, so a setting you don't manage is never sent. The credit limit has its own endpoint and is sent separately.

## How the model behaves

- `model_mode = "auto"` lets Notion choose the model for each run. Don't set `model_id` with it. `model_id` reads back as null.
- `model_mode = "pinned"` uses the model in `model_id`, a public model ID such as `claude-sonnet-5`. `model_id` is required with it. The ID must be one the token's user can use in this workspace, or Notion rejects the apply.
- Changing a Custom Agent's model also updates its published version, as Notion documents. Plan model changes with that in mind. The resource only sends the model when it changed in config and differs from what the agent runs on, so an apply that touches other settings doesn't republish the agent.
- Notion reports an early-access model as pinned with no public ID. When that happens the configured `model_id` is kept in state with a warning instead of showing a diff on every plan. The resource can't tell whether that early-access model is the one you configured, so a change made in Notion between two models without public IDs is not detected.
- If both `model_mode` and `model_id` are left out, the model is read from Notion and left alone.

## How description behaves

Set `description = ""` to clear it. An agent with no description reads back as an empty string, so `""` and a cleared description in Notion compare equal.

## How enabled behaves

- `enabled = false` disables the agent through the API. Notion then reports `status = "disabled"` with `pause_reason = "disabled_from_api"`.
- `enabled = true` re-enables an agent that was turned off through the API or the agent's settings in Notion.
- Status is only sent when the agent's current status differs from the one you want. Notion rejects the whole update when it is asked to disable an agent that is already disabled, so this also keeps name, description, and model changes from failing on an already disabled agent.
- Notion also pauses agents on its own, for example when a credit or run limit is reached. Those pauses show up in `status` and `pause_reason` but are not treated as drift on `enabled`, and the resource doesn't try to lift them. If you apply `enabled = true` while such a pause is active, the apply succeeds with a warning and the agent stays paused until the cause is fixed in Notion.
- Someone re-enabling the agent in Notion while the config says `enabled = false` does show up as drift, and the next apply disables it again.

## How credit_limit behaves

Notion's API only reports the effective limit, which folds in any workspace default. It never returns the agent's own limit on a read. So `credit_limit` is trusted as written, and a change made to the agent's own limit outside Terraform is not detected. Compare against `effective_credit_limit` if you need to see what Notion enforces.

## Schema

### Required

- `agent_id` (String) ID of the Custom Agent. Changing this forces a new resource. The personal agent (`notion_ai`) has no status or credit limit controls and is rejected.

### Optional

- `name` (String) The agent's name. Can't be empty. Omit to leave it unmanaged, in which case it reads back from Notion.
- `description` (String) The agent's short description. Set to `""` to clear it. Omit to leave it unmanaged.
- `model_mode` (String) `auto` or `pinned`. Omit to leave the model unmanaged.
- `model_id` (String) Public ID of the model to pin. Required when `model_mode` is `pinned` and not allowed otherwise. Null when the model is `auto`.
- `enabled` (Boolean) Whether the agent may run. Omit to leave the status unmanaged.
- `credit_limit` (Number) The agent's own credit limit, zero or more. Omit to leave it unmanaged. Conflicts with `unlimited_credits`.
- `unlimited_credits` (Boolean) Set to `true` to clear the agent's own credit limit. A workspace default limit can still apply.

### Read-Only

- `id` (String) Same as `agent_id`.
- `agent_type` (String) `custom_agent` or `autofill_custom_agent`.
- `status` (String) `active`, `disabled`, or `deleted`, as reported by Notion.
- `pause_reason` (String) Why the agent is paused when `status` is `disabled`, for example `disabled_from_api` or `credit_limit`. Null when active.
- `effective_credit_limit` (Number) The limit Notion enforces. Null when uncapped or when the token lacks full access.

`name`, `description`, `model_mode`, and `model_id` are also filled in from Notion when you don't set them.

If the agent is deleted in Notion, the next refresh removes the resource from state.

## Upgrading

`name` used to be read-only. `description`, `model_mode`, and `model_id` are new. Existing configurations don't set any of them, so they are read from Notion on the next refresh and the plan stays empty.

## Import

Import by agent ID. Name, description, and model are read from Notion. `enabled` and the credit limit start unmanaged until you set them in config.

```shell
terraform import notion_agent_settings.triage 1f0c7c06-781b-4987-9986-5c8dd3028013
```
