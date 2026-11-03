# The root page and everything that sits directly on it. Blocks and pages
# are appended to the end of their parent as they are created, so the
# depends_on chains below exist to make the root page read top to bottom.

locals {
  title             = "Provider showcase (${var.run_label})"
  template_id       = var.template_id == null ? "" : trimspace(var.template_id)
  lookup_page_query = var.lookup_page_query == null ? "" : trimspace(var.lookup_page_query)
  repo_url          = "https://github.com/delize/terraform-provider-notion"
}

resource "notion_page" "root" {
  parent_page_id = var.parent_page_id
  title          = local.title
  icon_name      = "pizza"
  icon_color     = "blue"
  position       = "page_start"
}

resource "notion_block" "root_intro" {
  parent_id = notion_page.root.id
  type      = "callout"
  icon      = "🤖"
  color     = "gray_background"
  rich_text = "Built by the showcase workflow with the terraform CLI. Everything on this page is a Terraform resource, and the next run moves it to the trash and builds a fresh copy. Source is in [examples/showcase](${local.repo_url}/tree/main/examples/showcase)."
}

# Pages

resource "notion_block" "root_pages_heading" {
  parent_id = notion_page.root.id
  type      = "heading_2"
  rich_text = "Pages"
  color     = "default"
  after     = notion_block.root_intro.id
}

resource "notion_page" "emoji" {
  parent_page_id = notion_page.root.id
  title          = "Child page with an emoji icon"
  icon           = "🌱"

  depends_on = [notion_block.root_pages_heading]
}

resource "notion_page" "locked" {
  parent_page_id = notion_page.root.id
  title          = "Locked page"
  icon           = "🔒"
  is_locked      = true

  depends_on = [notion_page.emoji]
}

resource "notion_page" "markdown" {
  parent_page_id = notion_page.root.id
  title          = "Markdown page"
  icon           = "📝"
  markdown       = file("${path.module}/content/markdown-page.md")

  # An imperative append that runs once at create. Changing it would
  # append again, so it stays fixed.
  markdown_insert = {
    content  = "## Added with markdown_insert\n\nThis section was appended to the end of the page after it was created."
    position = "end"
  }

  depends_on = [notion_page.locked]
}

# The blocks page and its content live in blocks.tf.

resource "notion_page" "from_template" {
  count = local.template_id == "" ? 0 : 1

  parent_page_id = notion_page.root.id
  title          = "Page created from a template"
  template_id    = local.template_id

  # Notion applies the template after the page is created, and a template
  # with an emoji icon would then show up as drift on icon.
  lifecycle {
    ignore_changes = [icon]
  }

  depends_on = [notion_page.blocks]
}

resource "notion_page" "placed" {
  parent_page_id = notion_page.root.id
  title          = "Placed right after the Pages heading"
  icon           = "📌"
  after_block_id = notion_block.root_pages_heading.id

  depends_on = [notion_page.blocks, notion_page.from_template]
}

# Databases (defined in databases.tf) follow this heading.

resource "notion_block" "root_databases_heading" {
  parent_id = notion_page.root.id
  type      = "heading_2"
  rich_text = "Databases"
  color     = "default"

  depends_on = [notion_page.placed]
}

# Comments

resource "notion_comment" "rich_text" {
  parent_page_id = notion_page.root.id
  rich_text      = "A plain rich_text comment, created by notion_comment."
}

resource "notion_comment" "markdown" {
  parent_page_id = notion_page.root.id
  markdown       = "A **markdown** comment with `inline code` and a [link](https://developers.notion.com/reference/create-a-comment)."

  depends_on = [notion_comment.rich_text]
}

resource "notion_comment" "reply" {
  discussion_id = notion_comment.rich_text.discussion_id
  markdown      = "A reply in the first comment's discussion, set through *discussion_id*."
}
