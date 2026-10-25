---
page_title: "notion_admin_agent_delete Action - Notion"
subcategory: "Admin API"
description: |-
  Deletes a Notion custom agent.
---

# notion_admin_agent_delete (Action)

Deletes a custom agent, moving it to Trash, through the [Admin API](https://developers.notion.com/reference/admin/delete-agent). Needs Terraform 1.14 or later and `admin_token` with the `workflows:write` scope. The personal agent (Notion AI) isn't supported. It runs only when invoked.

## Example Usage

```terraform
action "notion_admin_agent_delete" "old_digest" {
  config {
    workspace_id = data.notion_workspaces.org.workspaces[0].id
    agent_id     = "00000000-0000-0000-0000-000000000000"
  }
}
```

```shell
terraform apply -invoke=action.notion_admin_agent_delete.old_digest
```

## Schema

### Required

- `workspace_id` (String) The workspace.
- `agent_id` (String) The custom agent to delete.
