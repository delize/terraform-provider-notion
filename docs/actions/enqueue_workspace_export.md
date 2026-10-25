---
page_title: "notion_enqueue_workspace_export Action - Notion"
subcategory: "Admin API"
description: |-
  Starts an export of a Notion workspace.
---

# notion_enqueue_workspace_export (Action)

Starts an export of a workspace through the [Admin API](https://developers.notion.com/reference/admin/enqueue-space-export). Needs Terraform 1.14 or later and `admin_token` with the `workspace:export` scope. It runs only when invoked. The export job ID appears as a progress message. Read the job's progress and download URLs with the `notion_workspace_export_status` data source.

## Example Usage

```terraform
action "notion_enqueue_workspace_export" "monthly_backup" {
  config {
    workspace_id            = data.notion_workspaces.org.workspaces[0].id
    export_type             = "markdown"
    on_behalf_of_user_email = "it-admin@example.com"
    include_contents        = "everything"
  }
}
```

```shell
terraform apply -invoke=action.notion_enqueue_workspace_export.monthly_backup
```

## Schema

### Required

- `workspace_id` (String) The workspace to export.
- `export_type` (String) `html` or `markdown`.
- `on_behalf_of_user_email` (String) Email of the user the export runs as.

### Optional

- `collection_view_export_type` (String) `all` or `currentView`.
- `flatten_export_filetree` (Boolean)
- `include_comments` (Boolean)
- `include_contents` (String) `everything` or `no_files`.
- `locale` (String)
- `teamspace_ids` (List of String) Only export these teamspaces.
- `time_zone` (String)

The reference lists these options without describing them further, so their effect is as Notion's export dialog describes it.
