---
page_title: "notion_database_property_number Resource - Notion"
subcategory: ""
description: |-
  Manages a number property on a Notion database.
---

# notion_database_property_number (Resource)

Manages a number property on a Notion database. Number properties support various display formats including plain numbers, percentages, and currencies.

## Example Usage

```terraform
resource "notion_database_property_number" "priority" {
  database = notion_database.tasks.id
  name     = "Priority"
  format   = "number"
}

resource "notion_database_property_number" "budget" {
  database = notion_database.tasks.id
  name     = "Budget"
  format   = "dollar"
}
```

## Schema

### Required

- `database` (String) The ID of the parent database. Changing this forces a new resource.
- `name` (String) The name of the property. Changing this forces a new resource.
- `format` (String) The number format. Valid values: `number`, `number_with_commas`, `percent`, `dollar`, `canadian_dollar`, `euro`, `pound`, `yen`, `ruble`, `rupee`, `won`, `yuan`, `real`, `lira`, `rupiah`, `franc`, `hong_kong_dollar`, `new_zealand_dollar`, `krona`, `norwegian_krone`, `mexican_peso`, `rand`, `new_taiwan_dollar`, `danish_krone`, `zloty`, `baht`, `forint`, `koruna`, `shekel`, `chilean_peso`, `philippine_peso`, `dirham`, `colombian_peso`, `riyal`, `ringgit`, `leu`, `argentine_peso`, `uruguayan_peso`, `singapore_dollar`.

### Read-Only

- `id` (String) The ID of the property.

## Import

Number properties can be imported using a composite ID:

```shell
terraform import notion_database_property_number.priority <database-id>/<property-name>
```
