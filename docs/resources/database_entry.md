---
page_title: "notion_database_entry Resource - Notion"
subcategory: ""
description: |-
  Manages an entry (row) in a Notion database.
---

# notion_database_entry (Resource)

Manages an entry (row) in a Notion database. Each entry has a title that corresponds to the database's title column, and can optionally set values for other column types using typed property maps.

~> **Note:** Destroying an entry archives it in Notion rather than permanently deleting it.

## Example Usage

### Basic

```terraform
resource "notion_database_entry" "first_task" {
  database = notion_database.tasks.id
  title    = "Set up Terraform"
}
```

### With Property Values

```terraform
resource "notion_database_entry" "office" {
  database = notion_database.offices.id
  title    = "Singapore Office"

  rich_text_properties = {
    "Notes" = "Primary APAC hub"
  }

  select_properties = {
    "Region" = "APAC"
  }

  number_properties = {
    "Capacity" = 250
  }

  checkbox_properties = {
    "Active" = true
  }

  url_properties = {
    "Website" = "https://example.com"
  }

  email_properties = {
    "Contact" = "office@example.com"
  }

  phone_number_properties = {
    "Phone" = "+65 1234 5678"
  }

  date_properties = {
    "Opened" = "2024-01-15"
  }

  status_properties = {
    "Status" = "Active"
  }
}
```

## Schema

### Required

- `database` (String) The ID of the parent database. Changing this forces a new resource.
- `title` (String) The title of the entry (value of the title column).

### Optional

- `rich_text_properties` (Map of String) Map of rich text property name to string value.
- `number_properties` (Map of Number) Map of number property name to numeric value.
- `checkbox_properties` (Map of Boolean) Map of checkbox property name to boolean value.
- `select_properties` (Map of String) Map of select property name to option name.
- `status_properties` (Map of String) Map of status property name to status name.
- `url_properties` (Map of String) Map of URL property name to URL value.
- `email_properties` (Map of String) Map of email property name to email value.
- `phone_number_properties` (Map of String) Map of phone number property name to phone number value.
- `date_properties` (Map of String) Map of date property name to ISO 8601 date string (e.g. `2024-01-15` or `2024-01-15T10:30:00Z`).

### Read-Only

- `id` (String) The ID of the entry.
- `url` (String) The URL of the entry in Notion.

~> **Note:** Only properties included in the maps are managed by Terraform. Removing a key from a map during an update will clear that property's value in Notion. Properties not present in any map are left untouched.

## Import

Database entries can be imported using their Notion page ID:

```shell
terraform import notion_database_entry.first_task <entry-id>
```
