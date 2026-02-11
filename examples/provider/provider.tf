terraform {
  required_providers {
    notion = {
      source = "delize/notion"
    }
  }
}

# Configure the provider. The token can also be set via the NOTION_TOKEN env var.
provider "notion" {
  # token = "secret_..."
}

# Create a page
resource "notion_page" "example" {
  parent_page_id = "YOUR_PARENT_PAGE_ID"
  title          = "My Terraform Page"
}

# Create a database
resource "notion_database" "tasks" {
  parent             = notion_page.example.id
  title              = "Tasks"
  title_column_title = "Task Name"
}

# Add properties to the database
resource "notion_database_property_select" "status" {
  database = notion_database.tasks.id
  name     = "Status"
  options = {
    "To Do"       = "red"
    "In Progress" = "yellow"
    "Done"        = "green"
  }
}

resource "notion_database_property_number" "priority" {
  database = notion_database.tasks.id
  name     = "Priority"
  format   = "number"
}

resource "notion_database_property_date" "due_date" {
  database = notion_database.tasks.id
  name     = "Due Date"
}

resource "notion_database_property_checkbox" "completed" {
  database = notion_database.tasks.id
  name     = "Completed"
}

# Add an entry to the database
resource "notion_database_entry" "first_task" {
  database = notion_database.tasks.id
  title    = "Set up Terraform"
}

# Look up a user
data "notion_user" "admin" {
  email = "admin@example.com"
}

# Search for existing resources
data "notion_database" "existing" {
  query = "My Existing Database"
}

data "notion_page" "existing" {
  query = "My Existing Page"
}

# List entries in a database
data "notion_database_entries" "all_tasks" {
  database = notion_database.tasks.id
}
