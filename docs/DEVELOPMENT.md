# Development

## Requirements

- Go 1.27 or newer
- Docker for integration tests and Compose checks
- Node.js 22+ for the React console and browser verification scripts. The shipped Go binary does not need Node.
- Postgres 16 with pgvector for manual runs

## Local setup

```bash
docker run --rm -d --name chastedesk-pg \
  -e POSTGRES_USER=chastedesk \
  -e POSTGRES_PASSWORD=chastedesk \
  -e POSTGRES_DB=chastedesk \
  -p 5432:5432 pgvector/pgvector:pg16

export DATABASE_URL=postgresql://chastedesk:chastedesk@localhost:5432/chastedesk
export APP_SECRET=$(openssl rand -base64 48)
export APP_URL=http://localhost:3000

go run ./cmd/web
go run ./cmd/worker
```

The web process applies migrations on boot. Open `http://localhost:3000/setup` for a fresh database.

## Console workflow

The primary UI lives in `ui/` and builds to `internal/web/static/console`:

```bash
npm --prefix ui install
npm --prefix ui run typecheck
npm --prefix ui run build
go build ./...
go run ./cmd/web
```

`npm --prefix ui run dev` serves the console on port 5174 and proxies API and static asset requests to a Go server on port 3100. The production assets are committed, so a fresh checkout can build the Go binary without running Node.

OpenAI-compatible providers include DeepSeek, Ollama, and custom gateways. Tool names are sanitized to the provider wire format and mapped back to internal IDs, and native assistant tool-call messages are replayed in follow-up turns. Use `deepseek-flash` or `deepseek-v4-pro` with a base URL of `https://api.deepseek.com/v1` when testing DeepSeek.

Hybrid retrieval is enabled by setting `EMBEDDING_PROVIDER`, `EMBEDDING_MODEL`, and optionally `EMBEDDING_BASE_URL` and `EMBEDDING_API_KEY` for the worker. Local Ollama example:

```bash
EMBEDDING_PROVIDER=ollama
EMBEDDING_MODEL=embeddinggemma-2:270m-bf16-text
EMBEDDING_BASE_URL=http://localhost:11434
```

The installed Ollama must support the chosen model. Reindex each source after enabling embeddings so chunks receive vectors. The knowledge console reports `Vector search` when at least one chunk is embedded. The agent falls back to full-text retrieval if the embedding endpoint is unavailable.

The console is organized as:

| Path | Purpose |
| --- | --- |
| `ui/src/pages` | Route-level screens |
| `ui/src/features` | Feature components when a screen grows |
| `ui/src/components/ui` | shadcn-style primitives built on Radix |
| `ui/src/components` | App shell, command palette, charts, shared states |
| `ui/src/lib` | Typed API client, formatting, and API types |
| `internal/httpapi/console_*.go` | JSON endpoints and the embedded SPA shell |
| `ui/src/pages/quick-replies-page.tsx` | Shared quick reply library |
| `internal/db/migrations/0003_ticket_ops.sql` | Ticket numbers, priority, tags, snooze, and canned replies |
| `internal/web/static/console` | Committed production build |

## Commands

```bash
go build ./...
go test -p 1 ./...
go test ./internal/httpapi -run TestConsole -v
node scripts/ui-console-check.mjs
node scripts/ui-a11y-check.mjs
node scripts/ui-responsive-check.mjs
go vet ./...
gofmt -l .
npm --prefix ui run typecheck
npm --prefix ui run build
go tool templ generate   # legacy templates only
```

`go test -p 1 ./...` starts ephemeral pgvector containers for integration tests. If Docker is missing, those tests skip with a message. The full suite takes several minutes because it exercises real Postgres and model adapters.

On a slow machine or a machine with an existing Postgres, point the tests at a disposable server to skip container startup. Each test creates and drops its own database:

```bash
CHASTEDESK_TEST_DATABASE_URL=postgresql://chastedesk:chastedesk@localhost:5432/chastedesk   go test -p 1 ./...
```

The configured role must be allowed to create and drop databases.

## Legacy template workflow

The old pages at `/legacy/...` live in `internal/web/views/*.templ`. They are frozen compatibility surfaces. New product work belongs in the React console. After editing a legacy template:

```bash
go tool templ generate
go build ./...
```

The project uses templ components with implicit children for the shell:

```go
templ DashboardPage(shell ShellView, view OverviewView) {
    @appShell(shell) {
        @dashboardContent(view)
    }
}
```

Boolean HTML attributes use the templ shorthand:

```go
<option selected?={ current == "agent" }>Agent</option>
<input checked?={ policy.PublicScope }/>
```

Do not write `selected={ condition }` or `checked={ condition }`. HTML treats the attribute as present even when the value is the string `false`.

## Static assets

| Path | Purpose |
| --- | --- |
| `internal/web/static/console/` | Committed React console build and route chunks |
| `internal/web/static/css/tokens.css` | Design tokens and font faces |
| `internal/web/static/css/app.css` | Legacy page layout and components |
| `internal/web/static/js/app.js` | Legacy navigation, copy, composer, SSE refresh |
| `internal/web/static/js/theme.js` | Applies the stored theme before paint |
| `internal/web/static/htmx.min.js` | Vendored HTMX 2.0.7 for legacy pages |
| `internal/web/static/fonts/` | Self-hosted Instrument Sans, Instrument Serif, JetBrains Mono |
| `internal/widget/widget.js` | Embeddable widget bundle |

Static files are embedded at compile time. Restart the web process after changing the built console, CSS, JavaScript, or fonts. Do not add third-party scripts to the console.

The widget uses a closed Shadow DOM and inline CSS, so its styles do not depend on the host page. Test it from a different origin; same-origin testing hides CORS and Origin bugs.

## Adding a console page

1. Add the typed response in `ui/src/lib/types.ts` and the client call in `ui/src/lib/api.ts`.
2. Add the JSON handler in `internal/httpapi/console_handlers.go` or a focused `console_*.go` file.
3. Register the endpoint and any SPA route needs in `internal/httpapi/server.go`.
4. Build the page in `ui/src/pages`, then add its route in `ui/src/app.tsx`.
5. Add it to the sidebar and command palette in `ui/src/components/sidebar.tsx` and `ui/src/components/command-palette.tsx`.
6. Cover the API contract in `internal/httpapi/console_test.go` or the relevant package test.
7. Run the browser checks and capture the page with `node scripts/ui-console-screenshots.mjs`.

## Database changes

Add a new numbered migration in `internal/db/migrations`. Never edit an applied file.

- Table and column names are snake_case.
- Primary keys are UUIDs with `gen_random_uuid()` unless an external identifier requires text.
- Timestamps are `timestamptz`.
- Every domain table carries `org_id`.
- Repository functions take `db.RepoCtx` and always scope by organization.
- Audit-relevant actions write a row through `Repo.WriteAudit`.

## API and job validation

- Decode JSON with `decodeBody[T]`.
- Validate at the boundary: lengths, enums, UUID shape, URLs, and roles.
- Job payloads are validated in the handler before any database write.
- Return `AppError` values instead of raw errors from handlers.
- Never accept `orgId` from a request body. Use the session, API key, or widget token.

## Tests

| Area | Location |
| --- | --- |
| Auth, bootstrap, RBAC, rate limits | `internal/httpapi/auth_*_test.go` |
| Product pages, settings, invitations | `internal/httpapi/product_test.go` |
| Inbox, approvals, copilot | `internal/httpapi/p10_test.go` |
| Widget, REST channel | `internal/httpapi/p9_test.go` |
| Analytics and audit | `internal/httpapi/p11_test.go` |
| Agent loop, policy, tools | `internal/agent/*_test.go` |
| Retrieval and chunking | `internal/retrieval/*_test.go` |
| Ingestion and SSRF | `internal/ingest/*_test.go` |
| Crypto and migrations | `internal/db/*_test.go`, `internal/crypto/*_test.go` |

Tests use real Postgres containers and real HTTP servers. Prefer a full request through `httptest` over mocking the repository.

## Screenshots, browser checks, and docs checks

```bash
node scripts/ui-console-check.mjs
node scripts/ui-responsive-check.mjs
node scripts/ui-a11y-check.mjs
node scripts/ui-console-screenshots.mjs
node scripts/docs-check.mjs
node scripts/tags-check.mjs
node scripts/compose-smoke.mjs
```

The console scripts expect a running demo instance with a completed onboarding flow:

```bash
CHASTEDESK_URL=http://127.0.0.1:3000 \
CHASTEDESK_EMAIL=owner@example.com \
CHASTEDESK_PASSWORD=your-password \
node scripts/ui-console-check.mjs
```

`ui-console-screenshots.mjs` writes desktop and mobile screenshots to `docs/assets`. The responsive check covers 320, 375, 414, 768, 1024, and 1440 px and fails on horizontal overflow or missing navigation. The accessibility check runs axe WCAG A and AA rules on the primary screens.

For a fresh onboarding walkthrough, point a throwaway database at the Go server and run:

```bash
CHASTEDESK_FRESH_URL=http://127.0.0.1:3001 node scripts/ui-onboarding-check.mjs
```

## Style rules

- Every Go file starts with `// SPDX-License-Identifier: AGPL-3.0-only`.
- No comments unless they explain something non-obvious.
- Use concrete types. Avoid `any` outside JSON boundaries.
- Keep the standard library first. Justify every new direct dependency.
- Use `gofmt`. The build fails if formatting drifts.
- Do not use em dashes in code, docs, prompts, or UI copy. Use commas, colons, or parentheses.
- Keep UI copy plain and specific. Do not invent metrics or testimonials.

## Frozen reference code

`apps/` and `packages/` contain the earlier TypeScript and Next.js implementation. They are frozen reference material during the Go migration and are not built or deployed. Do not edit them unless a task explicitly asks for it.

`docs/IMPLEMENTATION_PLAN.md` is a historical acceptance contract for that earlier stack. Current architecture lives in `docs/ARCHITECTURE.md`.
