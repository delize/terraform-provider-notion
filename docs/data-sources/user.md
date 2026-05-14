---
page_title: "notion_user Data Source - Notion"
subcategory: ""
description: |-
  Look up a Notion user by email address.
---

# notion_user (Data Source)

Use this data source to look up a Notion workspace user by their email address.

## Implementation note

Notion's [`/v1/users`](https://developers.notion.com/reference/get-users) API does **not** support filtering, sorting, or searching server-side — its docs explicitly state: *"The API does not currently support filtering users by their email and/or name."*

This data source therefore paginates through all users (100 per request) and filters client-side, short-circuiting as soon as it finds a match. For workspaces with many users this can mean several API calls per `Read` even though only one user is returned. To list multiple users, use the [`notion_users`](./users.md) data source — fetching the full set once and filtering with Terraform `for` expressions is cheaper than calling `notion_user` repeatedly.

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
