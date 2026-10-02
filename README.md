# Sira CRM

An extensible, single-workspace CRM with a Go API, PostgreSQL, Remix v2, Tailwind, Radix dialogs and checkboxes, and an authenticated MCP endpoint built on the official Go MCP SDK.

## Features

- Clients and prospects with create, edit, delete, search, status filters and database persistence.
- Add sections from the UI, such as Employees, without changing code.
- Section-specific custom text, email, number, date and boolean fields, validated by the same backend for browser and agent writes.
- Administrator sign-in, bcrypt cost 12, opaque hashed session tokens, HttpOnly/SameSite cookies, HTTPS-only production cookies, origin checks, login throttling, password changes and session revocation.
- Team invitations by one-time link. Administrators manage people and roles. Members can view and edit records in every section.
- One-time agent token display, hashed tokens at rest, independent read/write grants per section and immediate token revocation on subsequent requests.
- New sections automatically appear in permission settings and MCP discovery. All new permissions default to denied.
- Record mutations are transactionally audited, with agent identity recorded.
- Generic outbound webhooks. Administrators subscribe an https endpoint to workspace events. Delivery is asynchronous from a transactional outbox.
- Non-root application containers, private API/database services, persistent storage and Caddy HTTPS.

## Release status

The application builds and runs. **The requested Remix v2 dependency tree still has upstream security advisories. Do not treat this as security-cleared for public production deployment.** Compatible build dependency overrides remove the critical tar advisory. Run `cd web && npm audit` for the current report; `npm audit --omit=dev` reports the runtime subset. See [SECURITY.md](SECURITY.md) for context and the recommended supported framework migration. The app deliberately does not enable Remix single-fetch.

## Production configuration

Requires Docker Engine with Compose, a domain pointing to the deployment host, and inbound TCP ports 80/443 (UDP 443 is optional for HTTP/3).

1. Copy `.env.example` to `.env` and restrict permissions with `chmod 600 .env`.
2. Generate separate random values for `POSTGRES_PASSWORD` and `ADMIN_PASSWORD`, for example with `openssl rand -hex 24`. Use URL-safe database passwords because the password is embedded in a PostgreSQL URL. Administrator passwords must be 14–72 bytes.
3. Set `ADMIN_EMAIL`, `APP_ORIGIN=https://your-domain`, and `SITE_ADDRESS=your-domain`. `APP_ORIGIN` is an exact origin without a trailing slash or path. HTTP is accepted only for localhost or 127.0.0.1.
4. Resolve the release gate above, then run:

```sh
docker compose up -d --build
curl --fail https://your-domain/healthz
```

Caddy provisions and renews certificates for public domains. PostgreSQL and the API are not published to the host. The browser talks to Remix, whose loaders/actions call the private API. Caddy routes `/mcp` to Go. All administrator capabilities require a browser session; agent tokens cannot access them.

`ADMIN_EMAIL` / `ADMIN_PASSWORD` only create the first administrator when the users table is empty. That account stays an administrator. Changing those environment variables does not reset a password. Change the password under **Account security**, which invalidates all browser sessions. There is no public signup or email password recovery. Further people join only through an invitation.

For a local Compose deployment, set `SITE_ADDRESS=http://localhost` and `APP_ORIGIN=http://localhost`. This uses port 80 and disables Secure cookies for local HTTP. Never use this configuration for a public host.

## Deploying with Coolify

Use this when Coolify's Traefik proxy terminates TLS for the public site. Keep `compose.yaml` for a host where Caddy itself obtains certificates.

1. Create a Docker Compose application from this repository. Set the base directory to `/` and the Docker Compose location to `/compose.coolify.yaml`.
2. Set the `proxy` service domain to `https://crm.projects.invboy.com:80`. Assign the public domain only on `proxy`. Traefik owns certificates and forwards plain HTTP to Caddy on port 80.
3. Set the environment variables Coolify reads from `${...}` in the compose file:
   - `POSTGRES_PASSWORD` — URL-safe random value
   - `ADMIN_EMAIL`
   - `ADMIN_PASSWORD` — 14–72 bytes; creates the first administrator only
   - `APP_ORIGIN=https://crm.projects.invboy.com` — exact origin, no trailing slash or path

The proxy image is built from `infra/Dockerfile.proxy`, which copies `Caddyfile.coolify` into the image. Coolify runs the stack from its application directory, so the compose file does not bind-mount repository files. Caddy listens with `SITE_ADDRESS=:80` and automatic HTTPS off. `/mcp` and `/healthz` go to the API, and every other path goes to the web app, with the same security headers as `infra/Caddyfile`. No service publishes host ports. `APP_ORIGIN` still starts with `https://`, so the session cookie is `Secure`, and browser origin checks compare the `Origin` header to that same value.

## Local development

Run a local PostgreSQL instance and create a database, then in separate terminals:

```sh
cd backend
export DATABASE_URL='postgres://sira:YOUR_URL_SAFE_PASSWORD@localhost:5432/sira?sslmode=disable'
export APP_ORIGIN='http://localhost:3000'
export ADMIN_EMAIL='your-email@example.com'
# Set ADMIN_PASSWORD in your shell securely; it is needed only on first startup.
go run ./cmd/server
```

```sh
cd web
npm ci
export API_URL='http://localhost:8080'
export APP_ORIGIN='http://localhost:3000'
npm run dev
```

Open http://localhost:3000. Versioned schema migrations are bundled into the Go binary and applied once under a transaction-scoped advisory lock. Production starts with empty records; test or demo data are never seeded into your deployment.

## Team

Administrators open **Team**, enter an email and a role, and receive a one-time link such as `https://your-domain/invite/sira_inv_…`. Copy it then; only a SHA-256 hash is stored, and Sira does not send email. The link expires after 7 days, works once, and can be revoked while it is pending. Inviting an address again replaces an expired invitation. The invitee chooses a name and a password (14–72 bytes) and is signed in.

**Administrator** can do everything in the workspace, including team management, agent tokens, webhooks, and sections and fields. **Member** can view and edit records in every section. The API rejects member calls to administrator routes, and the workspace redirects members away from administrator pages. The workspace must keep one administrator, and nobody can remove their own account. Removing someone else ends their sessions immediately. User and invite changes are audited.

## Webhooks

Administrators open **Webhooks** and add an endpoint. Members cannot open the page or call `/api/webhooks`. Each endpoint has an https URL, an optional description, one or more event types, an optional section filter, and at least one authentication method:

- A signing secret. The worker sends `X-CRM-Signature: sha256=<hex>` where the hex is HMAC-SHA256 of `<unix seconds>.<raw body>` using that secret, plus `X-CRM-Timestamp` with the same unix seconds.
- An optional custom header name and secret value, sent exactly as stored. Use this for a static sender key such as `Authorization: Bearer …`.

Secrets are write-only after save. Leave the secret fields blank to keep them, or check the remove box to clear one. An endpoint must keep a signing secret or a custom header.

Subscribed types are `record.created`, `record.updated`, `record.deleted`, `section.created`, `field.created`, `timeline.entry_created`, and `comment.mentioned`. `webhook.test` is not a subscription. **Send test event** queues one test for that endpoint, including while it is disabled. A section filter skips events for other sections. Events with no section, such as a workspace-wide change that does not name one, are delivered only to endpoints with no section filter.

The body is JSON, schema version 1:

```json
{
  "schema_version": 1,
  "id": "evt_…",
  "type": "record.updated",
  "occurred_at": "2026-10-02T19:00:00.000Z",
  "actor": { "kind": "user", "id": "…", "name": "Ada" },
  "section": { "id": "clients", "name": "Clients" },
  "record_id": "…",
  "data": {
    "changed_field_ids": ["status"],
    "fields": { "status": "Active" }
  }
}
```

`section` and `record_id` are null when they do not apply. `actor.kind` is `user`, `agent`, or `system`. `data` is always an object:

| Event | `data` |
| --- | --- |
| `record.created` | `fields` with the stored values |
| `record.updated` | `changed_field_ids` and `fields` containing only the new values. A cleared field is listed in `changed_field_ids` and omitted from `fields` |
| `record.deleted` | `fields` with the values that were removed |
| `section.created` | `id`, `name` |
| `field.created` | `field` with `id`, `label`, `type`, `required` |
| `timeline.entry_created` | `entry_id`, `kind`, `body` truncated to 500 characters |
| `comment.mentioned` | `entry_id`, `section` (`id`, `name`), `record_id`, `mentioned` (`kind` `user` or `agent`, `id`, `handle`), `author` (`kind`, `id`, `name`, `handle`), `body` truncated to 500 characters, `parent_id` (`null` for a top-level comment) |
| `webhook.test` | `message` |

Every request also sends `Content-Type: application/json`, `User-Agent: SiraCRM-Webhooks/1`, `X-CRM-Event` with the event type, and `X-CRM-Idempotency-Key` set to the event id. Delivery is at least once, so receivers should treat that key as the idempotency key. Redirects are not followed. The request times out after 10 seconds. Failures retry up to 5 attempts with exponential backoff from 5 seconds to 5 minutes. Ten consecutive failures, excluding test events, disable the endpoint and the page shows it as paused. Enabling it clears that pause. Finished deliveries and outbox rows that nothing still references are deleted after 30 days.

The outbox row is inserted in the same database transaction as the record, section, field, or timeline change. The API returns before delivery. Saving an activity, including an automatic status change, calls `store.EmitTimelineEntry` before commit. A comment calls the same hook and then queues one `comment.mentioned` event per newly mentioned user or agent, in the same transaction. An external agent can subscribe to `comment.mentioned` to be woken when it is @mentioned.

Production accepts https only and blocks private, loopback, link-local, multicast, and other non-public addresses when the endpoint is saved and again when the worker connects. Local development (`APP_ORIGIN=http://localhost:3000` or `http://127.0.0.1:3000`) may use `http://localhost` or `http://127.0.0.1` so a receiver on the same machine can be tested. Hostnames are resolved and the worker dials a checked address.

Endpoint create, update, delete, enable, disable, and automatic pause are written to `audit`.

## Connect an MCP client

1. Sign in and open **Agent access** → **Connect agent**.
2. Give the agent a name. Copy the token while it is displayed; it cannot be retrieved later.
3. Select read and/or write per section, and turn on **Manage schema** only when the agent should add sections and fields. Write access includes creation, complete replacement, and permanent deletion. Read, write, and schema management are independent. Manage schema starts off.
4. Configure an MCP client that supports Streamable HTTP and Authorization headers:

```json
{
  "mcpServers": {
    "sira": {
      "url": "https://your-domain/mcp",
      "headers": { "Authorization": "Bearer YOUR_AGENT_TOKEN" }
    }
  }
}
```

Client configuration syntax varies; use its remote Streamable HTTP connector. This server uses stateless JSON responses and static bearer credentials, not an OAuth discovery flow. Clients requiring OAuth-only connections need an OAuth layer before they can connect.

Tools are generated per authorized section:

| Permission | Tools |
| --- | --- |
| Always available | `sections_schema` (only accessible section definitions) |
| Always available | `mentions_list`, `mentions_mark_read` (the calling agent's own mentions) |
| Read | `<section>_list`, `<section>_get`, `<section>_activities`, `<section>_comments` |
| Write | `<section>_save`, `<section>_update`, `<section>_delete`, `<section>_log_activity`, `<section>_comment` |
| Read and write | `<section>_convert` |
| Manage schema | `sections_create`, `fields_add` |

`<section>_convert` also requires write on the target section. That check happens when the tool is called, because the target is an argument. No extra permission type is required.

A save with no `id` creates a record. A save with an `id` replaces its complete data; omit optional values to clear them. `<section>_update` changes only the fields in `data`; `null` clears a field and returns the full record. Use `sections_schema` first for field identifiers, types and required flags. A list call with no arguments still returns the latest 500 records as `{"records","limit"}`. Filters, sort, limit, and cursor page the whole section and add `next_cursor` and `total`. The record view shows links and the read-only timeline.

`sections_create` takes the same `id` and `name` as the admin form. `fields_add` takes `section`, `id`, `label`, `type` (`text`, `email`, `number`, `date`, or `boolean`), and `required`. These tools use the same validation as the admin UI: a new required field is rejected while the section has records, and definitions cannot be renamed, deleted, or have their type changed. Each change is audited with the agent identity. Creating a section does not grant read or write on it, including for the agent that created it. Those grants stay denied until an administrator saves them.

After adding a section, changing the schema, or changing grants, refresh the client's tool list. The server rebuilds tools on each request and sends `notifications/tools/list_changed` when the list changes. Permissions are checked again on each tool invocation. Revoked tokens fail authentication on the next HTTP request; requests already in progress may finish.

Example save arguments:

```json
{"data":{"name":"Mara Santos","email":"mara@example.test","company":"Northline Studio","status":"Active","value":18500}}
```

Argument shapes for the record tools, using `prospects` as an example of any section id:

```json
{"id":"RECORD_ID","data":{"status":"Ganado","notes":null}}
```

`prospects_update` — `id` (string), `data` (object). Omitted fields stay as they are.

```json
{"id":"RECORD_ID"}
```

`prospects_get` — `id` (string). The record includes `links`: `{section_id, section_name, record_id, direction, name?}`. `direction` is `outgoing` or `incoming`. `name` is present only when the agent can read the other section.

```json
{"filters":[{"field":"status","op":"eq","value":"Contactado"},{"field":"next_action_at","op":"lte","value":"2026-10-05"}],"sort":{"field":"updated_at","direction":"desc"},"limit":100,"cursor":"OPAQUE"}
```

`prospects_list` — every argument is optional. `filters` is `{field, op, value}` combined with AND. `op` is `eq`, `neq`, `contains`, `gt`, `gte`, `lt`, `lte`, `is_empty`, or `not_empty`. Range operators apply to number and date fields; `contains` applies to text and email. `sort.field` is a field id or `updated_at`; `sort.direction` is `asc` or `desc`. `limit` is 1–500. Send the returned `next_cursor` back with the same filters and sort. With no arguments the response stays `{"records","limit":500}`.

```json
{"id":"RECORD_ID","type":"email_enviado","date":"2026-10-02","summary":"Sent the introduction","channel":"email","ref":"thread-id"}
```

`prospects_log_activity` — `id`, `type`, `date`, `summary`; optional `channel` and `ref`. `type` is any short string, not a fixed list. `date` is `YYYY-MM-DD` or RFC3339. The author is the authenticated agent or user. An entry with `type` `comment` is stored as a comment, so it can be mentioned in and replied to like any other.

```json
{"id":"RECORD_ID"}
```

`prospects_activities` — `id`. Entries come back newest first.

```json
{"id":"RECORD_ID","target":"clients","mapping":{"source_note":"alias"},"overrides":{"email":"desk@example.test"},"status":"Ganado"}
```

`prospects_convert` — `id`, `target` section id; optional `mapping` (source field id to target field id), `overrides`, and `status`. Fields that share an id are copied first, then `mapping`, then `overrides`. `status`, when present, is written to the source record's `status` field. The same source cannot be converted into the same target section twice. The response is `{source, target}`, and each record includes `links`. There is no stored `prospect_id` or `client_id`; either id is the linked record id.

### Comments and @mentions

A comment is a timeline entry of type `comment` on any record, in any section. It has an author (user or agent), a plain-text body of up to 4000 characters with `` `code` `` and `**bold**`, `created_at`, an optional `edited_at`, and an optional `parent_id` for one level of replies. Authors edit and delete their own comments; administrators can delete any. Deleting a comment that has replies leaves a "deleted" placeholder so the thread stays readable.

Every user and agent has a unique, stable `@handle` (1–32 characters of `a-z 0-9 _ -`). It is derived from the name for agents and the name or email for users, and a number is appended on a collision. Users and agents share one namespace. A handle counts as a mention when it starts the text or follows a non-word character, so `me@example.test` and paths are ignored, as are code spans and fences. Mentions are stored structurally, up to 20 per comment, and each one is a notification for that user or agent. An agent is only mentioned if it can read the section. Editing a comment replaces its mention set; only newly mentioned principals are notified. The identifier `mentions` is reserved and cannot be used for a section.

In the web app, the record dialog shows a composer with `@` autocomplete of the team and of agents that can read the section. Mentions render as chips, and the timeline interleaves comments with other activity, newest first, with reply, edit and delete. The bell in the header lists the current user's mentions, links to the record, and marks them read.

```json
{"id":"RECORD_ID","body":"@agent-name please check this, cc @ana-lopez","parent_id":"OPTIONAL_COMMENT_ID","mentions":["agent:AGENT_ID"]}
```

`prospects_comment` — `id`, `body`; optional `parent_id` (a top-level comment on the same record) and `mentions` (ids, `user:<id>` or `agent:<id>`), in addition to @handles in `body`. Needs write on the section. Returns `{comment, unresolved_mentions}`; unresolved entries are handles or ids that matched nobody who can read the section. The comment is not rejected.

```json
{"id":"RECORD_ID","limit":50,"cursor":"OPTIONAL"}
```

`prospects_comments` — `id`; optional `limit` (1–200, default 50) and `cursor`. Needs read. Returns `{comments, next_cursor}` with top-level comments newest first and `replies` oldest first.

```json
{"unread_only":true,"limit":100}
```

`mentions_list` — optional `unread_only` and `limit` (1–500, default 100). Returns `{mentions, unread_count}`. Each mention has `id`, `entry_id`, `section_id`, `section_name`, `record_id`, `record_name`, `parent_id`, `author`, `body`, `created_at`, and `read_at`. An agent only sees mentions in sections it can currently read.

```json
{"ids":["MENTION_ID"]}
```

`mentions_mark_read` — `ids`. Returns `{marked}`. Only the caller's own mentions change.

## Extend the CRM

The `sections` registry drives the sidebar, record routes, field editor, permission matrix and MCP tool generation. UI-created sections start with a required `name` field. Every section is already supported by the generic record API and editor.

Fields live in `fields` with a stable identifier and a supported type; values live in `records.data` as JSONB, with a typed `record_values` projection for indexed filters. New required fields are rejected while records exist so existing records remain valid. In this version, definitions can be added but not renamed/deleted/type-changed. Schema changes affecting existing values should use an explicit migration and backfill. To introduce another field type, update the SQL constraint, Go validation, and frontend form controls together.

Code structure:

```text
backend/
  cmd/server/                # Process startup and graceful shutdown
  internal/
    config/                  # Environment settings and origin validation
    database/migrations/     # Embedded, versioned SQL migrations
    domain/                  # CRM models and shared field validation
    store/                   # PostgreSQL queries and transactional writes
    auth/                    # Passwords, sessions and login throttling
      token/                 # Credential generation and hashing
    httpapi/                 # Routes, middleware and small HTTP handlers
    mcpserver/               # Authenticated MCP transport and dynamic tools
    webhooks/                # Outbound delivery worker, signatures and SSRF checks
web/app/
  routes/                    # Remix route composition, sign-in and invitation acceptance
  features/workspace/        # Records, fields, agents, team, webhooks, editors and server actions
  components/                # Shared accessible Radix controls
  types/                     # Typed CRM transport models
```

Read [ARCHITECTURE.md](ARCHITECTURE.md) for dependency boundaries, extension guidelines and quality tooling.

The integration suite includes creation of Employees and checks that the section appears automatically as denied, then verifies write-only MCP access, permission removal and token revocation. It also checks that schema tools are denied until granted, match admin validation, leave new sections closed, and stop working when the grant is removed. Team tests cover invitation creation, acceptance, expiry, replacement, revocation, single use, member denial of administrator routes, and last-administrator protection. Webhook tests cover signatures, custom headers, retry and backoff, automatic pause, outbox durability, member denial, and the SSRF block.

## Verify

```sh
cd web
npm run check
npm audit
```

```sh
cd backend
go tool staticcheck ./...
go vet ./...
go test -race ./...
```

To run database integration tests, set `TEST_DATABASE_URL` to a disposable PostgreSQL database and repeat `go test -race ./...`. **The integration test drops and recreates the public schema. Never point it at a development or production database containing useful data.** Without that variable the database test is skipped. CI provides a dedicated PostgreSQL service and runs it automatically.

```sh
docker compose config --quiet
docker compose build
```

## Operations

Back up the database and protect backup files because they contain CRM records and password/token hashes:

```sh
umask 077
docker compose exec -T db pg_dump -U sira -d sira -Fc > sira-backup.dump
```

Test restoration in an isolated database before relying on a backup. `docker compose down` preserves named volumes; `down -v` removes all persisted data. Retain the `postgres_data`, `caddy_data` and `caddy_config` volumes between releases.

Record audit events are stored in `audit`; there is no audit viewer yet. Set retention and monitoring policies appropriate to your deployment, including cleanup of expired sessions. Login throttling is in-process and intended for a single API replica; add shared throttling before horizontally scaling. Deploy schema migrations in a controlled step before multiple API replicas. Keep secrets in a deployment secret manager, restrict host/Docker access, and keep runtime images and dependencies patched.

Local verification on this machine uses a generated `web/node_modules` symlink to `/tmp/sira-web-dependencies/node_modules` to avoid macOS offloading dependency files in Documents. This is ignored by Git and Docker. If the temporary directory is removed or after a reboot, remove the symlink and run `npm ci` again, ideally from a checkout outside a cloud-synced folder.
