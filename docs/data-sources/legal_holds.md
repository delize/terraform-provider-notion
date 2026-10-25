---
page_title: "notion_legal_holds Data Source - Notion"
subcategory: "Admin API"
description: |-
  Lists the organization's legal holds through the Notion Admin API.
---

# notion_legal_holds (Data Source)

Lists the organization's legal holds, both active and released, through the [Admin API](https://developers.notion.com/reference/admin/list-legal-holds). Needs `admin_token` with the `legal-hold:read` scope. The API doesn't guarantee an order.

## Example Usage

```terraform
data "notion_legal_holds" "for_ada" {
  user_id = "00000000-0000-0000-0000-000000000000"
}
```

## Schema

### Optional

- `user_id` (String) Only list holds that include this user.

### Read-Only

- `legal_holds` (List of Object) Each hold has `id`, `name`, `description`, `status`, `start_date`, `end_date`, `created_by`, `created_time`, `user_interaction_types`, `user_count`, and `workspace_count`. Dates are epoch milliseconds.
