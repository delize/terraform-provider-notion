variable "parent_page_id" {
  description = "ID of the page the showcase is built under. The integration needs edit access to it."
  type        = string
}

variable "template_id" {
  description = "Optional template page ID. When set, the showcase also creates a page from it. An empty string counts as unset, so an empty CI secret works."
  type        = string
  default     = null
}

variable "run_label" {
  description = "Suffix for the root page title, so each run's showcase is easy to tell apart. CI sets it to the run number and ref."
  type        = string
  default     = "local"
}

variable "lookup_page_query" {
  description = "Optional title to look up with the notion_page data source. It searches Notion, and a page created during this apply may not be indexed yet, so point it at a page that already exists. Leave unset to skip the lookup."
  type        = string
  default     = null
}
