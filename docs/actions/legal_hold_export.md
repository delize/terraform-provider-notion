---
page_title: "notion_legal_hold_export Action - Notion"
subcategory: "Admin API"
description: |-
  Starts or continues an export of a legal hold's content.
---

# notion_legal_hold_export (Action)

Starts an export of a legal hold's content for one workspace, or continues an earlier export, through the [Admin API](https://developers.notion.com/reference/admin/export-legal-hold). Needs Terraform 1.14 or later and `admin_token` with the `legal-hold:export` scope.

Actions run only when invoked, never during plan or destroy. The export ID Notion returns appears as a progress message, since actions have no outputs. The API reference doesn't describe how the export is delivered. In the settings UI, according to the [Help Center](https://www.notion.com/help/legal-holds), PDF exports of page content are downloaded from the hold's Exports list and stay valid for 30 days, while CSV metadata exports arrive as an email link. Expect an API export to land in the same Exports list, though Notion doesn't say so directly.

## Example Usage

```terraform
action "notion_legal_hold_export" "smith_v_acme" {
  config {
    legal_hold_id      = notion_legal_hold.smith_v_acme.id
    requesting_user_id = "00000000-0000-0000-0000-000000000000"
    workspace_id       = data.notion_workspaces.org.workspaces[0].id
  }
}
```

```shell
terraform apply -invoke=action.notion_legal_hold_export.smith_v_acme
```

## Schema

### Required

- `legal_hold_id` (String) The legal hold to export.
- `requesting_user_id` (String) The user the export is requested on behalf of.

### Optional

Set exactly one of these.

- `workspace_id` (String) Workspace to export. Starts a new export.
- `legal_hold_export_id` (String) An export ID from an earlier run. Continues that export.
