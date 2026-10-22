# AGENTS.md

ChasteDesk: AI-native, self-hostable customer support platform. AGPL-3.0-only.

## Stack

- Go 1.27 single module, standard library first (`net/http`, `log/slog`, `crypto/*`).
- Postgres 16 with pgvector is the only stateful dependency.
- Primary operator console: React 19 with Vite, Tailwind CSS v4, and shadcn-style Radix primitives in `ui/`. Built assets are committed to `internal/web/static/console` and embedded in the Go binary.
- Legacy Go pages use `templ` plus HTMX under `/legacy/`. They are frozen compatibility surfaces, not the primary UI.
- Self-hosted fonts and a vendored HTMX bundle for the legacy pages.
- Dependency-free vanilla JavaScript for the embeddable widget.
- The historical Next.js code under `apps/` and `packages/` is frozen reference material. Do not modify it.
- `docs/ARCHITECTURE.md` is the current architecture. `docs/IMPLEMENTATION_PLAN.md` is a historical acceptance contract for the earlier TypeScript prototype, not the current plan.

## Commands

- `go build ./...` builds web, worker, and migrate
- `go test ./...` runs unit and integration tests; integration tests use ephemeral pgvector containers
- `go vet ./...` runs vet
- `gofmt -l .` must print nothing
- `go tool templ generate` regenerates committed `*_templ.go` files after legacy template edits
- `npm --prefix ui run typecheck` typechecks the console
- `npm --prefix ui run build` rebuilds the console into `internal/web/static/console`
- `node scripts/ui-console-check.mjs` runs the browser workflow check against a seeded app
- `node scripts/ui-responsive-check.mjs` checks 320 to 1440 px layouts
- `node scripts/ui-a11y-check.mjs` runs axe accessibility checks
- `go run ./cmd/web` runs the web server in development
- `go run ./cmd/worker` runs the job worker
- `go run ./cmd/migrate` applies migrations once

## Product surfaces

- First-run `/setup`, login, onboarding, account, workspace, and invitation acceptance.
- React console for overview, inbox, approvals, customers, quick replies, knowledge, analytics, audit, settings, support agent policy, onboarding, account, and invitations.
- Ticket operations: human ticket numbers, priority, tags, snooze, SLA policies and breach state, close and reopen, bulk actions, filters, and organization-scoped quick replies.
- Optional TOTP two-factor authentication with recovery codes, challenge-based sign-in, and account security controls.
- Legacy templ pages at `/legacy/...` for compatibility tests and migration fallback.
- Widget API with token and origin checks, REST channel with `cd_` keys, dashboard SSE, and health probes.
- Compose deployment through `Dockerfile.web`, `Dockerfile.worker`, and `docker-compose.yml`.

## Conventions

- Every source file starts with `// SPDX-License-Identifier: AGPL-3.0-only`.
- No comments in code unless genuinely necessary.
- No em dashes anywhere in the project. Use commas, colons, parentheses, or hyphens.
- Keep a CHANGELOG.md under [Unreleased] for every user-visible or behavioral change, Added/Changed/Fixed/Removed. Run focused browser checks for the changed flow, then run the verification gates above.
- Validate at every API and job boundary. Do not use `any` where a concrete or generic type works.
- Every domain query is org-scoped: repository functions take `db.RepoCtx`; never accept `orgId` from client bodies.
- DB: snake_case columns; code: camelCase; timestamps `timestamptz`; UUID primary keys via `gen_random_uuid()`.
- Errors: `AppError { code, message, httpStatus }`. API envelope `{ data }` or `{ error: { code, message } }`.
- Audit: every security-relevant action writes an `audit_events` row. The table is append-only and rejects `UPDATE` and `DELETE`.
- Secrets: provider API keys are only stored AES-256-GCM encrypted. API responses return masked keys. The log scrubber redacts configured material.
- Runtime supply chain discipline: standard library first. Every new direct dependency must be justified in the commit message.
- HTML boolean attributes in templ use `selected?={ condition }` and `checked?={ condition }`. Never use `selected={ condition }`.
- Legacy authenticated page components wrap their content in `appShell(shell)` through the exported page function.
- Static assets are embedded at compile time. Restart the web process after changing CSS, JavaScript, fonts, HTMX, or the built console.

## Environment

Required: Go 1.27+, Docker (ephemeral test Postgres and Compose), and a Postgres 16 + pgvector instance for manual runs. Copy `.env.example` to `.env`.

`CHASTESK_INGEST_ALLOW_PRIVATE=1` is a test-only escape hatch, read only when `APP_ENV != 'production'`. Never set it in production.

## Documentation

- `README.md` is the product entry point.
- `docs/ARCHITECTURE.md` explains processes, the React console, data flow, retrieval, the agent loop, and security boundaries.
- `docs/SELF_HOSTING.md` covers deployment and backups.
- `docs/API.md` covers public and dashboard HTTP surfaces.
- `docs/SECURITY.md` contains the threat model and hardening checklist.
- `docs/PRODUCTION_READINESS.md` contains the pilot profile, pre-exposure checklist, and honest remaining gaps.
- `docs/OPERATIONS.md` is the runbook.
- `docs/DEVELOPMENT.md` covers local workflow and UI conventions.
- `design.md` is the locked UI system.
