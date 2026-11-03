# Outputs only reference managed resources. An output built from a data
# source would change whenever the data does and fail the drift check.

output "showcase_url" {
  description = "Link to the showcase root page in Notion."
  value       = notion_page.root.url
}

output "root_page_id" {
  value = notion_page.root.id
}

output "page_ids" {
  description = "Child pages of the showcase root."
  value = {
    emoji         = notion_page.emoji.id
    locked        = notion_page.locked.id
    markdown      = notion_page.markdown.id
    markdown_v2   = notion_page.markdown_v2.id
    blocks        = notion_page.blocks.id
    placed        = notion_page.placed.id
    from_template = one(notion_page.from_template[*].id)
  }
}

output "database_ids" {
  value = {
    tasks    = notion_database.tasks.id
    projects = notion_database.projects.id
  }
}

output "data_source_ids" {
  description = "Data source IDs Notion reports for each database."
  value = {
    tasks    = notion_database.tasks.data_source_ids
    projects = notion_database.projects.data_source_ids
  }
}

output "view_id" {
  value = notion_view.tasks_table.id
}

output "comment_ids" {
  value = {
    rich_text = notion_comment.rich_text.id
    markdown  = notion_comment.markdown.id
    reply     = notion_comment.reply.id
  }
}
