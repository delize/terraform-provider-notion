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

Email addresses are unique within a Notion workspace, so to look up one user this data source paginates through `/v1/users` (100 users per request) and matches each against `email`, stopping the moment it finds the unique match. In a small workspace that's typically one API call; in a large one it may be several before the match is found.

If you need many users, prefer [`notion_users`](./users.md) — calling it once and filtering the result with Terraform `for` expressions is cheaper than calling `notion_user` repeatedly, since each `notion_user` call has to start scanning from the first page.

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
