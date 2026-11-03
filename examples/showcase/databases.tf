# An inline Tasks database with one column of every property type the
# provider manages, and a full-page Projects database that the relation and
# rollup point at.
#
# Each property resource sends its own schema update to the same database.
# They are chained with depends_on so the updates go one at a time instead
# of racing each other.

resource "notion_database" "tasks" {
  parent             = notion_page.root.id
  title              = "Tasks (inline database)"
  title_column_title = "Task"
  is_inline          = true

  depends_on = [notion_block.root_databases_heading]
}

resource "notion_database" "projects" {
  parent             = notion_page.root.id
  title              = "Projects (full-page, locked)"
  title_column_title = "Project"
  icon_name          = "token"
  icon_color         = "purple"
  is_locked          = true

  depends_on = [notion_database.tasks]
}

# Projects columns and rows. The lock only applies in the Notion app, so the
# API can still change the schema and add rows.

resource "notion_database_property_number" "project_points" {
  database = notion_database.projects.id
  name     = "Points"
  format   = "number"
}

resource "notion_database_entry" "project_docs" {
  database          = notion_database.projects.id
  title             = "Documentation"
  number_properties = { "Points" = 8 }

  depends_on = [notion_database_property_number.project_points]
}

resource "notion_database_entry" "project_ci" {
  database          = notion_database.projects.id
  title             = "Continuous integration"
  number_properties = { "Points" = 13 }

  depends_on = [notion_database_property_number.project_points]
}

# Tasks columns

resource "notion_database_property_select" "priority" {
  database = notion_database.tasks.id
  name     = "Priority"
  options = {
    "Low"    = "gray"
    "Medium" = "yellow"
    "High"   = "red"
  }
}

resource "notion_database_property_multi_select" "tags" {
  database = notion_database.tasks.id
  name     = "Tags"
  options = {
    "Docs"  = "blue"
    "Infra" = "purple"
    "Bug"   = "red"
  }

  depends_on = [notion_database_property_select.priority]
}

resource "notion_database_property_number" "estimate" {
  database = notion_database.tasks.id
  name     = "Estimate"
  format   = "number"

  depends_on = [notion_database_property_multi_select.tags]
}

resource "notion_database_property_status" "status" {
  database = notion_database.tasks.id
  name     = "Status"
  options = {
    "Not started" = "default"
    "In progress" = "blue"
    "Blocked"     = "red"
    "Done"        = "green"
  }
  option_groups = {
    "In progress" = "In progress"
    "Blocked"     = "In progress"
    "Done"        = "Complete"
  }

  depends_on = [notion_database_property_number.estimate]
}

resource "notion_database_property_checkbox" "reviewed" {
  database = notion_database.tasks.id
  name     = "Reviewed"

  depends_on = [notion_database_property_status.status]
}

resource "notion_database_property_date" "due" {
  database = notion_database.tasks.id
  name     = "Due"

  depends_on = [notion_database_property_checkbox.reviewed]
}

resource "notion_database_property_rich_text" "notes" {
  database = notion_database.tasks.id
  name     = "Notes"

  depends_on = [notion_database_property_date.due]
}

resource "notion_database_property_url" "link" {
  database = notion_database.tasks.id
  name     = "Link"

  depends_on = [notion_database_property_rich_text.notes]
}

resource "notion_database_property_email" "contact" {
  database = notion_database.tasks.id
  name     = "Contact"

  depends_on = [notion_database_property_url.link]
}

resource "notion_database_property_people" "assignees" {
  database = notion_database.tasks.id
  name     = "Assignees"

  depends_on = [notion_database_property_email.contact]
}

resource "notion_database_property_created_time" "created" {
  database = notion_database.tasks.id
  name     = "Created"

  depends_on = [notion_database_property_people.assignees]
}

resource "notion_database_property_created_by" "created_by" {
  database = notion_database.tasks.id
  name     = "Created by"

  depends_on = [notion_database_property_created_time.created]
}

resource "notion_database_property_last_edited_time" "edited" {
  database = notion_database.tasks.id
  name     = "Last edited"

  depends_on = [notion_database_property_created_by.created_by]
}

resource "notion_database_property_last_edited_by" "edited_by" {
  database = notion_database.tasks.id
  name     = "Last edited by"

  depends_on = [notion_database_property_last_edited_time.edited]
}

resource "notion_database_property_relation" "project" {
  database         = notion_database.tasks.id
  name             = "Project"
  related_database = notion_database.projects.id

  depends_on = [notion_database_property_last_edited_by.edited_by]
}

# Sums Points across the related projects. notion_database_entry can't set
# relation values, so the rollup column stays empty on these rows.
resource "notion_database_property_rollup" "project_points" {
  database          = notion_database.tasks.id
  name              = "Project points"
  function          = "sum"
  relation_property = notion_database_property_relation.project.name
  rollup_property   = notion_database_property_number.project_points.name
}

# Tasks rows. They wait for the whole schema so every column exists.

resource "notion_database_entry" "task_config" {
  database = notion_database.tasks.id
  title    = "Write the showcase config"

  select_properties   = { "Priority" = "High" }
  number_properties   = { "Estimate" = 5 }
  status_properties   = { "Status" = "Done" }
  checkbox_properties = { "Reviewed" = true }
  date_properties     = { "Due" = "2026-11-03" }
  rich_text_properties = {
    "Notes" = "Lives in [examples/showcase](${local.repo_url}/tree/main/examples/showcase)."
  }
  url_properties   = { "Link" = local.repo_url }
  email_properties = { "Contact" = "showcase@example.com" }

  depends_on = [notion_database_property_rollup.project_points]
}

resource "notion_database_entry" "task_workflow" {
  database = notion_database.tasks.id
  title    = "Add the showcase workflow"

  select_properties    = { "Priority" = "Medium" }
  number_properties    = { "Estimate" = 3 }
  status_properties    = { "Status" = "In progress" }
  checkbox_properties  = { "Reviewed" = false }
  date_properties      = { "Due" = "2026-11-10" }
  rich_text_properties = { "Notes" = "Runs terraform apply, then a plan that must be empty." }

  depends_on = [notion_database_property_rollup.project_points]
}

resource "notion_database_entry" "task_cleanup" {
  database = notion_database.tasks.id
  title    = "Trash the previous showcase"

  select_properties   = { "Priority" = "Low" }
  number_properties   = { "Estimate" = 1 }
  status_properties   = { "Status" = "Not started" }
  checkbox_properties = { "Reviewed" = false }
  date_properties     = { "Due" = "2026-11-17" }

  depends_on = [notion_database_property_rollup.project_points]
}

resource "notion_database_entry" "task_docs" {
  database = notion_database.tasks.id
  title    = "Document the showcase"

  select_properties = { "Priority" = "Medium" }
  number_properties = { "Estimate" = 2 }
  status_properties = { "Status" = "Blocked" }
  date_properties   = { "Due" = "2026-11-24" }

  # The entry's page body, written as markdown.
  markdown = <<-EOT
    ## Why this is blocked

    Waiting on the first CI run of the showcase.

    - [ ] Run the workflow
    - [ ] Check the page in Notion
  EOT

  depends_on = [notion_database_property_rollup.project_points]
}

# A table view on the Tasks data source. The data source ID comes from the
# templates data source, which looks it up from the database ID.
resource "notion_view" "tasks_table" {
  database_id    = notion_database.tasks.id
  data_source_id = data.notion_database_templates.tasks.data_source_id
  name           = "All tasks (Terraform view)"
  type           = "table"
}
