---
page_title: "notion_admin_agents Data Source - Notion"
subcategory: "Admin API"
description: |-
  Lists the agents in a Notion workspace through the Admin API.
---

# notion_admin_agents (Data Source)

Lists the custom and database autofill agents in a workspace, with their status and sharing, through the [Admin API](https://developers.notion.com/reference/admin/get-workflows-metadata-for-space). Needs `admin_token` with the `workflows:read` scope. Deleted agents are included only when `statuses` contains `deleted`.

## Example Usage

```terraform
data "notion_admin_agents" "paused_for_credits" {
  workspace_id = data.notion_workspaces.org.workspaces[0].id
  statuses     = ["credit_limit", "workspace_credit_limit"]
}
```

## Schema

### Required

- `workspace_id` (String) The workspace.

### Optional

- `name` (String) Case-insensitive substring match on name or description.
- `statuses` (List of String) Only agents with these statuses. `disabled` matches any admin-disabled agent.

### Read-Only

- `agents` (List of Object) Each agent has `id`, `name`, `description`, `type`, `status`, `alive`, `model`, `created_by_id`, `created_time`, `last_edited_time`, `last_run_time`, `last_credit_usage_time`, `parent_id`, `parent_table`, and `permissions` (each with `principal_type`, `principal_id`, `role`, `resolved_role`). Times are epoch milliseconds.
