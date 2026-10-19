# Development

## Requirements

- Go 1.27 or newer
- Docker for integration tests and Compose checks
- Node.js 22+ and pnpm only for the screenshot and documentation scripts. The application does not use Node.
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

## Commands

```bash
go build ./...
go test -p 1 ./...
go test ./internal/httpapi -run TestProductSurfaces -v
go vet ./...
gofmt -l .
go tool templ generate
```

`go test -p 1 ./...` starts ephemeral pgvector containers for integration tests. If Docker is missing, those tests skip with a message. The full suite takes several minutes because it exercises real Postgres and model adapters.

On a slow machine or a machine with an existing Postgres, point the tests at a disposable server to skip container startup. Each test creates and drops its own database:

```bash
CHASTEDESK_TEST_DATABASE_URL=postgresql://chastedesk:chastedesk@localhost:5432/chastedesk   go test -p 1 ./...
```

The configured role must be allowed to create and drop databases.

## Templ workflow

Pages and components live in `internal/web/views/*.templ`. Generated `*_templ.go` files are committed. After editing a template:

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
| `internal/web/static/css/tokens.css` | Design tokens and font faces |
| `internal/web/static/css/app.css` | Layout, components, responsive rules |
| `internal/web/static/js/app.js` | Navigation, command palette, copy, composer, SSE refresh |
| `internal/web/static/js/theme.js` | Applies the stored theme before paint |
| `internal/web/static/htmx.min.js` | Vendored HTMX 2.0.7 |
| `internal/web/static/fonts/` | Self-hosted Instrument Sans, Instrument Serif, JetBrains Mono |
| `internal/widget/widget.js` | Embeddable widget bundle |

Static files are embedded at compile time. Restart the web process after changing assets. Do not add third-party scripts to the dashboard.

The widget uses a closed Shadow DOM and inline CSS, so its styles do not depend on the host page. Test it from a different origin; same-origin testing hides CORS and Origin bugs.

## Adding a page

1. Add a view model and template in `internal/web/views`.
2. Wrap authenticated pages in `appShell(shell)` through the exported page component.
3. Add a handler in `internal/httpapi` that calls `shellFor` and renders the page.
4. Register the route in `internal/httpapi/server.go`.
5. Add the page to the sidebar and command palette in `internal/web/views/layout.templ`.
6. Add an integration assertion to `internal/httpapi/product_test.go`.
7. Capture or update a screenshot if the page is user-facing.

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

## Screenshots and docs checks

```bash
node scripts/ui-screenshots.mjs
node scripts/docs-check.mjs
node scripts/tags-check.mjs
node scripts/compose-smoke.mjs
```

`ui-screenshots.mjs` expects a running demo instance:

```bash
CHASTEDESK_URL=http://127.0.0.1:3000 \
CHASTEDESK_EMAIL=owner@example.com \
CHASTEDESK_PASSWORD=your-password \
node scripts/ui-screenshots.mjs
```

It writes desktop and responsive screenshots to `docs/assets` and fails on horizontal overflow.

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
