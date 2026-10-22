<div align="center">

<img src="docs/assets/chastedesk-banner.png" alt="ChasteDesk, AI customer support platform" width="100%" />

# ChasteDesk

**The self-hostable AI support desk that shows its work.**

Answers cite the passage they came from. Risky actions wait for a human. Your data stays in your Postgres.

[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](go.mod)
[![Postgres](https://img.shields.io/badge/Postgres-16%20%2B%20pgvector-336791?logo=postgresql&logoColor=white)](docker-compose.yml)
[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-1f6f5c)](LICENSE)
[![Status](https://img.shields.io/badge/status-M1%20preview-c25a32)](docs/ROADMAP.md)
[![Self-hosted](https://img.shields.io/badge/deploy-self--hosted-6b5d4f)](docs/SELF_HOSTING.md)

[Quickstart](#quickstart) · [Features](#what-is-inside) · [Architecture](#architecture) · [Docs](#documentation) · [Roadmap](docs/ROADMAP.md)

</div>

---

ChasteDesk is an AI-native customer support desk you run yourself. It connects to the models you already trust, reads the knowledge you give it, answers customers with citations, and escalates the moment a decision needs a person. There is no vendor in the middle and no silent automation.

It is built for small support teams, privacy-sensitive products, and operators who want the whole desk in their own deployment: a Go server, a worker, and Postgres.

![ChasteDesk inbox and ticket thread](docs/assets/inbox-feature.png)

## Why ChasteDesk

Most AI support tools ask you to trust a black box. ChasteDesk takes the opposite position:

- **Cited, not guessed.** Every AI answer is grounded in retrieved chunks and carries the source name and version. If it cannot cite, it escalates.
- **Approvals, not autopilot.** The policy decides which tools are autonomous, which need a human, and which are forbidden. Risky actions pause before execution.
- **Yours to host.** One Go server, one worker, Postgres with pgvector, and your own provider keys. No telemetry, no hosted control plane.

## What is inside

### AI that can show its work
- Hybrid retrieval over Postgres full-text search plus pgvector, fused with reciprocal rank fusion.
- Structure-aware chunking with heading breadcrumbs and a 500 to 800 token target.
- Citation enforcement in the agent loop: a missing or invented citation escalates the conversation.
- Knowledge scopes: public sources can be quoted to customers, internal sources can inform the agent but are never quoted.
- Works with OpenAI, Anthropic, Google AI, DeepSeek, Ollama, and any OpenAI-compatible endpoint.
- Degrades honestly with zero providers: the desk still works and routes every conversation to a human.

### A desk designed for humans
- Three-pane inbox with filters, search, customer context, approval queue, and copilot drafts that are never sent automatically.
- Ticket operations with human ticket numbers, priority, tags, snooze, close and reopen, bulk actions, priority sorting, and shared quick replies.
- Configurable SLA targets by priority, first-response and resolution deadlines, breach filters, and signed breach alerts.
- Handoff packages that explain the problem, what was checked, the evidence used, and the recommended next step.
- Global approvals page, customers directory, knowledge health, analytics, and an append-only audit log.
- Invite teammates with a one-time link and role-based access for owner, admin, and agent.
- Keyboard command palette, a visible dark-mode switch, a live topbar clock, and responsive layouts from 320 px up.

### Channels and operations
- Embeddable vanilla-JS widget with Shadow DOM isolation, streaming replies, history restore, cross-origin CORS, and themed styling.
- REST channel authenticated with `cd_` API keys, rate limited per key.
- Docker Compose deployment with health checks, migrations, and named volumes.
- Structured JSON logs, provider key redaction, SSRF-guarded ingestion, and a security header baseline.

## Quickstart

### Prerequisites

- Docker Engine 24+ with Compose v2
- A machine with 1 GB of free memory for the stack
- Optional: an API key from OpenAI, Anthropic, Google AI, or a local Ollama instance

### Run it

```bash
git clone https://github.com/benaiah-muga/ChasteDesk.git
cd ChasteDesk
cp .env.example .env

# Required: replace the development secret before exposing the app
openssl rand -base64 48
# paste the output into APP_SECRET in .env

docker compose up -d --build --wait
```

Open [http://localhost:3000](http://localhost:3000). On a fresh database the app sends you to `/setup`:

1. Create the workspace and owner account.
2. Connect an AI provider, or leave it empty to run in human-only mode.
3. Add a help page URL. The worker crawls it, chunks it, and indexes it.
4. Finish setup and open the inbox.

### Embed the widget

Copy the signed snippet from **Settings → Chat widget** after adding your site origin:

```html
<script src="https://support.example.com/widget.js"
        data-org="YOUR_ORG_ID"
        data-token="YOUR_WIDGET_TOKEN"
        defer></script>
```

The widget verifies the page's `Origin` on every request. Add the exact origin, including scheme and port.

### No provider? No problem

With no AI provider configured, every conversation is escalated with an explanatory handoff. The inbox, knowledge, audit, and customer records all continue to work. This is a supported mode, not a broken one.

## Screenshots

The README features the banner and the inbox screenshot. The full product gallery lives in [docs/SCREENSHOTS.md](docs/SCREENSHOTS.md).

## Configuration

The web process and worker share one configuration source. Compose reads `.env` automatically.

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `APP_SECRET` | yes | none | 32+ byte secret used to derive encryption and widget-signing keys. Changing it invalidates provider keys and widget tokens. |
| `APP_URL` | yes | `http://localhost:3000` | Public URL. Used for snippets, secure cookies, and same-origin checks. |
| `DATABASE_URL` | compose sets it | none | Postgres DSN. Compose builds it from `POSTGRES_PASSWORD`. |
| `PORT` | no | `3000` | Public host port in Compose, and listen port for manual runs. |
| `APP_ENV` | no | `development` | `development`, `test`, or `production`. Compose sets `production`. |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, or `error`. |
| `WORKER_CONCURRENCY` | no | `4` in Compose, `2` in code | Workers per job queue. |
| `UPLOADS_DIR` | worker | `data/uploads` | Local directory for uploaded knowledge files. |
| `EMBEDDING_PROVIDER` | no | none | Optional embedding provider, for example `ollama` or `openai-compatible`. |
| `EMBEDDING_MODEL` | with provider | none | Embedding model name. Reindex sources after changing it. |
| `EMBEDDING_BASE_URL` | no | provider default | OpenAI-compatible or Ollama base URL. |
| `METRICS_TOKEN` | no | none | Bearer token for `/metrics`. The endpoint is hidden in production when unset. |
| `ALERT_WEBHOOK_URL` | no | none | Receives HTTP 5xx and failed-job alerts. |
| `ALERT_WEBHOOK_SECRET` | no | none | HMAC-SHA256 secret for the alert webhook signature. |
| `POSTGRES_PASSWORD` | compose | `chastedesk` | Password for the bundled Postgres. Use a strong value in production. |

`CHASTESK_INGEST_ALLOW_PRIVATE=1` is a test-only escape hatch for the crawler. It is ignored when `APP_ENV=production`. Never enable it on a public deployment.

See [docs/SELF_HOSTING.md](docs/SELF_HOSTING.md) for TLS, backups, upgrades, and reverse proxy examples.

## Architecture

```mermaid
flowchart LR
  Customer[Customer] -->|widget.js or REST| Web[Go web process]
  Operator[Support operator] -->|React console| Web
  Web --> DB[(Postgres 16 + pgvector)]
  Web -->|jobs| DB
  Worker[Go worker] --> DB
  Worker -->|chat + embeddings| Providers[AI providers]
  Web -->|LISTEN/NOTIFY| Operator
```

- **Web** (`cmd/web`): HTTP server, sessions, React console, widget API, REST channel, SSE, migrations on boot.
- **Worker** (`cmd/worker`): ingestion, embedding, AI runs, action execution, analytics rollups.
- **Migrate** (`cmd/migrate`): one-shot migration runner used by Compose. The web process also migrates on boot.
- **Postgres**: the only stateful dependency. It holds domain data, the job queue, full-text indexes, vectors, sessions, rate limits, and the append-only audit log.

The operator console is React 19 with Vite, Tailwind CSS v4, and shadcn-style Radix primitives. Build output is committed and embedded in the Go binary, so self-hosting does not need Node. The legacy Go pages remain under `/legacy/` for compatibility. The widget is dependency-free vanilla JavaScript.

More detail: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Development

Requirements: Go 1.27+, Docker, and a Postgres 16 + pgvector instance.

```bash
# Optional: start a local database
docker run --rm -d --name chastedesk-pg \
  -e POSTGRES_USER=chastedesk -e POSTGRES_PASSWORD=chastedesk -e POSTGRES_DB=chastedesk \
  -p 5432:5432 pgvector/pgvector:pg16

export DATABASE_URL=postgresql://chastedesk:chastedesk@localhost:5432/chastedesk
export APP_SECRET=$(openssl rand -base64 48)
go run ./cmd/web      # dashboard and API on :3000
go run ./cmd/worker   # ingestion and AI jobs
```

```bash
go build ./...
go test ./...
go vet ./...
gofmt -l .
npm --prefix ui run typecheck
npm --prefix ui run build
```

The console source lives in `ui/`. Its production build is written to `internal/web/static/console` and embedded in the Go binary. Node 22+ is only required when changing the console. The integration tests start an ephemeral pgvector container through Docker. If Docker is unavailable those tests skip.

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for the UI system, conventions, and test layout.

## API

Create an API key in **Settings → API keys**, then open a conversation:

```bash
curl -X POST https://support.example.com/api/v1/conversations \
  -H "Authorization: Bearer cd_your_key" \
  -H "Content-Type: application/json" \
  -d '{"customer":{"email":"ada@example.com","name":"Ada","attributes":{"plan":"Scale"}},"message":"How do I export invoices?"}'
```

The response contains a conversation id. Send follow-ups to
`POST /api/v1/conversations/{id}/messages` with `{"content":"..."}`.

Full reference: [docs/API.md](docs/API.md).

## Security

- Provider keys are encrypted with AES-256-GCM using a key derived from `APP_SECRET` with HKDF. API responses return only the last four characters.
- The crawler resolves and validates every address before connecting, revalidates redirects, and blocks private ranges.
- The widget token is HMAC-signed and bound to the workspace and its allowed origins. Widget requests require a matching `Origin`.
- Sessions are `HttpOnly`, `SameSite=Lax`, and `Secure` when `APP_URL` is HTTPS.
- Unsafe HTTP methods reject cross-origin browsers. Login, setup, invitation, and API key operations are rate limited.
- Security headers include CSP, frame denial, nosniff, a strict referrer policy, and HSTS when behind TLS.
- `audit_events` is append-only at the database level with triggers that reject `UPDATE` and `DELETE`.
- Audit payloads redact secret material and truncate free-form fields.

Read [docs/SECURITY.md](docs/SECURITY.md) before exposing an instance to the internet. To report a vulnerability, use a private GitHub security advisory.

## Project layout

```text
cmd/web/                 HTTP server entrypoint
cmd/worker/              job worker entrypoint
cmd/migrate/             migration runner
internal/agent/          policy, loop, tools, approvals, handoff
internal/analytics/      daily metric rollups
internal/auth/           Argon2id, sessions, RBAC
internal/db/             repositories, schema migrations, audit
internal/httpapi/        routes, pages, JSON API, SSE, middleware
internal/ingest/         crawler, parsers, SSRF guard, indexing
internal/model/          provider adapters and secret resolution
internal/queue/          Postgres job queue
internal/retrieval/      chunker, hybrid search, RRF
internal/web/            templ views and static assets
docs/                    architecture, deployment, API, security, operations
```

The historical Next.js implementation under `apps/` and `packages/` is frozen reference material. The Go server does not read it or depend on it.

## Documentation

- [Documentation index](docs/README.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Self-hosting](docs/SELF_HOSTING.md)
- [API reference](docs/API.md)
- [Operations runbook](docs/OPERATIONS.md)
- [Security](docs/SECURITY.md)
- [Development](docs/DEVELOPMENT.md)
- [Production readiness](docs/PRODUCTION_READINESS.md)
- [Screenshot gallery](docs/SCREENSHOTS.md)
- [Design system](design.md)
- [Roadmap](docs/ROADMAP.md)
- [Changelog](CHANGELOG.md)

## Status

ChasteDesk is an **M1 preview**. The core loop works end to end: setup, knowledge ingestion, cited answers, escalation, approvals, inbox, ticket operations, quick replies, analytics, audit, widget, REST, and self-host deployment. It is suitable for a controlled single-team pilot; email delivery, SSO, two-factor authentication, SLA policies, and multi-organization hosting are still on the [roadmap](docs/ROADMAP.md). Read the candid [production readiness](docs/PRODUCTION_READINESS.md) assessment before exposing an instance.

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md). The short version: keep it stdlib-first, scope every query by organization, add a test for the boundary you changed, and do not use em dashes in the codebase or documentation.

## License

AGPL-3.0-only. See [LICENSE](LICENSE).

ChasteDesk is named for the discipline it tries to keep: answers that can be checked, actions that can be stopped, and data that stays where it belongs.
