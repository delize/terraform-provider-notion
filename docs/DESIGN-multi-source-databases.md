# Design: Multi-Source Databases Support (API v2025-09-03)

> Status: **proposal**, not implemented. Filed as a follow-up to the
> `feat/changelog-catchup-2026-q1` branch.

## Context

The 2025-08-26 Notion API change (version `2025-09-03`) introduced
**multi-source databases**. A Notion database is now a container that can hold
one or more **data sources**. Property schemas, queries, page rows, and many
parent references that previously hung off the database now hang off a
data source.

This provider was built before that change and conflates "database" and
"data source" everywhere:

- `notion_database` manages a database object. Its schema (properties) is
  modelled here, but under v2025-09-03 properties live on a data source.
- `notion_database_entry` writes to `parent.database_id` instead of
  `parent.data_source_id`.
- Every `notion_database_property_*` resource targets `<database>/<name>`,
  but should target `<data_source>/<name>` under the new model.
- `notion_database_entries` data source posts to `POST /v1/databases/:id/query`
  instead of `POST /v1/data_sources/:id/query`.
- `notion_database_property_relation` writes `database_id` in relation
  config, but new relations must use `data_source_id`.

The 2026-03-19 `notion_view` resource added in this branch already
takes `data_source_id` (correctly), so it's the only resource shipped
today that works cleanly under v2025-09-03.

## Constraints

- Existing users have state pointing at `database_id` everywhere. A breaking
  rename without migration would brick every config.
- The Notion API still supports v2022-06-28 (what the upstream SDK is pinned
  to) where single-data-source databases are addressable by `database_id`. So
  there's a backwards path.
- The upstream `jomei/notionapi` SDK doesn't model data sources yet. Any
  data-source-aware code will need direct HTTP shims (same pattern as
  `notion_views.go`, `notion_page_extras.go`).

## Options

### Option A — Additive: introduce `notion_data_source` alongside `notion_database`

Add a new `notion_data_source` resource that owns the schema (properties),
queries, and entry parent references for one data source. Leave
`notion_database` as-is for the database container (title, icon, cover).

New resources:

- `notion_data_source` — title, parent database, properties move here over time
- New `notion_database_entry` variant or attribute `data_source_id` that
  takes precedence over `database_id`
- `notion_data_source_query` data source (parallel to `notion_database_entries`)

Pros:

- Non-breaking for existing users
- Users can opt in to the new model when they need multi-source
- Clean alignment with the new API for new code

Cons:

- Two ways to do the same thing for the next several quarters
- Property resources still target `database_id` for compat — eventually
  need to dual-target

### Option B — Breaking refactor + state migration

Rename `database_id` to `data_source_id` everywhere, write a state migration
that resolves the database's single data source on first refresh, and
deprecate the old field. Major version bump.

Pros:

- One clean model
- Aligns with where Notion is heading

Cons:

- Breaking change for everyone
- State migration is risky for multi-source databases (which `data_source_id`
  do we pick?)
- Forces a v2 release

### Option C — Add `data_source_id` as optional alongside `database_id`

On every resource that currently takes `database_id`, add an optional
`data_source_id`. When set, it wins; when null, the provider resolves the
database's default (first) data source and uses that.

Pros:

- Non-breaking
- Users can adopt incrementally per-resource
- Multi-source DBs become addressable

Cons:

- "Magic resolution" of default data source can surprise users
- Plan/apply diff churn if Notion ever reorders data sources

## Recommendation

**Option A (additive)**, scoped as follows:

1. **Phase 1 — this PR family**: Land Views API + status property + page
   move/template (done on `feat/changelog-catchup-2026-q1` and the
   `feat/changelog-catchup-2026-q2` predecessor). No multi-source work.

2. **Phase 2 — new resource**: Add `notion_data_source` resource and
   `notion_data_source_query` data source. Don't touch existing
   `notion_database*` resources. New users on multi-source DBs use the
   new resources; existing users keep working.

3. **Phase 3 — opt-in field on property resources**: Add optional
   `data_source_id` to every `notion_database_property_*` resource. When
   set, it routes the PATCH to `/v1/data_sources/:id` instead of
   `/v1/databases/:id`. `database_id` remains required for backcompat.

4. **Phase 4 — deprecation window** (months later): mark `database_id` on
   property resources as deprecated with a warning pointing at
   `data_source_id`. Bump major version when ready.

5. **Phase 5 — major version**: Remove `database_id` from property
   resources. Migrate state via terraform-plugin-framework's state-upgrade
   API.

Phases 2 and 3 can ship in parallel; 4 and 5 wait for ecosystem signal.

## Out of scope for this design

- Webhook handling (this provider doesn't subscribe to webhooks).
- Search filter object renames (`object=data_source` vs `object=database`) —
  trivial swap once the search data source becomes data-source-aware.
- TypeScript SDK migration notes — irrelevant; this provider uses the Go SDK.

## Open questions

1. Does the `jomei/notionapi` SDK have a roadmap for v2025-09-03 support? If
   yes, Phase 2's direct-HTTP shims can be dropped once the SDK ships it.
2. How does the markdown_client behave when the parent is a multi-source
   database? Page-create with `parent.database_id` may need to switch to
   `parent.data_source_id`. Test before Phase 2 ships.
3. Should `notion_view`'s `data_source_id` field gain an `auto` resolution
   path for the single-source-database case? Currently the user must look it
   up. Keeping it explicit is probably right.

## Estimated effort

- Phase 2: ~600-800 LOC, 2-3 days
- Phase 3: ~150 LOC per property resource × 13 resources, ~3-4 days
- Phase 4: docs only, ~1 day
- Phase 5: ~400 LOC for state-upgrade + removals, ~2 days

Total: ~2 weeks of focused work spread over a 6-12 month deprecation window.
