---
page_title: "notion_legal_hold_pages Data Source - Notion"
subcategory: "Admin API"
description: |-
  Lists the pages a legal hold preserves in one workspace.
---

# notion_legal_hold_pages (Data Source)

Lists the IDs of the pages a legal hold preserves in one workspace, through the [Admin API](https://developers.notion.com/reference/admin/list-legal-hold-pages). Needs `admin_token` with the `legal-hold:read` scope. The list can be long, and every page is read on each refresh.

## Example Usage

```terraform
data "notion_legal_hold_pages" "held" {
  legal_hold_id = notion_legal_hold.smith_v_acme.id
  workspace_id  = data.notion_workspaces.org.workspaces[0].id
}
```

## Schema

### Required

- `legal_hold_id` (String) The legal hold.
- `workspace_id` (String) The workspace to list held pages in.

### Read-Only

- `page_ids` (List of String) IDs of held pages.
