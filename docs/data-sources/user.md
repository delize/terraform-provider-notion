---
page_title: "notion_user Data Source - Notion"
subcategory: ""
description: |-
  Look up a Notion user by email address.
---

# notion_user (Data Source)

Use this data source to look up a Notion workspace user by their email address.

## Example Usage

```terraform
data "notion_user" "admin" {
  email = "admin@example.com"
}

output "admin_name" {
  value = data.notion_user.admin.name
}
```

## Schema

### Required

- `email` (String) The email address of the user to look up.

### Read-Only

- `id` (String) The ID of the user.
- `name` (String) The display name of the user.
- `user_id` (String) The Notion user ID.
