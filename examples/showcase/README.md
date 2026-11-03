# Provider showcase

This config builds one Notion page that uses every feature of the provider that a normal (non-Enterprise) workspace can exercise. CI applies it with the real `terraform` CLI and leaves the result in place, so you can open the page in Notion and see what each resource produces. The acceptance tests can't show this, because every test destroys what it builds.

## What it builds

A root page titled `Provider showcase (<run label>)` under the parent page, created at the top of the parent with a native icon. It holds the following.

- Child pages. One has an emoji icon, one is locked, one is written in markdown and then extended with `markdown_insert`, one is created from `var.template_id` when that is set, and one is placed with `after_block_id` right after the Pages heading.
- A Blocks page with one `notion_block` per supported type, in order: table of contents, headings 1 to 4 with colors and toggles (including a block nested in a toggleable heading), paragraphs with a link and a background color, bulleted and numbered lists, checked and unchecked to-dos, a toggle with a nested block, a quote, a callout, a code block with a caption, an equation, a divider, a bookmark, an embed, an external image, a tab block with three tabs and content in two of them, and an original synced block with a copy.
- An inline Tasks database with a column for every database property resource (select, multi-select, number, status with option groups, checkbox, date, rich text, URL, email, people, created time and by, last edited time and by, relation, and rollup), four rows with values, one with a markdown body, and a table view.
- A full-page Projects database with a native icon and `is_locked`, which the relation and rollup point at, plus a number column and two rows.
- Comments on the root page. One uses `rich_text`, one uses `markdown`, and one is a reply in the first comment's discussion.
- A Data source results section with one JSON code block per data source. It reads `notion_users`, `notion_search` (with `in_trash = false`), `notion_page`, `notion_page_markdown`, `notion_blocks`, `notion_database_entries` (with a filter and sorts), `notion_view_query`, `notion_custom_emojis`, and `notion_database_templates`. The code blocks hold short summaries such as counts, titles, and IDs, so each one stays under Notion's 2000 character limit for a rich text object.

## What it leaves out, and why

- The Admin API resources, data sources, and actions need an Enterprise organization and an organization token.
- `notion_agent_settings` and the agent data sources need a Custom Agent, and the test workspace has none.
- `column_list` and `column` blocks. Notion only accepts a column list together with at least two columns that each have content, and `notion_block` creates one block per request. The markdown page shows columns instead.
- `notion_database` and `notion_meeting_notes` data sources. The first searches by title like `notion_page` and adds nothing new. The second depends on the workspace plan including AI meeting notes.
- Phone number values. `notion_database_entry` can set them, but there is no phone number property resource to create the column.
- Multi-select and relation values on rows. `notion_database_entry` doesn't set either, so the Tags, Project, and Project points columns exist but stay empty.
- `rich_text_json` on blocks and `filter`, `sorts`, or `configuration` on the view. All of them store the JSON Notion sends back, which may not match the JSON in the config byte for byte. Until that is confirmed against the API, using them here could fail the apply.

## Drift

A second `terraform plan` right after apply must report no changes, and CI fails if it doesn't. Three things are excluded on purpose.

- The data source code blocks ignore changes to `rich_text`. They are a snapshot of the apply, and listings such as search results and users change on their own.
- The template page ignores `icon`, because Notion applies the template after the page exists and the template may carry an emoji.
- `markdown_insert` runs once at create. Changing it appends again, so it stays fixed.

Outputs only reference managed resources, since an output built from a data source would change with the data.

## Running it locally

You need a Notion integration token with access to a parent page, Go, and Terraform 1.6 or newer.

Build the provider from the repository root.

```sh
go build -o "$HOME/.terraform-dev/notion/terraform-provider-notion" .
```

Point Terraform at that directory with a CLI config file.

```hcl
# ~/.terraform-dev/dev.tfrc
provider_installation {
  dev_overrides {
    "delize/notion" = "/Users/you/.terraform-dev/notion"
  }
  direct {}
}
```

Then apply from this directory. With dev_overrides there is nothing for `terraform init` to install, so skip it.

```sh
export TF_CLI_CONFIG_FILE="$HOME/.terraform-dev/dev.tfrc"
export NOTION_TOKEN="<integration token>"
export TF_VAR_parent_page_id="<parent page ID>"
# Optional
export TF_VAR_template_id="<template page ID>"
export TF_VAR_lookup_page_query="<title of a page that already exists>"

terraform apply
terraform plan -detailed-exitcode   # exit code 0 means no drift
```

`terraform destroy` moves everything to the Notion trash.

## In CI

`.github/workflows/showcase.yml` runs on manual dispatch, on release tags, and on same-repo pull requests that change this directory, the workflow, or the cleanup script. Each run first calls `scripts/showcase_cleanup.py`, which moves every child page of the parent whose title starts with `Provider showcase` to the trash. It touches nothing else, and trashed pages stay recoverable for 30 days. The run then applies a fresh copy, so the newest run always replaces the previous showcase. CI keeps no state between runs and never destroys the showcase it built.

The job summary links to the new page. The job shares the `notion-acceptance-workspace` concurrency group with the acceptance tests, so the two never use the workspace at the same time.

To clear old showcases by hand, run the script yourself. It reads `NOTION_TOKEN` and `TF_VAR_parent_page_id`.

```sh
python3 scripts/showcase_cleanup.py --dry-run
python3 scripts/showcase_cleanup.py
```
