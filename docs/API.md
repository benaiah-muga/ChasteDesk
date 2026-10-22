# API reference

ChasteDesk exposes four API surfaces:

1. The console JSON API used by the React operator UI and available to trusted operators.
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

The console uses an `HttpOnly` session cookie. Create a session with the login form or:

```bash
curl -c cookies.txt -X POST https://support.example.com/api/auth/sign-in/email \
  -H "Content-Type: application/json" \
  -d '{"email":"owner@example.com","password":"..."}'
```

Relevant endpoints:

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/api/auth/sign-in/email` | Sign in and set the session cookie |
| `POST` | `/api/auth/sign-up/email` | Create a credential account. Organization membership comes from bootstrap or an invitation. |
| `POST` | `/api/auth/sign-out` | Revoke the current session |
| `POST` | `/api/auth/two-factor/verify` | Complete a two-factor sign-in challenge with an authenticator or recovery code |
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

## Console API

The React console at `/dashboard` uses this surface. It requires a session cookie and the listed capability. All responses use the normal `{ data }` envelope.

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
| `POST` | `/api/inbox/conversations/{id}/resolve` | inbox | Resolve, close, or reopen |
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
| `GET` | `/api/conversations/stream` | inbox | Console SSE stream |

### Console endpoints

These endpoints feed the React console. They aggregate data, include console-specific presentation fields, and are not a stable integration surface.

| Method | Path | Capability | Purpose |
| --- | --- | --- | --- |
| `GET` | `/api/app/session` | none | Auth state, setup state, permissions, onboarding, and live counts |
| `POST` | `/api/app/setup` | none | Create the first owner and start a session |
| `GET` | `/api/app/overview` | inbox | Desk brief, attention queue, readiness, metrics, and recent conversations |
| `GET` | `/api/app/inbox` | inbox | Filtered conversation list with counts and members. Supports `sla=breached`. |
| `POST` | `/api/app/inbox/bulk` | inbox | Resolve, close, reopen, assign, prioritize, or snooze up to 100 tickets |
| `PATCH` | `/api/app/inbox/{id}/ticket` | inbox | Update priority, tags, or snooze for one ticket |
| `GET` | `/api/app/canned-replies` | inbox | List shared quick replies |
| `POST` | `/api/app/canned-replies` | inbox | Create a quick reply |
| `PATCH` | `/api/app/canned-replies/{id}` | inbox | Update a quick reply |
| `DELETE` | `/api/app/canned-replies/{id}` | inbox | Delete a quick reply |
| `GET` | `/api/app/customers` | inbox | Customer directory with plan, status, and open counts |
| `GET` | `/api/app/customers/{id}` | inbox | Customer profile and conversation history |
| `GET` | `/api/app/approvals` | inbox | Pending and decided actions with parsed payloads |
| `POST` | `/api/app/approvals/bulk` | inbox | Decide up to 100 approvals |
| `GET` | `/api/app/knowledge` | knowledge read | Sources, health counts, and write permission |
| `POST` | `/api/app/knowledge` | knowledge write | Create a URL, file, or text source |
| `PATCH` | `/api/app/knowledge/{id}` | knowledge write | Change source scope |
| `POST` | `/api/app/knowledge/{id}/reindex` | knowledge write | Queue a reindex |
| `DELETE` | `/api/app/knowledge/{id}` | knowledge write | Delete a source |
| `GET` | `/api/app/analytics?days=` | inbox | Metrics, trend, live status mix, and open count |
| `GET` | `/api/app/audit` | audit read | Filtered, cursor-paginated audit events |
| `GET` | `/api/app/audit/export` | audit read | CSV export |
| `GET` | `/api/app/search?q=` | inbox | Conversations, customers, and sources for the command palette |
| `GET` | `/api/app/settings` | settings read | Providers, agent policy, widget, keys, members, and invitations |
| `POST` | `/api/app/settings/providers` | settings write | Save a provider |
| `DELETE` | `/api/app/settings/providers/{id}` | settings write | Remove a provider |
| `POST` | `/api/app/settings/widget` | settings write | Save widget appearance, origins, and enabled state |
| `POST` | `/api/app/settings/agent` | settings write | Save agent policy and budgets |
| `POST` | `/api/app/settings/api-keys` | settings write | Create a key and return the secret once |
| `DELETE` | `/api/app/settings/api-keys/{id}` | settings write | Revoke a key |
| `POST` | `/api/app/settings/workspace` | organization admin | Rename the workspace |
| `POST` | `/api/app/settings/sla` | settings write | Save first-response and resolution targets for every priority |
| `PATCH` | `/api/app/settings/members/{id}` | organization admin | Change a member role and revoke their sessions |
| `DELETE` | `/api/app/settings/members/{id}` | organization admin | Remove a member and revoke their sessions |
| `POST` | `/api/app/settings/members/{id}/revoke-sessions` | organization admin | Revoke every session for a member |
| `POST` | `/api/app/settings/account` | inbox | Update the current profile or password |
| `POST` | `/api/app/account/two-factor/setup` | inbox | Generate an authenticator secret and otpauth URI |
| `POST` | `/api/app/account/two-factor/enable` | inbox | Confirm a TOTP code and return one-time recovery codes |
| `POST` | `/api/app/account/two-factor/disable` | inbox | Disable two-factor authentication with a password and code |
| `POST` | `/api/app/settings/invitations` | settings write | Create a single-use invitation |
| `DELETE` | `/api/app/settings/invitations/{id}` | settings write | Revoke an invitation |
| `GET` | `/api/app/account` | inbox | Current account profile |
| `POST` | `/api/app/account` | inbox | Update the current profile or password |
| `GET` | `/api/app/invitations/{id}` | none | Invitation preview for the acceptance screen |
| `POST` | `/api/app/invitations/{id}/accept` | none | Accept an invitation and start a session |

### Ticket operations

A conversation is also a ticket. Console list and detail responses include `ticketNumber`, `priority` (`low`, `normal`, `high`, or `urgent`), `tags`, `snoozedUntil`, `firstResponseDueAt`, `resolutionDueAt`, and `slaBreached`.

Update one ticket:

```bash
curl -b cookies.txt -X PATCH https://support.example.com/api/app/inbox/TICKET_ID/ticket \
  -H "Content-Type: application/json" \
  -d '{"priority":"high","tags":["billing","vip"],"snoozedUntil":"2026-10-23T09:00:00Z"}'
```

Send the fields you want to change. Use `{"clearSnooze":true}` to remove a snooze. Tags are trimmed, deduplicated case-insensitively, limited to 24 characters, and limited to eight per ticket. Snooze times must be in the future and within one year.

The bulk endpoint accepts `action` values `resolve`, `close`, `reopen`, `assign`, `priority`, and `snooze`, with the matching `priority`, `snoozedUntil`, `clearSnooze`, or `assignedUserId` field.

Quick replies are shared across the organization:

```bash
curl -b cookies.txt -X POST https://support.example.com/api/app/canned-replies \
  -H "Content-Type: application/json" \
  -d '{"title":"Billing email update","body":"You can change the invoice email in Settings, then Billing."}'
```

`PATCH` and `DELETE` use the reply `id`. Titles are unique per organization, case-insensitively.

The legacy server-rendered pages remain under `/legacy/...` for compatibility. Their form endpoints under `/dashboard/...` are not a stable public API. Use the JSON endpoints or the REST channel for integrations.

## Health

| Path | Purpose |
| --- | --- |
| `GET /api/health` | Process liveness, always `{"data":{"ok":true}}` when the server is up |
| `GET /api/ready` | Database readiness, returns 503 when Postgres is unavailable |
| `GET /metrics` | Prometheus text metrics. Requires `Authorization: Bearer <METRICS_TOKEN>` when configured; hidden in production when no token is configured. |

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
