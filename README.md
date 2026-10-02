# Sira CRM

An extensible, single-workspace CRM with a Go API, PostgreSQL, Remix v2, Tailwind, Radix dialogs and checkboxes, and an authenticated MCP endpoint built on the official Go MCP SDK.

## Features

- Clients and prospects with create, edit, delete, search, status filters and database persistence.
- Add sections from the UI, such as Employees, without changing code.
- Section-specific custom text, email, number, date and boolean fields, validated by the same backend for browser and agent writes.
- Administrator sign-in, bcrypt cost 12, opaque hashed session tokens, HttpOnly/SameSite cookies, HTTPS-only production cookies, origin checks, login throttling, password changes and session revocation.
- One-time agent token display, hashed tokens at rest, independent read/write grants per section and immediate token revocation on subsequent requests.
- New sections automatically appear in permission settings and MCP discovery. All new permissions default to denied.
- Record mutations are transactionally audited, with agent identity recorded.
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

`ADMIN_EMAIL` / `ADMIN_PASSWORD` only create the first administrator when the users table is empty. Changing those environment variables does not reset a password. Change the password under **Account security**, which invalidates all browser sessions. There is no public signup, email password recovery or team invitation flow in this version. All human accounts are workspace administrators; fine-grained permissions currently apply to agents.

For a local Compose deployment, set `SITE_ADDRESS=http://localhost` and `APP_ORIGIN=http://localhost`. This uses port 80 and disables Secure cookies for local HTTP. Never use this configuration for a public host.

## Deploying with Coolify

Use this when Coolify's Traefik proxy terminates TLS for the public site. Keep `compose.yaml` for a host where Caddy itself obtains certificates.

1. Create a Docker Compose application from this repository. Set the base directory to `/` and the Docker Compose location to `/compose.coolify.yaml`.
2. Enable **Preserve Repository During Deployment** so the relative bind `./infra/Caddyfile.coolify` is available after Coolify clones the repo.
3. Set the `proxy` service domain to `https://crm.projects.invboy.com:80`. Assign the public domain only on `proxy`. Traefik owns certificates and forwards plain HTTP to Caddy on port 80.
4. Set the environment variables Coolify reads from `${...}` in the compose file:
   - `POSTGRES_PASSWORD` — URL-safe random value
   - `ADMIN_EMAIL`
   - `ADMIN_PASSWORD` — 14–72 bytes; creates the first administrator only
   - `APP_ORIGIN=https://crm.projects.invboy.com` — exact origin, no trailing slash or path

Caddy listens with `SITE_ADDRESS=:80` and automatic HTTPS off. `/mcp` and `/healthz` go to the API, and every other path goes to the web app, with the same security headers as `infra/Caddyfile`. No service publishes host ports. `APP_ORIGIN` still starts with `https://`, so the session cookie is `Secure`, and browser origin checks compare the `Origin` header to that same value.

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

## Connect an MCP client

1. Sign in and open **Agent access** → **Connect agent**.
2. Give the agent a name. Copy the token while it is displayed; it cannot be retrieved later.
3. Select read and/or write per section and save. Write access includes creation, complete replacement, and permanent deletion. Read and write are independent.
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
| Read | `<section>_list` |
| Write | `<section>_save`, `<section>_delete` |

A save with no `id` creates a record. A save with an `id` replaces its complete data; omit optional values to clear them. Use `sections_schema` first for field identifiers, types and required flags. Record lists return the latest 500 records; search/filter in the UI works within those 500. Paginated queries are an extension point for larger datasets.

After adding a section or changing grants, refresh the client's tool list. Permissions are checked again on each tool invocation. Revoked tokens fail authentication on the next HTTP request; requests already in progress may finish.

Example save arguments:

```json
{"data":{"name":"Mara Santos","email":"mara@example.test","company":"Northline Studio","status":"Active","value":18500}}
```

## Extend the CRM

The `sections` registry drives the sidebar, record routes, field editor, permission matrix and MCP tool generation. UI-created sections start with a required `name` field. Every section is already supported by the generic record API and editor.

Fields live in `fields` with a stable identifier and a supported type; values live in `records.data` as JSONB. New required fields are rejected while records exist so existing records remain valid. In this version, definitions can be added but not renamed/deleted/type-changed. Schema changes affecting existing values should use an explicit migration and backfill. To introduce another field type, update the SQL constraint, Go validation, and frontend form controls together.

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
web/app/
  routes/                    # Remix route composition and sign-in
  features/workspace/        # Records, fields, agents, editors and server actions
  components/                # Shared accessible Radix controls
  types/                     # Typed CRM transport models
```

Read [ARCHITECTURE.md](ARCHITECTURE.md) for dependency boundaries, extension guidelines and quality tooling.

The integration suite includes creation of Employees and checks that the section appears automatically as denied, then verifies write-only MCP access, permission removal and token revocation.

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
