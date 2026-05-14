---
page_title: "notion_users Data Source - Notion"
subcategory: ""
description: |-
  List all users in the Notion workspace.
---

# notion_users (Data Source)

List every user (people and bots) in the Notion workspace that the integration has access to. Wraps the [`/v1/users`](https://developers.notion.com/reference/get-users) endpoint and paginates through all results.

## Example Usage

```terraform
data "notion_users" "all" {}

output "people_emails" {
  value = [
    for u in data.notion_users.all.users :
    u.email if u.type == "person" && u.email != ""
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
