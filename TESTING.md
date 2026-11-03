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
| `NOTION_TEST_AGENT_ID` | `TestAccAgentSettingsResource`, `TestAccAgentDataSources` | ID of a throwaway Custom Agent the token has full access to. The resource test disables and re-enables it, changes its credit limit, and renames it and edits its description before restoring both. Use a personal access token, since a connection can't change a credit limit. |
| `NOTION_ADMIN_TOKEN` | Admin API acceptance tests | Organization bot token (Enterprise) with `permission-group:read`, `permission-group:write`, `workspace:read`, `user:read`, and for the read-only test `legal-hold:read`, `workflows:read`, `mcp-client-connection:read`, `personal-access-token:read`. |
| `NOTION_TEST_ADMIN_WORKSPACE_ID` | Admin API acceptance tests | Workspace ID where test groups are created and deleted and listings are read. |
| `NOTION_TEST_ADMIN_USER_ID` | `TestAccPermissionGroupMemberResource` | A full workspace member (not a guest) the test adds to and removes from a test group. |
| `GITHUB_PR_NUMBER` | (all CI) | Suffix added to ephemeral page titles so debris is identifiable. CI sets this automatically. |

## What's tested

| Feature | Test | Notes |
|---|---|---|
| `notion_meeting_notes` data source | `TestAccMeetingNotesDataSource[_WithLimit]` | Tolerates an empty workspace — only asserts `raw_json` set. Probes the endpoint first and skips if the workspace plan doesn't include AI meeting notes (Notion returns 400 `validation_error` in that case — it's a plan gate, not a provider bug). |
| `notion_page` `markdown_insert` | `TestAccPageMarkdownInsert` | State-only check; can't compare page body byte-for-byte because Notion normalizes markdown. |
| `notion_page` move | `TestAccPageMove` | Two steps flip `parent_page_id`; asserts the resource address stays the same (no recreate). |
| `notion_page` template | `TestAccPageWithTemplate` | Smoke test — Notion applies the template asynchronously so we only assert state. |
| `notion_database_property_status` | `TestAccDatabasePropertyStatusResource` | Create, option update, then `option_groups` (2026-06-22) with an in-place color change (2026-10-09). `default_option` (2026-10-22) is logged rather than asserted, because Notion doesn't promise it on the 2022-06-28 response this resource reads. |
| `option_groups` validation | `TestUnitStatusOptionGroups_*` | Plan-only through real Terraform, no workspace needed. Rejects unknown group names and grouped options missing from `options`. |
| `silent_notifications` | `TestAccSilentNotifications` | Page create, update, and destroy with the setting on (block appends are covered by unit tests), across the SDK (2022-06-28) and direct-HTTP (2026-03-11) paths. Proves Notion accepts `notifications.mode`. Whether a notification was sent can't be observed from the API. |
| `notion_block` heading_4 and tab | `TestAccBlockHeading4AndTabs` | Creates a heading_4 and a tab with content in its first tab, then relabels, clears an icon, adds a tab and recolors the heading, imports both (ignoring `after`), and shrinks the tab to one item. Content stays parented on `tab_items[0].id` throughout. |
| `notion_block` default color | `TestAccBlockDefaultColor`, `TestReadBlockIntoState_NormalizesDefaultColor`, `TestBuildBlockUpdateRequest_EmptyColorSendsDefault`, `TestUnitBlockColor_DefaultBackgroundAccepted` | A heading_1 with no color must plan empty after create even though Notion reports `default`, an explicit `default` paragraph round-trips, a red heading goes back to `""` when the color is removed, and the uncolored heading imports cleanly. Unit tests cover every colored SDK type on read, the `default` sent when an update clears a color, and the `default_background` value. |
| tab and heading_4 config rules | `TestUnitBlockTab*`, `TestUnitBlockTabsTypeRemoved`, `TestUnitBlockTabAndHeading4_ValidPlan` | Plan-only through real Terraform. Requires a non-empty `tab_items` on tab, rejects it elsewhere, rejects text and color on a tab, rejects the old `tabs` type. |
| Tab and heading_4 request building and parsing | `TestHeading4Body`, `TestTabBlockShape`, `TestTabItemBodyClearIcon`, `TestAppendBlockRequestPosition`, `TestPlanTabItemOps`, `TestTabItemsFromChildren`, `TestReadRawBlockIntoState_Heading4`, `TestReconcileTabItems`, `TestListBlockChildrenRawFollowsCursor` | Fixtures plus a fake Notion server wired in through the provider's per-token HTTP client registry. |
| `notion_block` callout icons | `TestAccBlockCalloutIcons` | Creates a callout with a native icon, switches it to an emoji with a background color, checks that removing the icon is refused (the API can't), import-verifies, then clears the color while keeping the emoji. |
| `notion_block` callout with no icon | `TestAccBlockCalloutNoIcon` | Creates a callout with no icon attribute and relies on the post-apply plan check to prove it plans nothing, whatever default icon Notion adds. Logs the icon Notion reports, so the run shows whether the API sets a default. |
| Callout icons against a fake | `TestUnitBlockCallout_IconLifecycle`, `TestUnitBlockCallout_URLAndCustomEmoji`, `TestUnitBlockCallout_ImportExistingEmoji`, `TestUnitBlockCallout_IconTypeChangedIsDrift`, `TestUnitBlockCallout_UnconfiguredIcon`, `TestUnitBlockCallout_ServerDefaultIcon`, `TestReadRawBlockIntoState_Callout`, `TestCalloutBody`, `TestCalloutIconRemoval` | Real Terraform against a local fake. Checks the request bodies for every icon kind, that removing a configured icon fails before any request, and that `icon: null` is never sent. An emoji callout imported with the config earlier versions used plans no changes. An icon kind changed in Notion is put back on the next apply, and a stale emoji in state is cleared on read. A callout with no icon configured ignores an emoji or native icon added in the app, and a server default 💡 on create plans nothing. An import takes Notion's emoji. |
| Callout icon config rules | `TestUnitBlockCalloutIcon_*` | Plan-only through real Terraform. The non-emoji icon attributes are rejected on other block types, and on callouts they follow the `notion_page` rules for conflicts, `icon_color`, and Notion icon set URLs. |
| `notion_page` icons, `is_locked`, `position` | `TestAccPageIconsLockPosition` | Native icon with color and lock plus `position = "page_start"`, then an external icon URL unlocked, then no icon (must clear it), then import (ignoring `position`). |
| `notion_database` icons, `is_locked` | `TestAccDatabaseIconsLock` | Native icon locked, then an emoji `icon` unlocked. |
| `notion_database` `show_data_source_title` | `TestAccDatabaseShowDataSourceTitle`, `TestUnitDatabaseShowDataSourceTitle_ValidPlan`, `TestDecodeContainerExtras_ShowDataSourceTitle`, `TestPatchContainerExtras_ShowDataSourceTitle`, `TestFinishExtras_ShowDataSourceTitle`, `TestDatabaseExtraFields` | Full-page database with one data source, where Notion never shows the title. Created unset (expects Notion's default `true`), then `false`, then `true`, then unset again (left alone), then import. Each step must leave an empty plan, which would catch Notion echoing the effective value instead of the stored one. Unit tests check that the flag rides on the existing extras PATCH, is sent only when configured, and never goes to pages. |
| `notion_database` `database_type`, `data_source_ids` | `TestAccDatabaseResource`, `TestAccDatabaseDataSourceIDsFeedView`, `TestUnitDatabaseDataSourceIDs_FeedView`, `TestDecodeContainerExtras_DatabaseTypeAndDataSources`, `TestDatabaseSetReadOnly` | A new database reads a null `database_type` and one hyphenless data source ID, and import-verifies. A `notion_view` created from `data_source_ids[0]` in the same config must land on that data source. Unit tests cover a typed database, ID normalization, an empty list, and a response with no `data_sources`. |
| Icon and position validation | `TestUnitPageIcon_*`, `TestUnitPagePosition_*`, `TestUnitDatabaseIcon_Conflicts`, `TestUnitPageNewAttributes_ValidPlans` | Plan-only through real Terraform. |
| Partial create keeps the object in state | `TestUnitPage_CreateExtrasFailureKeepsState`, `TestUnitPage_PlainCreateSkipsExtrasRequest`, `TestUnitBlockTab_CreateItemReadFailureKeepsState` | Real Terraform against a local fake. A page whose icon PATCH fails, or a tab whose items can't be read back, must be tainted and replaced on the next apply, not orphaned. A page with no native icon or lock makes no second request. |
| Icon parsing and reconcile rules, page decode | `TestIconFieldsRequest`, `TestParseIcon`, `TestReconcile_*`, `TestIsNotionIconSetURL`, `TestPagePositionRequest`, `TestDecodePageObject` | Unit tests on fixtures. |
| `notion_search` `in_trash` | `TestAccSearchDataSource_InTrash`, `TestBuildSearchBody`, `TestDoSearch_*` | Acceptance proves Notion accepts `filter.in_trash` on 2022-06-28. Trash contents aren't predictable, so only a successful read is asserted. |
| `email_verified` on user data sources | `TestAccUsersDataSource_EmailVerified`, `TestListUsersRaw_*` | Value is logged, not asserted, since it depends on the token's user capabilities. Unit tests cover pagination and null for bots. |
| `notion_page_markdown` `include_transcript` | `TestAccPageMarkdownDataSource_IncludeTranscript`, `TestPageMarkdownURL` | Page without meeting notes, so markdown must equal the plain read. |
| `markdown_version` and title-only updates | `TestAccPageMarkdownV2` | Creates a page with `markdown_version = "v2"`, appends a paragraph through the API as a stand-in for a Notion UI edit, changes only the title and checks the paragraph survived, then changes the markdown and checks the body was replaced. Reads back through `notion_page_markdown` with `v2`. |
| `markdown_version` on every markdown call | `TestUnitMarkdownVersion_*`, `TestPageMarkdownURL` | Real Terraform against a local fake. Page create, replace, insert, and the data source read all carry the resolved version (resource over provider, omitted when neither is set). A provider default isn't stored or diffed on pages without markdown. Entry creates are checked at the client level. |
| Markdown resent only on change | `TestMarkdownNeedsReplace`, `TestUnitPage_MarkdownResentOnlyOnChange`, `TestUnitDatabaseEntry_MarkdownResentOnlyOnChange` | A title-only update makes no `replace_content` call. A markdown change or a version change makes one. Unset and explicit `v1` count as the same. The entry test calls `Update` directly because the SDK it uses has no base URL hook. |
| `markdown_insert` inserts only on change | `TestUnitPage_MarkdownInsertOnlyOnChange` | Real Terraform against a local fake. Create inserts once. A title-only update, a `markdown` change, and removing the block insert nothing. A new `content`, a new `position`, or adding the block back inserts exactly once. |
| `notion_database_entry` create with markdown and properties | `TestAccDatabaseEntryResourceWithMarkdown`, `TestUnitDatabaseEntry_MarkdownCreateSendsProperties` | Acceptance sets a select and a number property next to `markdown` and checks both after a refresh from Notion. The unit test checks they are in the same POST /v1/pages body as the markdown. |
| Markdown `warnings` | `TestUnitMarkdownWarnings_ParsedFromWrites`, `TestUnitPage_InsertWarningsBecomeDiagnostics`, `TestUnitPage_WarningsDoNotFailApply`, `TestUnitDatabaseEntry_ReplaceWarningsBecomeDiagnostics` | Each warning in a replace or insert response becomes one warning diagnostic with its code and message, and the apply still succeeds. |
| `notion_database_entries` `filter` and `sorts` | `TestAccDatabaseEntriesDataSource_FilterAndSorts`, `TestParseEntriesQuery`, `TestQueryDatabaseRawAt_PassesFilterAndSorts` | Multi-value select filter (2026-04-17) plus a title sort, exact count and order asserted. |
| `notion_custom_emojis` | `TestAccCustomEmojisDataSource`, `TestListCustomEmojis_PaginatesWithNameFilter` | Most workspaces have no custom emojis, so acceptance asserts a successful read. |
| `notion_database_templates` | `TestAccDatabaseTemplatesDataSource`, `TestDiscoverDataSourceID`, `TestListDataSourceTemplates_Paginates`, `TestUnitDatabaseTemplates_RequiresExactlyOneID` | Acceptance checks data source discovery on a fresh database. Template count isn't asserted. |
| `notion_agent_settings` | `TestAccAgentSettingsResource` | Requires `NOTION_TEST_AGENT_ID`. Disables the agent with a 1000 credit limit, a test name, and a test description, then clears the description while it stays disabled (status must not be resent), then re-enables it, clears the limit, and restores the original name and description, then import-verifies. Changes a real agent, so point it at a throwaway one. Destroy leaves it enabled and uncapped. |
| `notion_agent`, `notion_agents`, `notion_agent_insights` (beta) | `TestAccAgentDataSources` | Requires `NOTION_TEST_AGENT_ID`. Read-only. |
| Agent API shim, status and drift rules, config validation | `TestGetAgent*`, `TestUpdateAgent*`, `TestQueryAgents_*`, `TestGetAgentInsights_*`, `TestStatusChangeNeeded`, `TestEnabledFromAgent`, `TestAgentUpdateFields`, `TestBuildAgentsQuery`, `TestUnitAgent*` | Unit tests against `httptest`, plus plan-only checks through real Terraform. `TestUnitAgentSettings_Validation` covers `model_mode` and `model_id` combinations and an empty `name`. |
| `notion_agent_settings` name, description, and model | `TestUnitAgentSettings_Lifecycle`, `TestAgentUpdateFields` | Real Terraform against a stateful fake of `GET` and `PATCH /v1/agents/{id}` that answers 403 when `disabled` is sent to a disabled agent, as Notion does. Asserts the exact PATCH body per apply: everything in one request on create, only the changed field afterwards, no status resend, auto planning `model_id` as null, an early-access model read back with a null ID keeping the configured ID with an empty follow-up plan, unmanaged fields reading back without being sent, name drift corrected alone, and import. |
| Retry transport, silent body injection, status schema parsing | `TestRetryTransport_*`, `TestNotificationsTransport_*`, `TestParseStatusProperty_*` | Unit tests against `httptest` servers and fixtures. `TestRetryTransport_NoReplay*` covers creates that must not be resent after reaching the API. |
| Admin API permission groups, users, workspaces | `TestAdminClient_*`, `TestUnitPermissionGroup_Lifecycle`, `TestUnitPermissionGroupMember_DriftRecreates`, `TestUnitAdminDataSources`, `TestUnitAdmin_*`, `TestUnitProvider_NoTokens` | Real Terraform against an in-memory fake Admin API that follows the documented paths, list envelopes, and error bodies, with forced pagination. Covers create, in-place updates, import, drift, destroy, admin-only provider blocks, and missing-token errors. |
| Admin API legal holds | `TestUnitLegalHold_*` | Lifecycle with an in-place rename and user swap, destroy keeps the hold by default, `release_on_destroy`, both data sources, and the export action fired from a `terraform_data` `action_trigger`. A failed read-back right after create still saves the hold, and a create that gets a JSON 500 is sent only once. |
| Admin API agents | `TestUnitAdminAgentSettings_Lifecycle`, `TestUnitAdminWorkspaceAgentSettings*`, `TestUnitAdminAgentDataSourcesAndDeleteAction`, `TestUnitAdminAgent_Validation` | Authoritative sharing including drift removal, credit limit cleared with null, destroy leaves the agent and settings untouched, multi-page credit periods, and the delete action. A failed credit limit call doesn't make the next apply resend `disable_existing_agents`. |
| Admin API MCP connections | `TestUnitMCPEnterpriseAccess_*`, `TestUnitMCPConnectionsAndRevokeAction` | Destroy keeps a denial by default, `restore_on_destroy`, bracket-encoded filters, and the revoke action. |
| Admin API PATs, exports, sessions | `TestUnitPATsDataSource*`, `TestUnitWorkspaceExport*`, `TestUnitRevokeUserSessionsAction*` | Filters and nullable fields, the revoke and enqueue actions, and request bodies that omit unset options. |
| Admin API against a real organization | `TestAccPermissionGroupResource`, `TestAccPermissionGroupMemberResource`, `TestAccAdminDataSources`, `TestAccAdminReadOnlyDataSources` | Skip without `NOTION_ADMIN_TOKEN` and `NOTION_TEST_ADMIN_WORKSPACE_ID`. The read-only test lists legal holds, agents, MCP connections, and PATs. |
| `notion_view` | `TestAccViewResource` | Create + rename + import-verify. Skips when the workspace's API doesn't return a `data_sources` array on database GET (pre-v2025-09-03). |
| `notion_view_query` | `TestAccViewQueryDataSource` | Requires `NOTION_TEST_VIEW_ID`. |

## What's not tested, and why

| Path | Why no test |
|---|---|
| `request_status.type="incomplete"` warning on `notion_database_entries` (2026-04-20) | Triggering the 10K pagination cap requires populating 10K+ entries in a test database, which is impractical for an acceptance test. The diagnostic-emit path is small and visually verified. |
| `agent_id` parent type warning on `notion_page.Read` (2026-05-11) | Pages parented by an agent can only come into existence through Notion's AI/agent feature; we can't synthesize that state from this provider. The fallthrough is a single `AddWarning` call. |
| Admin API legal holds, agent settings, MCP access, and every action against a real organization | Running them for real would create compliance records, change agent governance, or cut off access in the test organization. The fake-server tests cover them. |
| Bot IDs in people properties / user mentions (2026-06-10) | The provider doesn't currently write people-property values or rich-text mentions; this changelog item is a docs note. |
| Markdown `warnings` from a real workspace | Notion returns them only for input its parser changes, and which inputs do that isn't documented as stable. The fake covers the parsing. Warnings on page creates aren't documented at all, so they are parsed only if Notion sends them. |
| `markdown_version` on comments | `notion_comment` ships in a separate change and doesn't send the field yet. |
| OAuth fresh token pair (2026-06-08) | OAuth flow happens entirely outside this provider; nothing to test. |
| `notion_agent_settings` model against a real agent | Changing the model republishes the agent, and which model IDs a workspace can pin varies, so the acceptance test leaves the model alone. The fake-server lifecycle covers the request bodies and the early-access read-back. |
| AI meeting notes data source on plan-gated workspaces | The endpoint returns 400 `validation_error` when the workspace's plan doesn't include AI meeting notes. The tests probe and skip in that case rather than failing — a real user with a non-AI workspace using the data source still surfaces the error correctly. |

## Risk profile by test gate

If you can only run a subset, here's what each unlocks:

- `NOTION_TOKEN` alone → `notion_meeting_notes` + the other no-mutation data source tests
- `+ NOTION_TEST_PARENT_PAGE_ID` → all page/database/property/markdown_insert/move tests
- `+ NOTION_TEST_VIEW_DATABASE_ID` → view resource (requires v2025-09-03 API)
- `+ NOTION_TEST_VIEW_ID` → view query data source
- `+ NOTION_TEST_TEMPLATE_ID` → templated page creation
- `+ NOTION_TEST_AGENT_ID` → agent settings and agent data sources (changes a real agent)
- `NOTION_ADMIN_TOKEN` + `NOTION_TEST_ADMIN_WORKSPACE_ID` → Admin API permission groups and read-only admin listings (Enterprise only)

## Cleanup

Tests use `tf-acc-test-*` prefixes for ephemeral pages so they're easy to spot
in Notion if cleanup fails. Terraform's test framework calls Destroy at the
end of each `TestCase`, which trashes pages via the `in_trash` field — they
end up in the workspace trash, not hard-deleted.

## Showcase

The acceptance tests destroy everything they build, so the test pages in Notion are empty. `examples/showcase` is a Terraform config that builds one page using every non-Enterprise feature and leaves it in place. The `Showcase` workflow (`.github/workflows/showcase.yml`) applies it with the real `terraform` CLI against a provider built from the branch through `dev_overrides`, then runs `terraform plan -detailed-exitcode` and fails if the plan isn't empty. It runs on manual dispatch, release tags, and same-repo PRs that touch the showcase, and it shares the `notion-acceptance-workspace` concurrency group.

Each run first calls `scripts/showcase_cleanup.py`, which trashes the parent page's child pages whose titles start with `Provider showcase` and nothing else. No state is kept between runs, so the newest showcase replaces the previous one. It uses `NOTION_TOKEN`, `NOTION_TEST_PARENT_PAGE_ID`, and optionally `NOTION_TEST_TEMPLATE_ID`. See `examples/showcase/README.md` for what it covers, what it leaves out, and how to run it locally.

## CI and branch protection

`main` is protected by the ruleset in `.github/rulesets/main.json`. Changes land through a pull request and are squash-merged. Force-pushes and branch deletion are blocked. The required checks are `build` and `acceptance-result`. The repository admin role can bypass the rules only when merging a pull request, never for a direct push. Edit the JSON and run `python3 .github/rulesets/apply.py` to change it, since the script updates the ruleset by name.

`acceptance-result` exists so the required checks stay the same when the Terraform matrix changes. It passes only when `build` passed and every acceptance leg passed. On fork PRs it stays red on purpose, because acceptance for forks runs in the separate `Acceptance (fork)` workflow after a maintainer approves the `acceptance` environment. Once that run is green, merge the fork PR with the admin bypass.

Acceptance jobs from every run and branch share one concurrency group, `notion-acceptance-workspace`, with `queue: max`. Only one leg talks to the Notion test workspace at a time, because overlapping runs trip Notion's per-integration rate limit. Waiting legs queue in order instead of being cancelled.

The acceptance matrix covers the three newest Terraform minor versions. When a new minor ships, add it and drop the oldest in both `test.yml` and `acceptance-fork.yml`.

### Test workspace and secrets

CI runs acceptance against the "Terraform Notion - TestBed" workspace through the "Github - Terraform" internal integration. Everything the tests touch lives under the page "Terraform acceptance tests", which is connected to that integration. The repository secrets are:

| Secret | Points at | Needed by |
|---|---|---|
| `NOTION_TOKEN` | The integration's token | Every acceptance test |
| `NOTION_TEST_PARENT_PAGE_ID` | "Terraform acceptance tests" | Page, database, entry, property, markdown, move, and comment tests |
| `NOTION_TEST_TEMPLATE_ID` | "Terraform test template", a child page | `TestAccPageWithTemplate` |
| `NOTION_TEST_VIEW_DATABASE_ID` | "Terraform view test database", a child database | `TestAccViewResource` |
| `NOTION_TEST_VIEW_ID` | That database's default table view | `TestAccViewQueryDataSource` |
| `NOTION_TEST_AGENT_ID` | Not set. The workspace plan has no Custom Agents. | Agent tests |

A test whose secret is unset skips. Both workflow files pass every secret listed here, so a new one has to be added to `test.yml` and `acceptance-fork.yml`.
