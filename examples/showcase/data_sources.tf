# Every non-Enterprise data source, read back after the resources exist,
# with a summary of each written onto the root page as a JSON code block.
#
# The summaries are snapshots of the apply that built the page. Search
# results, users, and similar listings change on their own, so the code
# blocks ignore later changes to rich_text and the drift check stays clean.

data "notion_users" "all" {}

data "notion_search" "showcase_pages" {
  query         = "Provider showcase"
  filter_object = "page"
  in_trash      = false

  depends_on = [notion_page.root]
}

# Search-backed, and Notion's search index lags behind new pages, so this
# looks up a page that already exists. Skipped unless lookup_page_query is set.
data "notion_page" "lookup" {
  count = local.lookup_page_query == "" ? 0 : 1
  query = local.lookup_page_query
}

data "notion_page_markdown" "markdown_page" {
  page_id = notion_page.markdown.id
}

data "notion_blocks" "blocks_page" {
  parent_id = notion_page.blocks.id

  depends_on = [
    notion_block.columns_note,
    notion_block.heading_2_child,
    notion_block.toggle_child,
    notion_block.tab_overview_text,
    notion_block.tab_details_item,
    notion_block.synced_content,
  ]
}

data "notion_database_entries" "unfinished_tasks" {
  database = notion_database.tasks.id

  filter = jsonencode({
    property = "Status"
    status   = { does_not_equal = "Done" }
  })
  sorts = jsonencode([
    { property = "Estimate", direction = "descending" },
  ])

  depends_on = [
    notion_database_entry.task_config,
    notion_database_entry.task_workflow,
    notion_database_entry.task_cleanup,
    notion_database_entry.task_docs,
  ]
}

data "notion_view_query" "tasks_table" {
  view_id   = notion_view.tasks_table.id
  page_size = 10

  depends_on = [
    notion_database_entry.task_config,
    notion_database_entry.task_workflow,
    notion_database_entry.task_cleanup,
    notion_database_entry.task_docs,
  ]
}

data "notion_custom_emojis" "all" {}

data "notion_database_templates" "tasks" {
  database_id = notion_database.tasks.id
}

locals {
  users       = data.notion_users.all.users
  user_types  = distinct([for u in local.users : u.type])
  view_result = jsondecode(data.notion_view_query.tasks_table.raw_json)
  view_rows   = try(local.view_result.results, [])
  md          = data.notion_page_markdown.markdown_page.markdown
  blocks      = data.notion_blocks.blocks_page.blocks
  block_types = distinct([for b in local.blocks : b.type])

  summaries = {
    users = jsonencode({
      data_source     = "notion_users"
      count           = length(local.users)
      by_type         = { for t in local.user_types : t => length([for u in local.users : u if u.type == t]) }
      verified_emails = length([for u in local.users : u if u.email_verified == true])
      bot_names       = slice(sort([for u in local.users : u.name == null ? "" : u.name if u.type == "bot"]), 0, min(5, length([for u in local.users : u if u.type == "bot"])))
    })

    search = jsonencode({
      data_source = "notion_search"
      query       = "Provider showcase"
      in_trash    = false
      count       = length(data.notion_search.showcase_pages.results)
      note        = "Notion's search index catches up after a delay, so a page created in this apply may be missing."
      first_results = [
        for r in slice(data.notion_search.showcase_pages.results, 0, min(5, length(data.notion_search.showcase_pages.results))) : {
          object = r.object
          title  = substr(r.title == null ? "" : r.title, 0, 60)
        }
      ]
    })

    page = local.lookup_page_query == "" ? jsonencode({
      data_source = "notion_page"
      skipped     = "Set lookup_page_query to the title of a page that already exists."
      }) : jsonencode({
      data_source = "notion_page"
      query       = local.lookup_page_query
      id          = try(data.notion_page.lookup[0].id, null)
      title       = try(data.notion_page.lookup[0].title, null)
      url         = try(data.notion_page.lookup[0].url, null)
    })

    page_markdown = jsonencode({
      data_source = "notion_page_markdown"
      page        = notion_page.markdown.title
      characters  = length(local.md)
      lines       = length(split("\n", local.md))
      headings    = [for h in regexall("(?m)^#{1,4} .*$", local.md) : substr(h, 0, 80)]
    })

    blocks = jsonencode({
      data_source = "notion_blocks"
      page        = notion_page.blocks.title
      top_level   = length(local.blocks)
      by_type     = { for t in local.block_types : t => length([for b in local.blocks : b if b.type == t]) }
    })

    database_entries = jsonencode({
      data_source = "notion_database_entries"
      filter      = "Status is not Done"
      sort        = "Estimate, descending"
      count       = length(data.notion_database_entries.unfinished_tasks.entries)
      entries = [
        for e in data.notion_database_entries.unfinished_tasks.entries : {
          title    = e.title
          status   = lookup(e.properties, "Status", null)
          estimate = lookup(e.properties, "Estimate", null)
        }
      ]
    })

    view_query = jsonencode({
      data_source = "notion_view_query"
      view        = notion_view.tasks_table.name
      rows        = length(local.view_rows)
      has_more    = data.notion_view_query.tasks_table.has_more
      row_ids     = [for r in slice(local.view_rows, 0, min(5, length(local.view_rows))) : try(r.id, null)]
    })

    custom_emojis = jsonencode({
      data_source = "notion_custom_emojis"
      count       = length(data.notion_custom_emojis.all.emojis)
      names       = slice(sort([for e in data.notion_custom_emojis.all.emojis : e.name]), 0, min(10, length(data.notion_custom_emojis.all.emojis)))
    })

    database_templates = jsonencode({
      data_source    = "notion_database_templates"
      database       = notion_database.tasks.title
      data_source_id = data.notion_database_templates.tasks.data_source_id
      count          = length(data.notion_database_templates.tasks.templates)
      names          = [for t in data.notion_database_templates.tasks.templates : t.name]
    })
  }

  # Notion caps a rich text object at 2000 characters. The summaries are
  # built to stay well under that, and this is the backstop.
  summary_text = {
    for k, v in local.summaries : k => length(v) <= 1900 ? v : "${substr(v, 0, 1880)} (truncated)"
  }
}

resource "notion_block" "root_results_heading" {
  parent_id = notion_page.root.id
  type      = "heading_2"
  rich_text = "Data source results"

  depends_on = [notion_database.projects]
}

resource "notion_block" "root_results_intro" {
  parent_id = notion_page.root.id
  type      = "paragraph"
  rich_text = "Each code block below summarizes one data source, read during the apply that built this page. Captions name the data source."
  after     = notion_block.root_results_heading.id
}

resource "notion_block" "result_users" {
  parent_id = notion_page.root.id
  type      = "code"
  language  = "json"
  caption   = "data.notion_users.all"
  rich_text = local.summary_text["users"]
  after     = notion_block.root_results_intro.id

  lifecycle {
    ignore_changes = [rich_text]
  }
}

resource "notion_block" "result_search" {
  parent_id = notion_page.root.id
  type      = "code"
  language  = "json"
  caption   = "data.notion_search.showcase_pages"
  rich_text = local.summary_text["search"]
  after     = notion_block.result_users.id

  lifecycle {
    ignore_changes = [rich_text]
  }
}

resource "notion_block" "result_page" {
  parent_id = notion_page.root.id
  type      = "code"
  language  = "json"
  caption   = "data.notion_page.lookup"
  rich_text = local.summary_text["page"]
  after     = notion_block.result_search.id

  lifecycle {
    ignore_changes = [rich_text]
  }
}

resource "notion_block" "result_page_markdown" {
  parent_id = notion_page.root.id
  type      = "code"
  language  = "json"
  caption   = "data.notion_page_markdown.markdown_page"
  rich_text = local.summary_text["page_markdown"]
  after     = notion_block.result_page.id

  lifecycle {
    ignore_changes = [rich_text]
  }
}

resource "notion_block" "result_blocks" {
  parent_id = notion_page.root.id
  type      = "code"
  language  = "json"
  caption   = "data.notion_blocks.blocks_page"
  rich_text = local.summary_text["blocks"]
  after     = notion_block.result_page_markdown.id

  lifecycle {
    ignore_changes = [rich_text]
  }
}

resource "notion_block" "result_database_entries" {
  parent_id = notion_page.root.id
  type      = "code"
  language  = "json"
  caption   = "data.notion_database_entries.unfinished_tasks"
  rich_text = local.summary_text["database_entries"]
  after     = notion_block.result_blocks.id

  lifecycle {
    ignore_changes = [rich_text]
  }
}

resource "notion_block" "result_view_query" {
  parent_id = notion_page.root.id
  type      = "code"
  language  = "json"
  caption   = "data.notion_view_query.tasks_table"
  rich_text = local.summary_text["view_query"]
  after     = notion_block.result_database_entries.id

  lifecycle {
    ignore_changes = [rich_text]
  }
}

resource "notion_block" "result_custom_emojis" {
  parent_id = notion_page.root.id
  type      = "code"
  language  = "json"
  caption   = "data.notion_custom_emojis.all"
  rich_text = local.summary_text["custom_emojis"]
  after     = notion_block.result_view_query.id

  lifecycle {
    ignore_changes = [rich_text]
  }
}

resource "notion_block" "result_database_templates" {
  parent_id = notion_page.root.id
  type      = "code"
  language  = "json"
  caption   = "data.notion_database_templates.tasks"
  rich_text = local.summary_text["database_templates"]
  after     = notion_block.result_custom_emojis.id

  lifecycle {
    ignore_changes = [rich_text]
  }
}
