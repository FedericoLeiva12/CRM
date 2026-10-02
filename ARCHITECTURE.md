# Architecture and contribution guide

## Backend boundaries

The backend uses idiomatic Go `cmd/` and `internal/` packages. `cmd/server` composes dependencies, starts the HTTP listener and handles shutdown. It contains no CRM or database query logic.

`config` validates environment configuration. `database` creates the PostgreSQL pool and applies numbered embedded migrations once, using a transaction and advisory lock to serialize startup across replicas. Add a new SQL file for schema changes; do not edit an already applied migration.

`domain` owns CRM models, field type constants and validation. It imports only the standard library and knows nothing about HTTP, MCP or PostgreSQL. User-facing validation failures have a specific error type.

`store` owns SQL and transactional invariants, grouped into sections, records, agents, accounts, team, and webhooks. It maps database conflicts and missing rows to stable errors. Record saves lock the section while loading its schema and validating values; adding a required field takes the same lock. A write, its audit event, and its webhook outbox rows commit together. `records.data` stays the JSONB source of truth. Each write also replaces that record's rows in `record_values`, a typed projection indexed by section, field, and value, so equality, range, emptiness, and sort stay indexed as fields are added. `activities` is a per-record timeline with an open `type` string; a comment is an entry of type `comment` with `parent_id`, `edited_at` and `deleted_at`, and `comment_mentions` stores structured mentions that double as notifications (`read_at`). Users and agents carry a unique `handle` assigned by a database trigger. Writing an activity calls `EmitTimelineEntry` on that same transaction. `record_links` stores one generic link from a source record to a target section. Password replacement uses compare-and-swap and revokes all sessions in the same transaction. Removing a user deletes that account, and session rows cascade with it. Role changes lock administrator rows so the workspace cannot lose its last administrator.

`webhooks` delivers the outbox. It does not decide which events exist. The worker claims due rows with `FOR UPDATE SKIP LOCKED`, signs and posts them, then records the result. It refuses redirects and dials only addresses that pass the SSRF policy.

`auth` orchestrates password verification, credential generation, server-side sessions and bounded login throttling. `auth/token` is an independent utility package used for hashes and random identifiers; it does not depend on the authentication service.

`httpapi` translates typed request bodies into service/repository calls. The route registry declares session and administrator requirements explicitly. Middleware validates browser origins and authenticates sessions; handlers receive the authenticated user identity and role through a typed context key. Error handling returns safe validation messages while logging unexpected database errors only on the server. Invitation preview and acceptance are the only unauthenticated account routes besides login.

`mcpserver` independently authenticates agent tokens, derives discovery from the current section registry and grants, then invokes the same repository and validation as HTTP. It never imports HTTP handlers. Discovery is not authorization: each tool checks access again at invocation. Database failures deny access. Read, write, and schema-management grants are independent. Schema management defaults to denied and does not grant record access on sections the agent creates.

Dependency direction:

```text
cmd/server → config, database, auth, httpapi, store, webhooks
httpapi → auth, store, domain, mcpserver, webhooks
mcpserver → store, domain
webhooks → store, domain
 auth → store, domain, auth/token
store → domain, auth/token, PostgreSQL driver
domain, config, auth/token → standard library
```

The repository remains a concrete PostgreSQL adapter because the current integration tests exercise real persistence. Introduce an interface at a consuming boundary when a second adapter or a useful isolated test needs it; do not add duplicate abstraction layers in anticipation of an unspecified implementation.

## Frontend boundaries

Remix routes compose feature views and expose loader/action exports. The server-only `workspace.server.ts` module handles browser mutations, typed API calls and form conversion. `invite.$token.tsx` accepts an invitation without an existing session. API cookies stay in server calls; database credentials never reach browser code.

Workspace presentation is split into `records-view`, `fields-view`, `agents-view`, `team-view`, `webhooks-view`, `sidebar`, `workspace-dialogs`, and small editor/cell components. Presentation maps replace repeated nested conditional labels. Records-view owns search/filter state and is keyed by section, which resets those controls when navigation changes the section. Administrator navigation is hidden for members, and administrator views redirect members to the records page. The API still enforces the role.

Record comments live in `workspace/record-history`, `comment-composer`, `comment-body` and `mentions-bell`; the composer and bell talk to the `records.$section.$id`, `mentionables.$section` and `mentions` routes. Shared Radix dialog and permission checkbox components stay in `components`. CRM request/response shapes and field types live in `types/crm.ts`. API response types describe the trusted Go service boundary; Go performs authoritative validation on every write.

## Quality checks

- **Staticcheck** is pinned as a Go module tool. Run `go tool staticcheck ./...` in `backend`.
- **go vet** catches additional correctness issues, and `go test -race` checks tests with the race detector.
- **gofmt** defines Go formatting. `make format-check` fails when files need formatting.
- **ESLint** checks TypeScript, unused declarations, React hooks and nested ternaries, with zero warnings permitted.
- **Prettier** enforces frontend formatting, and TypeScript runs with strict mode.
- `make check` runs backend lint/vet/tests and frontend lint/format/typecheck/production build.
- CI runs the same linters and builds, and provides an isolated PostgreSQL database for integration tests.

The integration test intentionally recreates the public schema. Set `TEST_DATABASE_URL` only to an isolated disposable database. Unit tests cover domain validation, secure origin configuration and limiter limits/expiry. Integration tests cover browser authentication, CSRF, password changes, session revocation, registry changes, dynamic fields, MCP grant isolation, token revocation, invitations, member restrictions, last-administrator protection, transactional audit events, and outbound webhook delivery.

Prefer descriptive names and small functions with a clear responsibility. Comments explain security boundaries, concurrency rules or non-obvious decisions; they should not narrate what a straightforward assignment or loop already says. Keep transport-specific logic out of domain and persistence packages. Add meaningful regression tests when changing authorization or transactional behavior.

## Extension path

Adding a section or a supported field uses the existing registry and needs no new Go handler, sidebar entry or hardcoded MCP permission. A new field type requires a migration for the SQL constraint, a domain constant/validation rule, a frontend type and input renderer. Keep HTTP and MCP routed through the same validation path.

Before public production deployment, resolve the upstream Remix v2 dependency release gate documented in SECURITY.md. Code organization and passing linters do not remove those advisories.
