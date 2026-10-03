# Item views: architecture and extension guide

An item opens as a full page at `/?section=clients&record=<id>`. `tab=<view-id>` selects a configured view. Opening a record from a list or a mention uses this same screen. Returning to the section keeps its search, filters and sorting.

Every existing and newly created section starts with **Info only**. Info renders field values as text; **Edit** opens the shared field editor. Save validates and persists through the existing record write path. Cancel discards the local draft. Navigation and reloads warn about unsaved changes. Tab navigation is rendered only when more than one supported view is enabled.

**Activity** is an optional built-in view that reuses comments, mentions, the timeline and related records. Install it from **Settings → Sections → Item views** to use those features in a section. Disabling or removing the view does not delete record history. A previously shared mention link still opens the referenced item; its Activity view is visible only if that section has enabled it.

## Boundaries

| Responsibility | Code |
| --- | --- |
| Supported IDs, labels and configuration fields | `backend/internal/itemviews/catalog.go` |
| Authoritative validation | `itemviews.Validate` |
| Section settings model | `domain.SectionView` |
| Persistent optional views and order | `store/item_views.go`, migration `008_section_item_views.sql` |
| Session-protected catalog and admin-only updates | `httpapi/item_views.go` |
| View component contract | `web/app/features/record-detail/types.ts` |
| Frontend renderer registry | `web/app/features/record-detail/registry.ts` |
| URL, tabs and unsaved navigation | `record-detail/record-detail.tsx` |
| Shared settings UI | `section-settings/item-views-editor.tsx`, `view-config-fields.tsx` |
| Browser mutation adapter | `workspace/workspace.server.ts` |

Go provides catalog metadata; the browser does not duplicate view labels or configuration definitions. The renderer registry maps IDs to components. A view owns its content and data requests; the host owns enabled views and navigation. No growing conditional chain is needed in the workspace route when new views are added.

`GET /api/item-view-types` returns the catalog to signed-in users. `GET /api/sections` includes each section's ordered `views`. Administrators update a section with `PUT /api/sections/{section}/views`:

```json
{
  "views": [
    { "id": "info", "enabled": true, "config": {} },
    { "id": "activity", "enabled": true, "config": {} }
  ]
}
```

The repository validates before writing, locks the section, replaces optional view settings, and writes an audit entry in one transaction. Unknown IDs, duplicate IDs, unregistered settings, invalid setting types and required/length violations are rejected. Info must be present and enabled in update requests. Info is synthesized on reads and is never a database row, so even deleting all optional settings cannot remove it. A database constraint rejects attempts to store a mutable Info row.

Agents with `manage_schema` use `item_view_types` and `section_views_configure` to discover and update this same configuration. See [MCP.md](MCP.md) for arguments and grants.

Settings configure presentation, **not authorization**. Disabling Activity hides its UI; it does not revoke access to comments through API or MCP. All future view-specific endpoints must enforce their own session/section/agent authorization. Existing MCP grants and record write checks remain independent.

## Add a view

1. Add a definition to `itemviews.Catalog()` with a stable lowercase ID. Keep `Required` false for optional views. Describe any non-secret configuration fields using `ConfigField`; supported editors are `text` and `textarea`. A field can be required and have a Unicode character limit.
2. Implement a component in `web/app/features/record-detail/`. It receives `ItemViewProps`: `section`, `record`, authenticated `viewer`, its `settings`, and `onPendingChangesChange` for drafts. Use the existing browser API adapters and server actions; keep credentials and provider calls on the server.
3. Register the component in `itemViewPlugins` under the same ID. This makes it eligible for the settings editor and the record screen. The generic host handles tabs, URL selection and the single-view case.
4. If the view needs server operations, implement them behind authenticated Go routes and a server-only Remix adapter. If it needs persistent domain data, add a numbered migration and repository operations; configuration alone needs no new table or column.
5. Test configuration validation, role checks and persistence. Verify desktop/mobile, the single-view case, disabled views, loading/error states and draft navigation. Deploy the backend catalog and frontend renderer together, then enable the view in the desired sections.

The settings editor can install, enable/disable and remove optional views. Enabled views keep the configured order; Info is always first. Turning off or removing a view preserves its underlying record data. Removed configuration is discarded when saved. Unknown renderers are omitted from item tabs; Info remains available, and an unavailable installed view can be removed from Settings.

## Future agent view and system prompt

When an agent implementation is ready, its catalog entry can include:

```go
{
    ID: "agent",
    Label: "Agent",
    Description: "Discuss this item with your workspace assistant.",
    ConfigFields: []ConfigField{
        {
            Key: "system_prompt",
            Label: "System prompt",
            Type: "textarea",
            Required: true,
            MaxLength: 10000,
        },
    },
}
```

The existing settings editor will render that textarea and persist a different prompt for each section. An `AgentView` component registered under `agent` receives its setting through `settings.config.system_prompt`. No changes are needed to the item tab host or section settings form.

Before implementing chat, add an authenticated server endpoint that loads the section configuration and authorized record itself; do not trust a prompt or item context supplied by a browser. Keep provider API keys in server environment/secrets, not `config`. Section configuration is returned to signed-in workspace users and can appear in schema responses, so it must contain **no credentials or private provider secrets**. A system prompt does not grant tool permissions; agent actions still use the configured MCP grants. Decide conversation ownership, persistence and access policy explicitly.

The current application does not register an Agent view, call a provider, or display a nonfunctional chat placeholder. This is an extension example for a future complete feature.

## Verification

`itemviews/catalog_test.go` covers mandatory Info, unknown and duplicate views, unsupported settings and text validation. `httpapi/item_views_test.go` uses PostgreSQL to check migration defaults, new sections, enable/disable/remove persistence, isolation across sections, rejected writes, audit events, authentication and member restrictions. Run database tests against an isolated disposable `TEST_DATABASE_URL`; the integration helpers recreate the public schema.
