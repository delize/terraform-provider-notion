---
page_title: "notion_users Data Source - Notion"
subcategory: ""
description: |-
  List all users in the Notion workspace.
---

# notion_users (Data Source)

List every user (people and bots) in the Notion workspace that the integration has access to. Wraps the [`/v1/users`](https://developers.notion.com/reference/get-users) endpoint and paginates through all results.

## Filtering

The Notion REST API does **not** support server-side filtering, sorting, or searching on `/v1/users` — its docs explicitly state: *"The API does not currently support filtering users by their email and/or name."* Only `start_cursor` and `page_size` are accepted, and the API does not guarantee a particular sort order.

This data source therefore always fetches **all users** the integration can see, paginating through every page until the API reports `has_more = false`. Any narrowing is done client-side after the full query — either with Terraform `for` expressions (see example below) or by passing the result through downstream resources/locals.

For typical workspaces (hundreds to low thousands of users) this is fine. For very large workspaces, expect the data source's `Read` to make multiple API calls (one per 100 users) and to bring the full set into Terraform state.

If you only need a single user by email, use the existing [`notion_user`](./user.md) data source instead — it short-circuits as soon as the matching user is found.

## Example Usage

```terraform
data "notion_users" "all" {}

# Client-side filter: emails of all human users
output "people_emails" {
  value = [
    for u in data.notion_users.all.users :
    u.email if u.type == "person" && u.email != ""
  ]
}

# Client-side filter: only people with an @example.com email
locals {
  example_dot_com_users = [
    for u in data.notion_users.all.users :
    u if u.type == "person" && endswith(u.email, "@example.com")
  ]
}
```

## Schema

### Read-Only

- `users` (Attributes List) All users returned by the Notion API. (see [below for nested schema](#nestedatt--users))

<a id="nestedatt--users"></a>
### Nested Schema for `users`

Read-Only:

- `id` (String) The Notion user ID.
- `name` (String) The user's display name.
- `type` (String) The user type (`person` or `bot`).
- `email` (String) Email address for person-type users (empty for bots).
- `avatar_url` (String) URL of the user's avatar image, if set.
