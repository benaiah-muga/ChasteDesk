# Security

ChasteDesk is a self-hosted product that stores customer conversations and provider credentials. This document describes the controls in the code, the assumptions an operator must hold, and the known gaps.

## Reporting

Use a private GitHub security advisory on `benaiah-muga/ChasteDesk`. Do not open a public issue for an unpatched vulnerability. Include affected version, reproduction steps, and impact. This is an M1 preview, so fixes are best-effort.

## Threat model

The relevant adversaries are:

- A customer sending hostile content through the widget or REST channel.
- A malicious page trying to drive a logged-in operator's browser.
- A forged or leaked widget token used from an allowed or unlisted origin.
- A knowledge source that tries to make the crawler reach internal services.
- An attacker with a stolen database dump looking for provider keys or sessions.
- A compromised provider response trying to trigger tools or exfiltrate data.

The product does not defend against a host-level compromise, a malicious operator with database access, or a compromised AI provider that returns harmful text to a human who acts on it without review.

## Controls in the code

### Authentication

- Argon2id password hashing with 64 MiB memory, 3 iterations, and 2 lanes.
- Random 32-byte session tokens stored as SHA-256 hashes.
- Session cookies are `HttpOnly`, `SameSite=Lax`, and `Secure` when `APP_URL` is HTTPS. Lifetime is seven days.
- Login, setup, and invitation endpoints are rate limited in Postgres.
- Password changes verify the current password and revoke every other session.
- Invitation links are 192-bit random IDs, single use, role-scoped, and expire after seven days.

### Authorization

- Roles are owner, admin, and agent. The capability matrix lives in `internal/auth/rbac.go`.
- Every repository method takes `db.RepoCtx`, which carries the organization id. Handlers never accept an organization id from a request body.
- API keys are scoped to one organization and stored as SHA-256 hashes.
- Widget conversation tokens are HMAC-signed and bound to the organization and conversation.

### Provider secrets

- API keys are encrypted with AES-256-GCM. The key is derived from `APP_SECRET` using HKDF-SHA256 with the info string `secrets-v1`.
- The nonce is random per encryption. The database stores ciphertext, IV, auth tag, key version, and last four characters.
- Responses return only the masked tail. Logs redact configured secret material.
- Rotating `APP_SECRET` makes existing provider keys unreadable. Re-enter them after rotation.

### Ingestion

- The crawler resolves every A and AAAA address before connecting.
- Private, loopback, link-local, carrier-grade NAT, and unique-local ranges are rejected.
- Every redirect target is revalidated. Redirects are capped at three.
- The fetcher allows only `http` and `https`, has a 15-second timeout, and caps the body at 5 MiB.
- Content types are allowlisted to HTML, plain text, and Markdown.
- Uploads are checked by extension and magic bytes and capped at 10 MiB.
- `CHASTESK_INGEST_ALLOW_PRIVATE` is a test-only escape hatch and is ignored when `APP_ENV=production`.

### Prompt injection

- Knowledge chunks and customer messages are delimited as untrusted data in the model prompt.
- The model cannot execute a tool that is not in the saved agent policy, even if it asks for one. Refusals are audited.
- Approval-required tools create a database row and stop the run. Nothing executes without an authenticated decision.
- Action execution has a verification step. A customer-facing success message is only allowed after verification passes.

### Browser security

- Global security headers include CSP, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`, a restrictive permissions policy, and HSTS when `APP_URL` is HTTPS.
- The CSP allows self-hosted scripts only. Inline style is permitted because templates use dynamic chart widths and color swatches. There are no inline scripts.
- Unsafe methods require a same-origin `Origin` when one is present and reject `Sec-Fetch-Site: cross-site` for cookie-authenticated routes.
- Widget routes are exempt from the cookie origin check because they authenticate with a token and their own origin allowlist.
- The widget runs in a Shadow DOM so host page CSS cannot read or restyle its internals. User content is inserted with `textContent`.
- The dashboard has no third-party scripts, analytics, or fonts.

### Data protection

- Audit rows are append-only. Database triggers reject `UPDATE` and `DELETE` on `audit_events`.
- Audit payload keys named `content`, `args`, and `result` are truncated to 500 characters. Configured secret material is redacted.
- The application does not send telemetry.
- Uploaded files are stored under `UPLOADS_DIR` with randomized names.

## Operator responsibilities

1. Set a strong, unique `APP_SECRET` and keep it out of source control.
2. Use HTTPS and set `APP_URL` to the exact public origin.
3. Run `APP_ENV=production` and leave `CHASTESK_INGEST_ALLOW_PRIVATE` unset.
4. Keep Postgres private. Do not publish port 5432.
5. Back up the database and uploads volume, encrypt backups, and test restores.
6. Review audit exports after role changes and approval decisions.
7. Use unique passwords for every operator and remove unused accounts.
8. Keep the host, Docker, and the base images patched.
9. Put a reverse proxy in front of the app and keep SSE buffering disabled.
10. Treat the widget token as public embed data. It is signed, but anyone who can read a page that embeds it can copy it. The origin allowlist is the real boundary.

## Known gaps

- No two-factor authentication.
- No SSO, SAML, or SCIM in this build.
- Single organization per installation.
- No email delivery, so invitations are copyable links.
- No per-tenant encryption keys. Every installation uses one `APP_SECRET`.
- No antivirus scanning for uploads. Parseable formats only.
- No ANN vector index. This is a performance constraint, not a confidentiality one.
- The dashboard JSON API is stable enough for the UI but not yet versioned.

## Hardening checklist

- [ ] `APP_URL` is HTTPS
- [ ] `APP_SECRET` is at least 32 random bytes
- [ ] `APP_ENV=production`
- [ ] `CHASTESK_INGEST_ALLOW_PRIVATE` is unset
- [ ] Postgres is not publicly reachable
- [ ] Reverse proxy sets security headers if it terminates TLS
- [ ] Backups are encrypted and restore-tested
- [ ] Audit log export is reviewed regularly
- [ ] Provider keys are rotated after staff changes
- [ ] Widget origins are reviewed after domain changes
