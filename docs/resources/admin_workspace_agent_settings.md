---
page_title: "notion_admin_workspace_agent_settings Resource - Notion"
subcategory: "Admin API"
description: |-
  Sets who can create custom agents in a workspace and its default agent credit limit.
---

# notion_admin_workspace_agent_settings (Resource)

Sets who can create custom agents in a workspace and the default credit limit for agents, through the [Admin API](https://developers.notion.com/reference/admin/update-agent-creation-policy). Needs an Enterprise plan and `admin_token` with the `workflows:write` scope.

The Admin API can change these settings but has no endpoint to read them. Terraform keeps the last values it applied, so **changes made in Notion aren't detected**. Destroying the resource leaves both settings as they are.

`disable_existing_agents = true` with `creation_policy = "disabled"` also disables every existing custom agent when the policy changes. Re-enabling them afterwards is a per-agent change, for example with `notion_admin_agent_settings`.

## Example Usage

```terraform
resource "notion_admin_workspace_agent_settings" "acme" {
  workspace_id               = data.notion_workspaces.org.workspaces[0].id
  creation_policy            = "workspace_owners_only"
  default_agent_credit_limit = 1000
}
```

## Schema

### Required

- `workspace_id` (String) The workspace. Changing this forces a new resource.

### Optional

- `creation_policy` (String) `all_workspace_members`, `workspace_owners_only`, or `disabled`. Removing it leaves the current policy in place.
- `disable_existing_agents` (Boolean) Only with `creation_policy = "disabled"`. Sent only when the policy changes.
- `default_agent_credit_limit` (Number) Default limit in whole credits for agents going forward. Removing it removes the default cap.

### Read-Only

- `id` (String) Same as `workspace_id`.
- `current_policy_description` (String) Notion's description of the policy after the last policy change.

## Import

```shell
terraform import notion_admin_workspace_agent_settings.acme <workspace-id>
```
