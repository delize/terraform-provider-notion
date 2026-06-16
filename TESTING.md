# Testing

This provider's tests are real acceptance tests — they make API calls against
a Notion workspace. Unit tests are intentionally light because every code path
of value goes through the network.

## Running

```sh
make testacc
```

That's the canonical entry point — it sets `TF_ACC=1` and runs everything
under `internal/provider/`. Without `TF_ACC=1` the framework no-ops the
acceptance steps.

Tests skip themselves when their required env vars aren't set, so you can
opt in to whichever subset you want by exporting only those.

## Required environment

Always required for any acceptance test:

| Variable | Purpose |
|---|---|
| `NOTION_TOKEN` | Integration token. Must have access to whatever pages/databases the tests reference. |

Required for tests that mutate pages/databases:

| Variable | Purpose |
|---|---|
| `NOTION_TEST_PARENT_PAGE_ID` | A Notion page ID that the integration can write to. Acts as the parent for ephemeral test pages and databases. |

Optional, gates specific tests:

| Variable | Test | Purpose |
|---|---|---|
| `NOTION_TEST_VIEW_DATABASE_ID` | `TestAccViewResource` | Database ID whose data source the test view will be attached to. The test discovers `data_source_id` via direct API. |
| `NOTION_TEST_VIEW_ID` | `TestAccViewQueryDataSource` | An existing view ID to query. Typically copy this from a prior `TestAccViewResource` run. |
| `NOTION_TEST_TEMPLATE_ID` | `TestAccPageWithTemplate` | A Notion template page ID the integration can access. Without this the test skips. |
| `GITHUB_PR_NUMBER` | (all CI) | Suffix added to ephemeral page titles so debris is identifiable. CI sets this automatically. |

## What's tested

| Feature | Test | Notes |
|---|---|---|
| `notion_meeting_notes` data source | `TestAccMeetingNotesDataSource[_WithLimit]` | Tolerates an empty workspace — only asserts `raw_json` set. Probes the endpoint first and skips if the workspace plan doesn't include AI meeting notes (Notion returns 400 `validation_error` in that case — it's a plan gate, not a provider bug). |
| `notion_page` `markdown_insert` | `TestAccPageMarkdownInsert` | State-only check; can't compare page body byte-for-byte because Notion normalizes markdown. |
| `notion_page` move | `TestAccPageMove` | Two steps flip `parent_page_id`; asserts the resource address stays the same (no recreate). |
| `notion_page` template | `TestAccPageWithTemplate` | Smoke test — Notion applies the template asynchronously so we only assert state. |
| `notion_database_property_status` | `TestAccDatabasePropertyStatusResource` | Create + option update. Groups (To-do / In progress / Complete) are server-assigned and not asserted. |
| `notion_view` | `TestAccViewResource` | Create + rename + import-verify. Skips when the workspace's API doesn't return a `data_sources` array on database GET (pre-v2025-09-03). |
| `notion_view_query` | `TestAccViewQueryDataSource` | Requires `NOTION_TEST_VIEW_ID`. |

## What's not tested, and why

| Path | Why no test |
|---|---|
| `request_status.type="incomplete"` warning on `notion_database_entries` (2026-04-20) | Triggering the 10K pagination cap requires populating 10K+ entries in a test database, which is impractical for an acceptance test. The diagnostic-emit path is small and visually verified. |
| `agent_id` parent type warning on `notion_page.Read` (2026-05-11) | Pages parented by an agent can only come into existence through Notion's AI/agent feature; we can't synthesize that state from this provider. The fallthrough is a single `AddWarning` call. |
| `heading_4` / `tabs` / `tab` block builder error path | Intentional error case for SDK-gap block types — exercising it would just assert the error message, which is fragile and low-value. |
| Bot IDs in people properties / user mentions (2026-06-10) | The provider doesn't currently write people-property values or rich-text mentions; this changelog item is a docs note. |
| OAuth fresh token pair (2026-06-08) | OAuth flow happens entirely outside this provider; nothing to test. |
| AI meeting notes data source on plan-gated workspaces | The endpoint returns 400 `validation_error` when the workspace's plan doesn't include AI meeting notes. The tests probe and skip in that case rather than failing — a real user with a non-AI workspace using the data source still surfaces the error correctly. |

## Risk profile by test gate

If you can only run a subset, here's what each unlocks:

- `NOTION_TOKEN` alone → `notion_meeting_notes` + the other no-mutation data source tests
- `+ NOTION_TEST_PARENT_PAGE_ID` → all page/database/property/markdown_insert/move tests
- `+ NOTION_TEST_VIEW_DATABASE_ID` → view resource (requires v2025-09-03 API)
- `+ NOTION_TEST_VIEW_ID` → view query data source
- `+ NOTION_TEST_TEMPLATE_ID` → templated page creation

## Cleanup

Tests use `tf-acc-test-*` prefixes for ephemeral pages so they're easy to spot
in Notion if cleanup fails. Terraform's test framework calls Destroy at the
end of each `TestCase`, which trashes pages via the `in_trash` field — they
end up in the workspace trash, not hard-deleted.
