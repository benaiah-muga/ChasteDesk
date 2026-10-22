# Production readiness

## Short answer

ChasteDesk is ready for a careful, single-organization pilot on the widget and REST channels, run by a small support team that can operate TLS, backups, and a worker. It is not yet a drop-in replacement for an omni-channel enterprise helpdesk. Email ingestion, transactional email, SSO, two-factor authentication, and SLA automation are still missing.

This page is deliberately candid. Use it to decide whether the current build fits a deployment, not to infer a certification.

## Ready now

- Single-organization, self-hosted deployment with a Go web process, worker, and Postgres 16 plus pgvector.
- Cookie sessions, role checks on console routes, same-origin protection for unsafe requests, rate limits on login, widget, and API-key traffic, and append-only audit events.
- AES-256-GCM provider-key storage, masked key responses, log redaction, SSRF-guarded ingestion, and a security header baseline.
- Cited AI answers, policy-controlled tools, approval decisions, handoff packages, and honest escalation when no provider or evidence is available.
- OpenAI-compatible model routing, including DeepSeek, with wire-safe tool names. A live DeepSeek `deepseek-flash` smoke test produced a cited answer during this release.
- Optional hybrid retrieval with local or hosted embeddings. A local Ollama `nomic-embed-text` run indexed 768-dimension vectors and produced a cited hybrid answer. The requested `embeddinggemma-2:270m-bf16-text` model requires a newer Ollama than the installed 0.32.5 release.
- Ticket operations: human ticket numbers, priority, tags, snooze, close and reopen, filters, sort by priority, bulk actions, assignment, internal notes, and customer context.
- Shared quick replies with create, edit, delete, and composer insertion.
- Owner-only member role changes, member removal, and session revocation, with last-owner protection and audit events.
- Optional TOTP two-factor authentication with encrypted secrets, one-time recovery codes, challenge-based sign-in, and session revocation on enable or disable.
- SCIM 2.0 user provisioning with organization-scoped bearer tokens, activation invitations, role mapping, deactivation, and session revocation.
- Knowledge health, analytics, audit inspection, customer profiles, workspace settings, invitations, account security, and CSV audit export.
- Dark mode, a single-panel sign-in, a live topbar clock, responsive layouts from 320 px, and WCAG A/AA checks on primary screens.
- Health and readiness probes, Docker Compose, migrations, backup guidance, structured logs, and local-asset-only console builds.
- Prometheus metrics, request IDs in every response and request log, and signed operator webhook alerts for HTTP 5xx responses and failed jobs.
- Configurable first-response and resolution targets by priority, deadline display, breach filtering, an overview breach count, and worker-driven signed breach alerts.

## Required before exposing an instance

1. Terminate TLS at a trusted reverse proxy and set `APP_URL` to the public HTTPS origin.
2. Use a random `APP_SECRET` of at least 32 bytes, and a strong database password. Never reuse development values.
3. Run the worker alongside the web process. Without it, AI replies and knowledge ingestion do not progress.
4. Configure provider keys with the least privilege available, and rotate them on a schedule.
5. Back up Postgres, including pgvector data, and test a restore. See [Self-hosting](SELF_HOSTING.md).
6. Restrict network access to Postgres and the worker. Expose only the reverse proxy.
7. Decide a retention policy for conversations, customer records, audit events, and uploaded knowledge.
8. Review the security checklist in [Security](SECURITY.md), including proxy header trust and cookie settings.
9. Monitor the process, queue depth, database size, and failed knowledge sources. There is no bundled metrics or tracing exporter yet.
10. Tell operators that invitations are copy-link only. The product does not send email.

## Still missing for full production parity

- **Email channel and transactional email.** Inbound email, outbound replies, and emailed invitations are not implemented. Widget and REST are the supported channels.
- **SSO.** Password sessions, TOTP two-factor authentication, and SCIM user provisioning are available. OIDC and SAML are roadmap items. SCIM Groups are not implemented; role changes use the user `roles` attribute.
- **Data subject tooling.** There is no customer export, erasure workflow, or scheduled retention purge.
- **Observability.** Metrics, request correlation, and webhook alerts exist. Distributed span tracing, bundled dashboards, and alert rule templates do not.
- **Multi-organization hosting.** The data model is org-scoped, but the product is a single-organization install.
- **Push and notification preferences.** The console updates over SSE while open. There is no email or mobile push when it is closed.

## Suggested profile

| Deployment | Verdict |
| --- | --- |
| Internal pilot, one team, widget or REST, a named operator | Suitable with the checklist above. |
| Privacy-sensitive self-hosted support with a small team | Suitable if backups, TLS, retention, and worker monitoring are in place. |
| Public 24/7 support with email as the primary channel | Not yet. Use a pilot, or add an email integration first. |
| Enterprise rollout with SSO, SCIM, SLA, and compliance reporting | Not yet. Several platform features are still roadmap work. |

## Operational acceptance checks

Before a pilot, confirm:

- `/api/health` returns 200 through the proxy.
- `/api/ready` returns 200 and the database migration version is current.
- A test widget conversation receives a cited answer, or escalates with a complete handoff when no provider is configured.
- A teammate can sign in through an invitation, and an owner can revoke an unused invitation.
- A ticket can be assigned, prioritized, tagged, snoozed, closed, reopened, and found with filters and search.
- A quick reply can be created, inserted in the composer, edited, and deleted.
- A backup restores into a disposable database and the web process starts against it.
- Audit export opens in a spreadsheet tool and preserves the expected columns.
