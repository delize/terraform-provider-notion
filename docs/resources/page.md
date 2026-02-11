---
page_title: "notion_page Resource - Notion"
subcategory: ""
description: |-
  Manages a Notion page.
---

# notion_page (Resource)

Manages a Notion page. Pages can be created under an existing parent page.

~> **Note:** Destroying a page archives it in Notion rather than permanently deleting it, as the Notion API does not support hard deletes.

## Example Usage

```terraform
resource "notion_page" "example" {
  parent_page_id = "your-parent-page-id"
  title          = "My Terraform Page"
}
```

## Schema

### Required

- `parent_page_id` (String) The ID of the parent page. Changing this forces a new resource.
- `title` (String) The title of the page.

### Read-Only

- `id` (String) The ID of the page.
- `url` (String) The URL of the page in Notion.

## Import

Pages can be imported using their Notion page ID:

```shell
terraform import notion_page.example <page-id>
```
