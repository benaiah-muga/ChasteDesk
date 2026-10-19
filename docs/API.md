# API reference

ChasteDesk exposes four API surfaces:

1. The dashboard JSON API used by the server-rendered UI and available to trusted operators.
2. The REST channel for external systems, authenticated with `cd_` API keys.
3. The widget API, authenticated with an HMAC token and an allowed `Origin`.
4. Health and readiness probes.

All JSON responses use one envelope:

```json
{ "data": { } }
```

Errors use:

```json
{ "error": { "code": "not_found", "message": "conversation not found" } }
```

Common error codes are `bad_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `rate_limited`, and `internal`. HTTP status codes follow the usual semantics.

## Authentication

### Sessions

The dashboard uses an `HttpOnly` session cookie. Create a session with the login form or:

```bash
curl -c cookies.txt -X POST https://support.example.com/api/auth/sign-in/email \
  -H "Content-Type: application/json" \
  -d '{"email":"owner@example.com","password":"..."}'
```

Relevant endpoints:

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/api/auth/sign-in/email` | Sign in and set the session cookie |
| `POST` | `/api/auth/sign-up/email` | Create a user account (first-run uses `/api/bootstrap`) |
| `POST` | `/api/auth/sign-out` | Revoke the current session |
| `GET` | `/api/auth/get-session` | Return the current user and session |
| `POST` | `/api/bootstrap` | Create the first organization and owner. Returns 409 after the first call |

Unsafe cookie-authenticated requests must not carry a foreign `Origin` or `Sec-Fetch-Site: cross-site`. Browser forms use the same rule.

### API keys

Create keys in **Settings → API keys**. The plaintext key is shown once. It looks like `cd_...`, is stored as a SHA-256 hash, and is limited to 600 requests per minute.

```bash
Authorization: Bearer cd_your_key
```

## REST channel

These endpoints are intended for external systems that create and continue customer conversations.

### Create a conversation

`POST /api/v1/conversations`

```bash
curl -X POST https://support.example.com/api/v1/conversations \
  -H "Authorization: Bearer cd_your_key" \
  -H "Content-Type: application/json" \
  -d '{
    "customer": {
      "email": "ada@example.com",
      "name": "Ada Lovelace",
      "attributes": {"plan": "Scale", "region": "EU"}
    },
    "message": "How do I export invoices?"
  }'
```

Response:

```json
{ "data": { "conversationId": "..." } }
```

The message is queued for the AI. When a provider is not configured, the conversation is escalated immediately with a handoff for a human.

### Continue a conversation

`POST /api/v1/conversations/{id}/messages`

```bash
curl -X POST https://support.example.com/api/v1/conversations/CONVERSATION_ID/messages \
  -H "Authorization: Bearer cd_your_key" \
  -H "Content-Type: application/json" \
  -d '{"content":"Which invoices are included in the CSV?"}'
```

### List conversations

`GET /api/v1/conversations`

Returns the 50 most recent conversations for the key's organization:

```json
{ "data": [ { "id": "...", "status": "ai_active", "createdAt": "..." } ] }
```

There is also a legacy `PUT /api/v1/conversations` body form with `conversationId` and `content`. Prefer the path-based message endpoint.

## Widget API

The widget token is signed for one organization and a list of allowed origins. Every request must carry the token as a query parameter or `X-Widget-Token`, and the browser `Origin` must match the saved list.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/widget/config?token=...` | Theme, display name, greeting |
| `POST` | `/api/widget/conversations?token=...` | Create a conversation and short-lived conversation token |
| `POST` | `/api/widget/conversations/{id}/messages?token=...&convToken=...` | Send a customer message |
| `GET` | `/api/widget/conversations/{id}/history?token=...&convToken=...` | Load the thread |
| `GET` | `/api/widget/conversations/{id}/stream?token=...&convToken=...` | SSE stream of replies |

Create request body:

```json
{
  "visitorName": "Ada",
  "visitorEmail": "ada@example.com",
  "firstMessage": "How do I export invoices?"
}
```

CORS preflight is supported for allowed origins. A request from any other origin is rejected.

### SSE frames

The stream sends server-sent events:

- `ready`: stream is open
- `message`: a message payload with `role`, `content`, `citations`
- `action.pending`: a risky action is waiting for approval
- `ping`: heartbeat every 15 seconds

Streams last 10 minutes. EventSource reconnects automatically.

## Dashboard API

These endpoints require a session cookie and the listed capability.

| Method | Path | Capability | Purpose |
| --- | --- | --- | --- |
| `GET` | `/api/onboarding/status` | inbox | Wizard state |
| `POST` | `/api/onboarding/complete` | inbox | Mark onboarding complete |
| `GET` | `/api/settings/providers` | settings read | Masked provider list |
| `POST` | `/api/settings/providers` | settings write | Add or update a provider |
| `POST` | `/api/settings/providers/{id}/connect` | settings write | Test connectivity |
| `DELETE` | `/api/settings/providers/{id}` | settings write | Remove a provider |
| `GET` | `/api/settings/models?configId=...` | settings read | List and cache models |
| `GET` | `/api/knowledge/sources` | knowledge read | List sources |
| `POST` | `/api/knowledge/sources` | knowledge write | Create a URL, file, or text source |
| `POST` | `/api/knowledge/sources-update` | knowledge write | Update scope or name |
| `DELETE` | `/api/knowledge/sources-update` | knowledge write | Delete a source |
| `POST` | `/api/knowledge/sources-reindex` | knowledge write | Reindex a source |
| `GET` | `/api/inbox/conversations` | inbox | List conversations |
| `GET` | `/api/inbox/conversations/{id}` | inbox | Conversation with context and handoff |
| `POST` | `/api/inbox/conversations/{id}/reply` | inbox | Human reply or internal note |
| `POST` | `/api/inbox/conversations/{id}/resolve` | inbox | Resolve or reopen |
| `POST` | `/api/inbox/conversations/{id}/assign` | inbox | Assign to the current user |
| `POST` | `/api/copilot/draft` | inbox | Draft from retrieval |
| `POST` | `/api/copilot/send` | inbox | Send a reviewed draft |
| `GET` | `/api/actions?status=pending_approval` | inbox | List actions |
| `POST` | `/api/actions/decide` | inbox | Approve or reject an action |
| `GET` | `/api/analytics/summary?days=14` | inbox | Metrics and trend |
| `POST` | `/api/analytics/rollup` | inbox | Queue a rollup |
| `GET` | `/api/audit?type=&q=&limit=` | audit read | Audit rows |
| `GET` | `/api/settings/widget` | settings read | Widget config |
| `PUT` | `/api/settings/widget` | settings write | Origins and theme |
| `GET` | `/api/settings/widget-token` | settings read | Signed token and snippet |
| `GET` | `/api/conversations/stream` | inbox | Dashboard SSE stream |

The browser pages use form endpoints under `/dashboard/...`. They are not a stable public API. Use the JSON endpoints or the REST channel for integrations.

## Health

| Path | Purpose |
| --- | --- |
| `GET /api/health` | Process liveness, always `{"data":{"ok":true}}` when the server is up |
| `GET /api/ready` | Database readiness, returns 503 when Postgres is unavailable |

## Limits

| Surface | Limit |
| --- | --- |
| Login | 10 attempts per 15 minutes per IP and email |
| Setup | 10 attempts per hour per IP |
| Invitation acceptance | 20 attempts per hour per IP |
| Widget conversation creation | 30 per hour per organization |
| Widget message | 20 per minute per conversation, 60 per hour per IP |
| REST API | 600 requests per minute per key |
| Message length | 4000 characters on public channels, 8000 on human replies |
| Upload size | 10 MiB |
| JSON body | 1 MiB |

## Versioning

The API is an M1 preview. Paths under `/api/v1/` are the intended stable surface. Dashboard endpoints may change with the UI. Breaking changes are recorded in `CHANGELOG.md`.
