---
page_title: "notion_database_property_phone_number Resource - Notion"
subcategory: ""
description: |-
  Manages a phone number property on a Notion database.
---

# notion_database_property_phone_number (Resource)

Manages a phone number property on a Notion database. Set values on entries with `phone_number_properties` on `notion_database_entry`. Notion stores the value as a plain string and doesn't enforce a phone number format.

## Example Usage

```terraform
resource "notion_database_property_phone_number" "phone" {
  database = notion_database.offices.id
  name     = "Phone"
}

resource "notion_database_entry" "singapore" {
  database = notion_database.offices.id
  title    = "Singapore Office"

  phone_number_properties = {
    "Phone" = "+65 1234 5678"
  }

  depends_on = [notion_database_property_phone_number.phone]
}
```

## Schema

### Required

- `database` (String) The ID of the parent database. Changing this forces a new resource.
- `name` (String) The name of the property. Changing this forces a new resource.

### Read-Only

- `id` (String) The ID of the property.

## Import

```shell
terraform import notion_database_property_phone_number.phone <database-id>/<property-name>
```
