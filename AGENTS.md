# AGENTS.md

ChasteDesk: AI-native, self-hostable customer support platform. AGPL-3.0-only.

## Commands

- `pnpm install` — install all workspace packages
- `pnpm build` — build everything (turbo)
- `pnpm typecheck` — TypeScript strict check across the workspace
- `pnpm lint` — ESLint
- `pnpm test` — vitest (unit + integration; integration tests bootstrap their own Docker Postgres via `scripts/verify/lib/ephemeral-pg.mjs`)
- `pnpm dev:web` / `pnpm dev:worker` — run web / worker in dev
- `pnpm db:generate` / `pnpm db:migrate` — drizzle-kit generate / migrate

## Conventions

- Every source file starts with `// SPDX-License-Identifier: AGPL-3.0-only`
- TypeScript strict; `any` is banned; zod validation at every API and job boundary
- Every domain query is org-scoped: repository functions take `ctx: { orgId }`; never accept `orgId` from client bodies
- DB: snake_case columns; code: camelCase; timestamps timestamptz; UUID pk via `gen_random_uuid()`
- Errors: `AppError { code, message, httpStatus }`; API envelope `{ data }` or `{ error: { code, message } }`
- Audit: every security-relevant action writes an `audit_events` row (append-only; the app role cannot UPDATE/DELETE it)
- Secrets: provider API keys are only ever stored AES-256-GCM encrypted (packages/db/src/crypto); API responses return masked keys; log scrubber redacts
- No comments in code unless genuinely necessary

## Environment

Required: Node 22+, pnpm 11, Docker (ephemeral test Postgres + compose). Copy `.env.example` to `.env`.

## Version-accurate reference

The full Next.js 16.3.4 docs are vendored at `docs/next-16.3/` (see `docs/next-16.3/VERSION.txt`). This framework version is newer than most model training data: when touching App Router, caching (`use cache`, `cacheTag`, `cacheLife`), `proxy.ts`, route handlers, or config, read the vendored docs first and never rely on memory of older Next.js versions. `cacheComponents` is enabled; dynamic APIs are async-only.

`CHASTESK_INGEST_ALLOW_PRIVATE=1` is a test-only escape hatch read exclusively when `NODE_ENV !== 'production'`. Never set it in production.
