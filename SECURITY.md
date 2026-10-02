# Security notes

## Implemented boundaries

Production requires an HTTPS APP_ORIGIN. Passwords use bcrypt cost 12; session, invitation, and MCP credentials are random 256-bit tokens stored as SHA-256 hashes. Sessions expire after 12 hours and are revoked on logout, password change, or removal of the user. Browser mutations require the configured origin, cookies are HttpOnly and SameSite=Strict, and production cookies are Secure. API bodies are limited to 1 MiB. The login limiter caps per-email attempts and total tracked keys. There is no public signup. An invitation link is shown once, expires after seven days, and works a single time. Accepting it creates the account and a session. Human roles are administrator and member. Members can read and write records in every section. Schema, agent, and team routes require an administrator. The last administrator cannot be removed or demoted, and an account cannot remove itself. User and invite changes are audited with the acting user. Agent grants default to denied, new sections are denied automatically, and each invocation rechecks its grant. Schema management is a separate grant, denied by default, and does not include read or write even on a section that agent creates. Schema changes are audited with the agent identity in the same transaction as the definition change. SQL queries use parameters. Record changes and their audit entries share a transaction. Partial updates validate the merged record with the same rules as a full save. List filters accept only fields defined on that section and send comparison values as parameters; `contains` escapes `%`, `_`, and `\`. A timeline author is the authenticated user or agent, not a value supplied in the tool arguments. Readers of a section can list that section's timeline. Converting a record requires read and write on the source section and write on the target section, using the existing grants. An agent receives the other record's name only when it can read that section; the link ids are part of the record it can already read.

The Docker deployment exposes only Caddy. App services run as non-root with read-only filesystems and dropped capabilities. Caddy adds content type, framing, referrer and permissions headers. Its CSP permits inline scripts/styles required by the current Remix rendering; a future nonce-based policy can tighten this.

## Upstream dependency release gate

The explicitly requested Remix v2 is retained. On verification, npm reported 12 dependency advisories (4 moderate, 8 high), with 8 in the production dependency tree (2 moderate, 6 high). These counts include parent packages affected by the same underlying dependency; they are not 12 distinct exploitable paths.

The remaining tree includes React Router v6 and turbo-stream v2. The turbo-stream advisory specifically concerns Remix 2.9+ **when single-fetch is enabled**, which this app does not enable. Fixed, generated internal navigation avoids user-supplied destinations, reducing the open redirect exposure. This is contextual mitigation, not a claim that all upstream advisories are resolved.

Compatible development dependency overrides pin patched tar, toml, ESTree conversion, esbuild and nested Vite versions. Both the normal and container builds were checked after these changes. Do not replace incompatible protocol dependencies with npm overrides just to silence the audit.

Before public production use, migrate the frontend to the supported React Router framework successor, or adopt verified upstream patches for the remaining Remix dependencies and rerun typechecks, builds, integration tests and the audit. The Remix team recommends React Router for new projects: https://remix.run/blog/incremental-path-to-react-19. Advisory context: https://github.com/remix-run/react-router/security/advisories/GHSA-rxv8-25v2-qmq8.

## Scope

This is a single-workspace application. The first account is bootstrapped as an administrator; further people join only through an invitation link. MFA, password recovery, OAuth for MCP, agent token expiry, database-level row permissions and multi-tenancy are not implemented. Agent write access includes permanent deletion; grant it only where intended. Member access includes creating, replacing, and deleting records. Permissions and revocation apply to subsequent requests, not cancellation of an already running request. Read access includes all fields in the section. Test data and integration database credentials must not be reused in production.

No public deployment has been performed or security certification claimed.
