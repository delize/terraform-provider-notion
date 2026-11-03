terraform {
  required_version = ">= 1.6"

  required_providers {
    notion = {
      source = "delize/notion"
    }
  }
}

# The token comes from NOTION_TOKEN. silent_notifications keeps applies from
# notifying anyone mentioned on the pages this config builds.
provider "notion" {
  silent_notifications = true
}
