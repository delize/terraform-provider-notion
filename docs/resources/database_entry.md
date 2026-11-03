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

### With Markdown Links

The `title` and `rich_text_properties` values support markdown link syntax. Links render as clickable hyperlinks in Notion.

```terraform
resource "notion_database_entry" "linked" {
  database = notion_database.references.id
  title    = "ISO-3166"

  rich_text_properties = {
    "Description" = "Country codes based on the [ISO-3166](https://en.wikipedia.org/wiki/ISO_3166-1) standard."
  }
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

### With Multi-Select, Relation, and People Values

```terraform
resource "notion_database_entry" "launch" {
  database = notion_database.projects.id
  title    = "Launch"

  multi_select_properties = {
    "Tags" = ["Infra", "Q3"]
  }

  relation_properties = {
    "Offices" = [notion_database_entry.office.id]
  }

  people_properties = {
    "Owner" = [data.notion_user.lead.id]
  }
}
```

## List values

`multi_select_properties`, `relation_properties`, and `people_properties` map a property name to a list. Each list replaces the whole value in Notion, so values added in the Notion UI are removed on the next apply that writes that property. An empty list clears the value, and so does removing the key. Notion accepts at most 100 items per property in one request, and the provider rejects longer lists at plan time.

Order doesn't matter. When Notion holds the same items as the config, the config's order is kept in state. Relation and people IDs may be written with or without hyphens. When Notion's value matches the config, the config's form is kept, and an ID Notion added on its own is stored without hyphens.

Writing a multi-select option name that the property doesn't have yet makes Notion add it to the property's options, as long as the integration can edit the database. If the property is managed by `notion_database_property_multi_select`, list the option there too, or that resource will show the new option as drift on its next plan. Option names can't contain commas.

A page response carries at most 25 relation or people references. When a managed relation or people value has 25 or more, the provider reads the full list from Notion's [page property item endpoint](https://developers.notion.com/reference/retrieve-a-page-property) during refresh, so long lists don't show a diff. Related pages the integration can't access may be left out of the value Notion returns, which shows up as a diff.

### With Markdown Content

```terraform
resource "notion_database_entry" "meeting" {
  database         = notion_database.meetings.id
  title            = "Weekly sync"
  markdown_version = "v2"
  markdown         = "## Agenda\n\n- Roadmap\n- Hiring"
}
```

## Markdown versions

Notion accepts two markdown syntaxes. `v1` is [enhanced markdown](https://developers.notion.com/guides/data-apis/enhanced-markdown), the original format, which Notion has deprecated. `v2` is [Notion-flavored Markdown](https://developers.notion.com/guides/data-apis/notion-flavored-markdown), which Notion recommends. The next Notion API version will make `v2` the default and reject `v1` with a 400 error, and Notion plans to retire `v1` about three months after that version ships.

Set `markdown_version` on the resource, or on the provider block to cover every page and database entry at once. The resource value wins. When neither is set, the provider leaves the field out of its requests and Notion uses `v1`, which is how earlier versions of the provider behaved. The `notion_page_markdown` data source has its own `markdown_version` and otherwise follows the provider block.

The same text can mean different things in the two syntaxes, so moving to `v2` means rewriting the markdown string, not only setting the attribute. Notion's [migration guide](https://developers.notion.com/guides/data-apis/migrate-to-notion-flavored-markdown) lists every change. These are the ones most likely to affect a config.

- In `v2` a single newline doesn't start a new block. Separate blocks with a blank line, or the lines join into one paragraph.
- Children are no longer indented with tabs, except in lists. A block with children uses its tag form, such as `<toggle>` or `<quote>`, with the children inside the tag after a blank line.
- `{color="blue"}` suffixes become a `color` attribute on the block's tag. `v2` saves the old suffix as literal text and doesn't warn about it.
- `<br>` becomes `<br />`, `<details>` becomes `<toggle>`, and several other block tags change name or attributes.
- `v2` rejects input it can't turn into blocks with a 400 error whose message starts with `Markdown errors blocked parsing`. `v1` saved the same input as text.

Change `markdown_version` and the `markdown` string in the same apply. A change to the effective version, whether it comes from the resource or the provider block, replaces the page body once with the current `markdown`, even when the text itself didn't change. Going from unset to an explicit `"v1"`, or back, isn't a change, since both mean `v1`. Setting a provider default shows a plan diff only on resources that set `markdown`.

When Notion accepts the markdown but changes or ignores part of it, for example converting a `v1` tag or saving an unknown tag as text, each of Notion's warnings shows up as a Terraform warning. The apply still succeeds.

### When the body is rewritten

The page body is replaced (Notion's `replace_content`) only when `markdown` changes or the effective `markdown_version` changes. Updating anything else leaves the body alone. Earlier versions of the provider resent the markdown on every update, so a title change also overwrote edits made in the Notion UI.

A replace overwrites everything on the entry's page with the configured markdown, including edits made in the Notion UI. Child pages and databases that aren't in the markdown can be deleted, because the provider allows Notion to delete content during a replace. Replacing content may also drop pending suggested edits on the entry. Notion doesn't document this either way.

## Schema

### Required

- `database` (String) The ID of the parent database. Changing this forces a new resource.
- `title` (String) The title of the entry (value of the title column). Supports markdown links: `[text](url)`.

### Optional

- `markdown` (String) Body content of the entry's page as markdown in the syntax set by `markdown_version`. Full-rewrite semantics (`replace_content`), sent only when the markdown or its version changes. Not read back from Notion, since Notion normalizes markdown. The typed property maps are sent in the same create request as the markdown. Earlier provider versions sent only the title there, so properties stayed unset until the next update.
- `markdown_version` (String) Markdown syntax for `markdown`: `v2` (Notion-flavored Markdown) or `v1` (enhanced markdown, deprecated). Overrides the provider's `markdown_version`. When neither is set, the field is left out and Notion uses `v1`. Records the provider default when only that is set. Changing the effective version replaces the body once. See [Markdown versions](#markdown-versions).
- `rich_text_properties` (Map of String) Map of rich text property name to string value. Values support markdown links: `[text](url)`.
- `number_properties` (Map of Number) Map of number property name to numeric value. Removing a key from this or any other property map clears that value in Notion.
- `checkbox_properties` (Map of Boolean) Map of checkbox property name to boolean value.
- `select_properties` (Map of String) Map of select property name to option name.
- `status_properties` (Map of String) Map of status property name to status name. Removing a key resets the status to the property's default option, which is how Notion clears a status.
- `url_properties` (Map of String) Map of URL property name to URL value.
- `email_properties` (Map of String) Map of email property name to email value.
- `phone_number_properties` (Map of String) Map of phone number property name to phone number value.
- `date_properties` (Map of String) Map of date property name to ISO 8601 date string (e.g. `2024-01-15` or `2024-01-15T10:30:00Z`).
- `multi_select_properties` (Map of List of String) Map of multi-select property name to the list of selected option names. A name that isn't an option yet is added to the property by Notion. At most 100 names per property. See [List values](#list-values).
- `relation_properties` (Map of List of String) Map of relation property name to the list of related page IDs, with or without hyphens. At most 100 IDs per property. See [List values](#list-values).
- `people_properties` (Map of List of String) Map of people property name to the list of user IDs, with or without hyphens. Bot user IDs are accepted, though Notion notes that its own internal bots may not be assignable. At most 100 IDs per property. See [List values](#list-values).

### Read-Only

- `id` (String) The ID of the entry.
- `url` (String) The URL of the entry in Notion.

~> **Note:** Only properties included in the maps are managed by Terraform. Removing a key from a map during an update will clear that property's value in Notion. Properties not present in any map are left untouched.

## Import

Database entries can be imported using their Notion page ID:

```shell
terraform import notion_database_entry.first_task <entry-id>
```

Import reads the title, but no typed property maps, because the provider can't tell which properties the config manages. The first apply after an import writes every map from config.
