---
page_title: "notion_admin_agent_settings Resource - Notion"
subcategory: "Admin API"
description: |-
  Governs an existing Notion custom agent: admin status, credit limit, and sharing.
---

# notion_admin_agent_settings (Resource)

Governs an existing custom agent through the [Admin API](https://developers.notion.com/reference/admin/update-agent-status): whether an admin has disabled it, its admin credit limit, and who it's shared with. Needs an Enterprise plan and `admin_token` with the `workflows:read` and `workflows:write` scopes.

This resource never creates or deletes an agent. **Destroying it leaves the agent and every setting as they are.** To delete an agent, use the `notion_admin_agent_delete` action. Attributes you leave unset aren't managed.

- `admin_status`: Notion rejects both enable and disable while an agent is paused for a credit or operational limit (such as `credit_limit`, `run_limit`, or `tool_unavailable`). The provider applies `credit_limit` before `admin_status`, so raising a limit and enabling in one apply works when the limit was the cause.
- `credit_limit`: the API can set and clear it but can't read it back. Changes made in Notion aren't detected. `enforced_credit_limit` shows what actually applies.
- `permissions`: when set, the list is authoritative. Grants not listed, including comment-only grants made in Notion, are removed on apply. The API can grant `edit`, `full_access`, and `view_and_interact`, not `comment_only`.

## Example Usage

```terraform
resource "notion_admin_agent_settings" "triage" {
  workspace_id = data.notion_workspaces.org.workspaces[0].id
  agent_id     = "00000000-0000-0000-0000-000000000000"
  admin_status = "active"
  credit_limit = 500

  permissions = [
    { principal_type = "workspace", role = "view_and_interact" },
    { principal_type = "group", principal_id = notion_permission_group.engineering.id, role = "edit" },
  ]
}
```

## Schema

### Required

- `workspace_id` (String) The agent's workspace. Changing this forces a new resource.
- `agent_id` (String) The custom agent's ID. The personal agent (Notion AI) isn't supported. Changing this forces a new resource.

### Optional

- `admin_status` (String) `active` or `disabled`.
- `credit_limit` (Number) Admin per-agent credit limit in whole credits. Removing it clears the limit.
- `permissions` (Set of Object) Authoritative sharing list. Each entry has `principal_type` (`user`, `group`, or `workspace`), `principal_id` (unset for `workspace`), and `role`.

### Read-Only

- `id` (String) `<workspace_id>/<agent_id>`.
- `name` (String) The agent's name.
- `type` (String) `custom` or `database_autofill`.
- `status` (String) `active`, `deleted`, or a paused reason.
- `enforced_credit_limit` (Number) The limit that applies: the lower of the admin and creator limits, or the workspace default. Null when none applies.

## Import

```shell
terraform import notion_admin_agent_settings.triage <workspace-id>/<agent-id>
```

After import `admin_status`, `credit_limit`, and `permissions` are unset, so the next apply sets whatever the configuration lists.
