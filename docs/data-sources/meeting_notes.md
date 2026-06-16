---
page_title: "notion_meeting_notes Data Source - Notion"
subcategory: ""
description: |-
  Query AI meeting notes for the integration's user.
---

# notion_meeting_notes (Data Source)

Query AI meeting notes for the integration's user via the 2026-05-11
`POST /v1/blocks/meeting_notes/query` endpoint. Returns the common identifying
fields per result plus the raw JSON response for callers that need fields not
surfaced directly (e.g. `attendees`, transcripts).

The `attendees` filter alias is normalized server-side, so filters round-trip
cleanly.

## Example Usage

```terraform
# All meeting notes available to the integration
data "notion_meeting_notes" "all" {}

# Filter by attendee, sort, and cap to 25
data "notion_meeting_notes" "with_andrew" {
  filter = jsonencode({
    attendees = ["a1b2c3d4-e5f6-7890-abcd-ef1234567890"]
  })
  sort  = jsonencode([{ direction = "descending", timestamp = "created_time" }])
  limit = 25
}

# Access raw fields not surfaced individually
locals {
  notes = jsondecode(data.notion_meeting_notes.with_andrew.raw_json)
}
```

## Schema

### Optional

- `filter` (String) Optional filter as a JSON object string. The `attendees`
  alias is normalized server-side so filters round-trip cleanly.
- `sort` (String) Optional sort as a JSON object/array string.
- `limit` (Number) Optional maximum number of results to return.

### Read-Only

- `results` (List of Object) Meeting notes returned by the query. Each object has:
  - `id` (String)
  - `object` (String)
  - `created_time` (String)
  - `last_edited_time` (String)
  - `url` (String) — present when the response includes it
  - `title` (String) — plain-text concatenation of the title rich-text segments
- `raw_json` (String) The full raw JSON response from the endpoint. Use
  `jsondecode` to access fields not surfaced individually.
