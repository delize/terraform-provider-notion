---
page_title: "notion_legal_hold Resource - Notion"
subcategory: "Admin API"
description: |-
  Manages an organization legal hold and its users through the Notion Admin API.
---

# notion_legal_hold (Resource)

Manages an organization legal hold and the users under it through the [Admin API](https://developers.notion.com/reference/admin/create-legal-hold). Needs an Enterprise plan and `admin_token` with the `legal-hold:read` and `legal-hold:write` scopes.

~> **Destroying does not release the hold.** By default, removing this resource (or replacing it) only removes it from Terraform state and leaves the hold active in Notion, with a warning. Releasing a hold is a compliance decision. To release on destroy, set `release_on_destroy = true`, apply, then destroy. That also needs the `legal-hold:write-high-impact` scope.

The API has no way to change a hold's dates or preserved activity types, so changing `start_date`, `end_date`, or `user_interaction_types` creates a new hold. With the default `release_on_destroy = false`, the old hold stays active.

Dates are numbers. Neither the legal hold reference nor the [Help Center article](https://www.notion.com/help/legal-holds) states a unit, and the provider passes them through as epoch milliseconds, the unit every other Admin API endpoint documents.

What the Help Center adds:

- Holds created through the API work like holds made in organization settings (Manage organization, Data & Compliance, Legal holds) and show up in the same list.
- A hold covers every workspace in the organization. It can reach back to the workspace's Enterprise upgrade date or May 2022, so an earlier `start_date` preserves nothing extra.
- `user_ids` can include workspace owners, members, and guests, plus users removed after August 1, 2024.
- The settings page offers three scopes, which correspond to these `user_interaction_types` values: "Created only" is `page.created`, "Edited & created" adds `page.edited`, and "Viewed, edited & created" adds `page.viewed`.
- Releasing a hold removes its users and turns the normal retention settings back on for the held pages. Hold creation, release, membership changes, and exports appear in the audit log.

## Example Usage

```terraform
resource "notion_legal_hold" "smith_v_acme" {
  name                   = "Smith v. Acme"
  description            = "Preserve product team activity from March 2026"
  start_date             = 1772323200000 # 2026-03-01T00:00:00Z
  user_interaction_types = ["page.created", "page.edited", "page.viewed"]
  user_ids               = [for u in data.notion_admin_users.org.users : u.id if u.email == "ada@example.com"]
}
```

## Schema

### Required

- `name` (String) The hold's name. Changes in place.
- `start_date` (Number) When the hold starts, as epoch milliseconds. Changing it forces a new hold.
- `user_interaction_types` (Set of String) Activity the hold preserves: `page.created`, `page.edited`, `page.viewed`. Changing it forces a new hold.
- `user_ids` (Set of String) Users under the hold. Adding or removing IDs updates the hold in place.

### Optional

- `description` (String) Changes in place. Removing it clears the description.
- `icon` (String) Changes in place.
- `end_date` (Number) When the hold ends, as epoch milliseconds. Changing it forces a new hold.
- `release_on_destroy` (Boolean) Release the hold in Notion when the resource is destroyed or replaced. Defaults to `false`.

### Read-Only

- `id` (String) The legal hold ID.
- `status` (String) `active` or `released`.
- `created_by` (String) ID of the user or bot that created the hold.
- `created_time` (Number) Creation time, as epoch milliseconds.
- `workspace_ids` (Set of String) Workspaces the hold covers.

## Import

```shell
terraform import notion_legal_hold.smith_v_acme <legal-hold-id>
```

Imported holds get `release_on_destroy = false`.
