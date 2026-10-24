---
page_title: "notion_workspace_export_status Data Source - Notion"
subcategory: "Admin API"
description: |-
  Reads the status of a Notion workspace export.
---

# notion_workspace_export_status (Data Source)

Reads the status of a workspace export started with the `notion_enqueue_workspace_export` action, through the [Admin API](https://developers.notion.com/reference/admin/get-space-export-status). Needs `admin_token` with the `workspace:export` scope.

`download_urls` is marked sensitive, because anyone holding a URL can download the export until `url_expires_time`.

## Example Usage

```terraform
data "notion_workspace_export_status" "monthly_backup" {
  workspace_id  = data.notion_workspaces.org.workspaces[0].id
  export_job_id = "job-id-from-the-action-output"
}
```

## Schema

### Required

- `workspace_id` (String) The exported workspace.
- `export_job_id` (String) The job ID the action reported.

### Read-Only

- `status` (String) `in_progress`, `completed`, `url_generation_failed`, or `failed`.
- `export_format` (String) `html`, `markdown`, or `pdf`.
- `export_type` (String) `space`, `teamspace`, or `page`.
- `pages_exported` (Number)
- `created_time`, `last_edited_time` (Number) Epoch milliseconds.
- `download_urls` (List of String, Sensitive) Present only when `status` is `completed`.
- `url_expires_time` (Number) Epoch milliseconds.
- `error_type`, `error_message` (String) Set when the export failed.
