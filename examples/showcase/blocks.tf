# One notion_block per supported type, on their own page. Each block is
# placed with `after` so the page reads top to bottom.
#
# Blocks whose type has a color set color = "default" explicitly. Notion
# reports "default" for an uncolored block, and the attribute defaults to an
# empty string, so leaving it out would fail the apply on the SDK-backed
# types.

resource "notion_page" "blocks" {
  parent_page_id = notion_page.root.id
  title          = "Blocks"
  icon           = "🧱"

  depends_on = [notion_page.markdown]
}

resource "notion_block" "toc" {
  parent_id = notion_page.blocks.id
  type      = "table_of_contents"
  color     = "gray"
}

# Headings

resource "notion_block" "heading_1" {
  parent_id = notion_page.blocks.id
  type      = "heading_1"
  rich_text = "Heading 1 in blue"
  color     = "blue"
  after     = notion_block.toc.id
}

resource "notion_block" "heading_2" {
  parent_id     = notion_page.blocks.id
  type          = "heading_2"
  rich_text     = "Heading 2, toggleable"
  color         = "green"
  is_toggleable = true
  after         = notion_block.heading_1.id
}

resource "notion_block" "heading_2_child" {
  parent_id = notion_block.heading_2.id
  type      = "paragraph"
  rich_text = "A toggleable heading can hold blocks. This one is parented on the heading."
  color     = "default"
}

resource "notion_block" "heading_3" {
  parent_id = notion_page.blocks.id
  type      = "heading_3"
  rich_text = "Heading 3 on an orange background"
  color     = "orange_background"
  after     = notion_block.heading_2.id
}

resource "notion_block" "heading_4" {
  parent_id     = notion_page.blocks.id
  type          = "heading_4"
  rich_text     = "Heading 4, toggleable and purple"
  color         = "purple"
  is_toggleable = true
  after         = notion_block.heading_3.id
}

# Text and lists

resource "notion_block" "paragraph" {
  parent_id = notion_page.blocks.id
  type      = "paragraph"
  rich_text = "A paragraph with a [link to the Notion API reference](https://developers.notion.com/reference/intro) in the middle of it."
  color     = "default"
  after     = notion_block.heading_4.id
}

resource "notion_block" "paragraph_colored" {
  parent_id = notion_page.blocks.id
  type      = "paragraph"
  rich_text = "A paragraph on a blue background."
  color     = "blue_background"
  after     = notion_block.paragraph.id
}

resource "notion_block" "bullet_1" {
  parent_id = notion_page.blocks.id
  type      = "bulleted_list_item"
  rich_text = "First bulleted item"
  color     = "default"
  after     = notion_block.paragraph_colored.id
}

resource "notion_block" "bullet_2" {
  parent_id = notion_page.blocks.id
  type      = "bulleted_list_item"
  rich_text = "Second bulleted item, in red"
  color     = "red"
  after     = notion_block.bullet_1.id
}

resource "notion_block" "number_1" {
  parent_id = notion_page.blocks.id
  type      = "numbered_list_item"
  rich_text = "First numbered item"
  color     = "default"
  after     = notion_block.bullet_2.id
}

resource "notion_block" "number_2" {
  parent_id = notion_page.blocks.id
  type      = "numbered_list_item"
  rich_text = "Second numbered item"
  color     = "default"
  after     = notion_block.number_1.id
}

resource "notion_block" "todo_done" {
  parent_id = notion_page.blocks.id
  type      = "to_do"
  rich_text = "A checked to-do"
  checked   = true
  color     = "default"
  after     = notion_block.number_2.id
}

resource "notion_block" "todo_open" {
  parent_id = notion_page.blocks.id
  type      = "to_do"
  rich_text = "An unchecked to-do"
  checked   = false
  color     = "default"
  after     = notion_block.todo_done.id
}

# Containers and callouts

resource "notion_block" "toggle" {
  parent_id = notion_page.blocks.id
  type      = "toggle"
  rich_text = "A toggle with a nested block"
  color     = "default"
  after     = notion_block.todo_open.id
}

resource "notion_block" "toggle_child" {
  parent_id = notion_block.toggle.id
  type      = "paragraph"
  rich_text = "This paragraph is parented on the toggle."
  color     = "default"
}

resource "notion_block" "quote" {
  parent_id = notion_page.blocks.id
  type      = "quote"
  rich_text = "A quote block. Plans are cheap, applies are forever."
  color     = "default"
  after     = notion_block.toggle.id
}

resource "notion_block" "callout" {
  parent_id = notion_page.blocks.id
  type      = "callout"
  rich_text = "A callout with an emoji icon on a yellow background."
  icon      = "💡"
  color     = "yellow_background"
  after     = notion_block.quote.id
}

# Code and math

resource "notion_block" "code" {
  parent_id = notion_page.blocks.id
  type      = "code"
  rich_text = "terraform apply -auto-approve\nterraform plan -detailed-exitcode"
  language  = "shell"
  caption   = "The two commands CI runs against this config"
  after     = notion_block.callout.id
}

resource "notion_block" "equation" {
  parent_id  = notion_page.blocks.id
  type       = "equation"
  expression = "E = mc^2"
  after      = notion_block.code.id
}

resource "notion_block" "divider" {
  parent_id = notion_page.blocks.id
  type      = "divider"
  after     = notion_block.equation.id
}

# Links and media

resource "notion_block" "bookmark" {
  parent_id = notion_page.blocks.id
  type      = "bookmark"
  url       = "https://www.notion.so"
  caption   = "A bookmark block"
  after     = notion_block.divider.id
}

resource "notion_block" "embed" {
  parent_id = notion_page.blocks.id
  type      = "embed"
  url       = "https://developers.notion.com"
  after     = notion_block.bookmark.id
}

resource "notion_block" "image" {
  parent_id = notion_page.blocks.id
  type      = "image"
  url       = "https://upload.wikimedia.org/wikipedia/commons/4/45/Notion_app_logo.png"
  caption   = "An external image from Wikimedia Commons"
  after     = notion_block.embed.id
}

# Tabs. Each tab item is a paragraph block whose id is exported, so a tab's
# content is parented on that id.

resource "notion_block" "tabs" {
  parent_id = notion_page.blocks.id
  type      = "tab"
  after     = notion_block.image.id

  tab_items = [
    { label = "Overview", icon = "📋" },
    { label = "Details", icon = "🔍" },
    { label = "Empty", icon = "📭" },
  ]
}

resource "notion_block" "tab_overview_text" {
  parent_id = notion_block.tabs.tab_items[0].id
  type      = "paragraph"
  rich_text = "Content inside the Overview tab."
  color     = "default"
}

resource "notion_block" "tab_details_item" {
  parent_id = notion_block.tabs.tab_items[1].id
  type      = "bulleted_list_item"
  rich_text = "Content inside the Details tab."
  color     = "default"
}

# Synced blocks. The original holds the content, and the copy mirrors it.

resource "notion_block" "synced_original" {
  parent_id = notion_page.blocks.id
  type      = "synced_block"
  after     = notion_block.tabs.id
}

resource "notion_block" "synced_content" {
  parent_id = notion_block.synced_original.id
  type      = "paragraph"
  rich_text = "This paragraph lives in the original synced block and shows up in its copy."
  color     = "default"
}

resource "notion_block" "synced_copy" {
  parent_id   = notion_page.blocks.id
  type        = "synced_block"
  synced_from = notion_block.synced_original.id
  after       = notion_block.synced_original.id

  depends_on = [notion_block.synced_content]
}

resource "notion_block" "columns_note" {
  parent_id = notion_page.blocks.id
  type      = "callout"
  icon      = "🧩"
  color     = "gray_background"
  rich_text = "No column_list here. Notion only accepts a column list together with its columns and their content, and notion_block creates one block at a time. The Markdown page shows columns instead."
  after     = notion_block.synced_copy.id
}
