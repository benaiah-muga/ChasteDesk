# AGENTS.md

ChasteDesk: AI-native, self-hostable customer support platform. AGPL-3.0-only.

Stack (Go migration in progress, branch `go-migration`): single Go 1.27 module, stdlib-first (`net/http`, `log/slog`, `crypto/*`), Postgres 16 + pgvector, HTMX + templ for the dashboard, vanilla-JS widget. The Next.js code under `apps/` and `packages/` is frozen reference material during the migration; do not modify it. `docs/IMPLEMENTATION_PLAN.md` is the acceptance contract and `scripts/verify/*.mjs` are the oracles being ported to Go tests, phase by phase.

## Commands

- `go build ./...` — build web + worker
- `go test ./...` — unit + integration tests
- `go vet ./...` — vet
- `gofmt -l .` — formatting check (must print nothing)
- `go run ./cmd/web` / `go run ./cmd/worker` — run web / worker in dev

## Conventions

- Every source file starts with `// SPDX-License-Identifier: AGPL-3.0-only`
- No comments in code unless genuinely necessary
- Validate at every API and job boundary; no `any` where a concrete or generic type works
- Every domain query is org-scoped: repository functions take a context carrying `orgId`; never accept `orgId` from client bodies
- DB: snake_case columns; code: camelCase; timestamps timestamptz; UUID pk via `gen_random_uuid()`
- Errors: `AppError { code, message, httpStatus }`; API envelope `{ data }` or `{ error: { code, message } }`
- Audit: every security-relevant action writes an `audit_events` row (append-only; the app role cannot UPDATE/DELETE it)
- Secrets: provider API keys are only ever stored AES-256-GCM encrypted; API responses return masked keys; log scrubber redacts
- Runtime supply chain discipline: stdlib first; every new direct dependency must be justified in the commit message

## Environment

Required: Go 1.27+, Docker (ephemeral test Postgres + compose). Copy `.env.example` to `.env`.

`CHASTESK_INGEST_ALLOW_PRIVATE=1` is a test-only escape hatch, read only when `APP_ENV != 'production'`. Never set it in production.
