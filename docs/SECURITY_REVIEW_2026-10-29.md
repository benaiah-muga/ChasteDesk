# Independent security review, 2026-10-29

An independent read-only review of the production-hardening branch was run against commit `73d4aa3` plus the local hardening changes. It covered authentication, TOTP, session handling, team administration, SCIM, SLA scanning, inbound and outbound email, tenant scoping, secret storage, audit, migrations, CI, and Compose deployment.

No critical findings were reported. There was no confirmed cross-organization data access, SQL injection, XSS, CSRF bypass, or SMTP open relay.

## Findings addressed

- Production sign-up is disabled and sign-up is rate limited. A case-insensitive unique index on `user.email` was added.
- The AI knowledge tool now uses the configured knowledge scopes instead of always searching public and internal sources. A regression test covers public versus internal scope.
- Inbound email rejects self-addressed mail, automatically submitted mail, and explicit SPF, DKIM, or DMARC failures. Message-ID deduplication and ticket threading remain.
- SCIM last-owner checks now use live membership state, and console role changes synchronize SCIM state. SCIM tokens are revoked when their creator is demoted from owner or removed.
- The legacy sign-in path no longer creates a session when the 2FA lookup fails.
- The worker no longer panics when no embedding provider is configured.
- Production defaults were hardened: a known placeholder `APP_SECRET` is rejected, non-local production origins must use HTTPS, and Compose binds the web port to localhost for a TLS proxy.

## Residual risks

- Email sender authentication still depends on the inbound provider. The app rejects explicit authentication failures, but it cannot verify a provider that omits authentication result headers. Configure SPF, DKIM, and DMARC and reject or quarantine unauthenticated mail at the provider.
- Enabling internal knowledge scope can expose internal excerpts in customer-facing citations. Keep internal scope disabled for public desks.
- SCIM tokens are owner-equivalent and do not expire, though they are revoked on creator demotion or removal. Require reauthentication before minting one.
- Last-owner checks now read live membership state, but the count-and-update sequence is not fully atomic. Concurrent owner demotions should be serialized at the database level in a future migration.
- Provider base URLs are admin-controlled and can reach private network addresses. Restrict provider endpoints with an operator allowlist or network egress policy.
- Public widget new-conversation throttling is organization-scoped, and the raw `X-Forwarded-For` header is trusted in one widget path. Place the desk behind a trusted proxy that overwrites forwarding headers.
- Invitation tokens are stored as random IDs, not hashes. Protect database backups.
- The documented log scrubber is not wired into all logging paths.
- There is no outbound email bounce, complaint, or suppression handling.
- The migration runner has no advisory lock; run one migrate process at a time.
- The app is single-organization by design. Multi-organization support requires changes to inbound routing and membership resolution.

## Verification after remediation

- Full Go test suite: `go test -p 1 -timeout 30m ./...` against an isolated Postgres test database.
- Email delivery: production-mode web and worker with a local SMTP capture, inbound Postmark-shaped webhook, threaded reply, and transactional invitation. Two messages were sent and captured with SPF/DKIM/DMARC not applicable to the local capture server.
- Backup and restore: custom-format `pg_dump` from PostgreSQL 16, restore into a separate database, row-count and migration checks, and a successful web boot against the restored database.
