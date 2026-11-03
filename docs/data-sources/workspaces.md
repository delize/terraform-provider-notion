---
page_title: "notion_workspaces Data Source - Notion"
subcategory: "Admin API"
description: |-
  Lists the live workspaces your Notion organization owns, through the Admin API.
---

# notion_workspaces (Data Source)

Lists the live workspaces your Notion organization owns, ordered by ID, through the [Admin API](https://developers.notion.com/reference/admin/list-spaces). Deleted workspaces and workspaces outside the organization aren't included. Needs `admin_token` with the `workspace:read` scope.

Use a workspace `id` as `workspace_id` on the other Admin API resources and data sources.

## Example Usage

```terraform
data "notion_workspaces" "org" {}

locals {
  workspace_ids = { for w in data.notion_workspaces.org.workspaces : w.name => w.id if w.name != null }
}
```

## Schema

### Read-Only

- `workspaces` (List of Object) The organization's workspaces.
  - `id` (String) The workspace ID.
  - `name` (String) The workspace name, or null when it has none.
