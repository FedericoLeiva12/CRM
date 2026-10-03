# MCP tools and authorization

Agents connect to `/mcp` using Streamable HTTP and their own Bearer token. Browser `/api` routes use human sessions; an agent token cannot use those routes as a substitute for a missing MCP tool. The MCP adapter and browser handlers share repository validation and transactions.

## Discovery and grants

Tools are rebuilt from the live section registry on every request. A newly created section appears in the administrator's permission matrix immediately and starts with all record grants denied. Tools recheck authorization when invoked; discovery alone is never authorization. Revocations apply to subsequent requests, not cancellation of an already running request.

| Grant | Tools |
| --- | --- |
| Authenticated agent | `sections_schema`, `item_view_types`, `mentions_list`, `mentions_mark_read` |
| Section read | `<section>_list`, `_get`, `_activities`, `_comments`, `_mentionables` |
| Section write | `<section>_save`, `_update`, `_log_activity`, `_comment`, `_comment_update`, `_comment_delete` |
| Section delete | `<section>_delete`; delete requires write |
| Source read + write and target write | `<section>_convert` |
| `manage_schema` | `sections_create`, `fields_add`, `section_views_configure` |

Write requires Read. The settings editor enables Read when Write is selected; clearing Read clears Write and Delete. API requests with write but no read are rejected atomically. Migration 010 gives existing writers their required read grant and enforces the invariant in PostgreSQL, preserving read-only and denied grants. Runtime authorization also denies malformed legacy write grants.

Permanent record deletion is separate from writing. Migration 009 disables implicit deletion for existing agents; an administrator must explicitly enable Delete again. Deleting a comment is part of maintaining the calling agent's own contributions and does not grant deletion of records or other authors' comments.

`sections_schema` returns fields and configured item views for accessible sections. A schema manager can discover every section definition and its non-secret view settings without gaining access to record contents. `item_view_types` returns supported view IDs, labels, descriptions and configuration fields. Account, role, invitation, agent-token and webhook administration remain human administrator operations; no MCP tool can grant itself permissions.

## Searching and editing records

`<section>_list` accepts `search`, `filters`, `sort`, `limit`, and `cursor`. Free text behaves exactly like the browser search: every word must match some text/email value in the record, across different fields if needed. Search has a 200-character, eight-word limit. Filters are combined with AND. Parameterized values and field validation protect the shared query path.

```json
{
  "search": "mara northline",
  "filters": [{ "field": "status", "op": "eq", "value": "Active" }],
  "sort": { "field": "name", "direction": "asc" },
  "limit": 50
}
```

Filtered/search pages return `records`, `limit`, `total` and `next_cursor`. Pass the returned cursor to the next call with the same query. A call with no arguments preserves the legacy latest-500 response (`records`, `limit`).

`<section>_save` creates when `id` is absent and replaces complete field data when present. `_update` changes supplied fields only; `null` clears a field. The update response includes stored field values only when the caller also has read permission. As defense in depth, if read is revoked while a write is already running, the response contains only `id` and `updated_at`, including for an empty patch. This avoids disclosing unchanged values through a write operation.

## Comments and mentions

- `<section>_mentionables` returns eligible people/agents as IDs, names and handles, with no emails, credentials or grant settings. It requires section read.
- `<section>_comment` takes a **record** `id`, `body`, optional `parent_id`, and optional explicit `mentions`. Replies remain one level deep.
- `<section>_comment_update` takes a **comment** `id`, replacement `body`, and optional explicit `mentions`. The agent must own the comment and have write on its section. The mention set is replaced; newly mentioned principals receive notifications. The parent cannot change.
- `<section>_comment_delete` takes a **comment** `id`. Ownership and section write are required. A parent with replies remains a deleted placeholder; deleting its last reply also removes that placeholder.
- `<section>_comments` pages top-level comments with their replies. `_activities` returns the timeline, including comments.
- `mentions_list` returns only the caller's mentions in sections it can currently read.
- `mentions_mark_read` takes `ids`, or `all: true` to mark all of the calling agent's own mentions. It never modifies another principal's notifications. Empty requests are rejected.

Comment mutation verifies the authorized section against the locked comment row inside the same transaction as the mutation. Knowing or owning a comment ID from another section does not bypass section grants. Browser and MCP reuse ownership, mention, audit and tombstone logic.

## Section views

`section_views_configure` requires `manage_schema` and replaces the complete ordered configuration:

```json
{
  "section": "clients",
  "views": [
    { "id": "info", "enabled": true, "config": {} },
    { "id": "activity", "enabled": true, "config": {} }
  ]
}
```

Use `item_view_types` first for supported IDs and settings. The repository rejects removal/disable of Info, unknown IDs, duplicate views and invalid settings. Changes are atomic and audited as `agent:<id>`. Disabling/removing an optional view preserves record history. Presentation settings never grant record access.

Future views with settings (for example an Agent view's `system_prompt`) automatically use this catalog/configuration contract when registered. Configuration is not a secret store. See [ITEM_VIEWS.md](ITEM_VIEWS.md) for the extension path.

## Extending and testing

Add section tools through the read/write/delete registration functions in `internal/mcpserver`; do not hardcode client/prospect IDs. Always recheck the intended grant in the callback. Use a section-bound repository method for operations targeting IDs that could belong to another section. Reuse domain validation and audited transactions, rather than calling browser HTTP handlers or duplicating SQL rules.

`httpapi/mcp_coverage_test.go` checks search parity/pagination, mention discovery, comment ownership and section isolation, reply tombstones, caller-only mark-all, view configuration and schema revocation, the separate delete grant, write prerequisites, rejected-grant rollback and migration of legacy grants. Run integration tests with `TEST_DATABASE_URL` pointing only to a disposable database: the test fixture recreates its public schema.
