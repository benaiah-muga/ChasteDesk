# Changelog

All notable changes to ChasteDesk. The format follows Keep a Changelog, and versions use semantic versioning once a stable API exists.

## [Unreleased]

### Added

- A production React operator console at `/dashboard`, built with Vite, Tailwind CSS v4, and shadcn-style Radix primitives, with committed assets embedded in the Go binary.
- Human-readable ticket numbers, priority, tags, snooze, close and reopen, priority sorting, and combined ticket filters.
- Shared quick replies with a management page, create/edit/delete dialogs, and composer insertion.
- Bulk ticket actions for priority, snooze, clear snooze, and close.
- A visible dark-mode switch in the sidebar and sign-in page, plus a live local clock in the topbar.
- A single-panel sign-in on warm paper, with the marketing split removed.
- A candid production readiness guide covering the suitable pilot profile and remaining platform gaps.
- Optional local or hosted embedding configuration for hybrid retrieval, with chunk and query embeddings, model-scoped vector search, full-text fallback, and retrieval mode telemetry.
- Owner-only member administration: role changes, member removal, and session revocation, with last-owner protection and audit events.
- Prometheus metrics, request correlation IDs, structured request timing logs, and signed alert webhooks for HTTP 5xx responses and failed background jobs.
- Configurable SLA policies by priority, due dates on tickets, breach filters and summaries, and worker-driven signed breach alerts.
- Optional TOTP two-factor authentication with QR setup, one-time recovery codes, challenge-based sign-in, and audited enable/disable flows.
- SCIM 2.0 user provisioning with organization-scoped bearer tokens, activation invitations, role mapping, deactivation, and automatic session revocation.
- Owner member administration, SLA page, and two-factor controls now appear in the settings and account surfaces.
- The supplied ChasteDesk brand mark is now the in-app logo, browser favicon, and Apple touch icon. The supplied banner is the repository hero.
- The README now features only the banner and the supplied inbox screenshot; the rest of the product captures live in the docs screenshot gallery.
- A prominent brown topbar clock and a fully sticky settings rail that keeps the Secrets note in place.
- OpenAI-compatible provider compatibility for DeepSeek-style APIs, including wire-safe tool names and native assistant tool-call replay. Verified with a live DeepSeek `deepseek-flash` answer and citation.
- Command palette with page actions and live search across conversations, customers, and knowledge sources, opened with Cmd/Ctrl K.
- Keyboard-first inbox triage: J/K navigation, open, reply, note, resolve, assign, copilot, search, and a shortcuts sheet.
- Inbox bulk selection with assign, resolve, undo, and per-conversation live updates over SSE.
- Conversation context panel with citations, a collapsible handoff evidence trail, inline approvals, customer history, and a mobile detail sheet.
- Guided onboarding with provider setup, source indexing, and a final readiness summary.
- Full settings console for providers, agent policy and budgets, widget appearance and origins, API keys, workspace membership, and account security.
- Customer directory with plan and open-conversation filters, search, and a profile page with conversation history.
- Knowledge console with status filters, add-source dialog, scope changes, reindex, delete confirmation, and polling while indexing.
- Analytics console with range controls, accessible trend table, interactive chart, live status mix, and recompute action.
- Audit console with search, type and actor filters, cursor pagination, payload inspection, JSON copy, and CSV export.
- Console JSON API under `/api/app/*` for session, overview, inbox, customers, approvals, knowledge, analytics, audit, search, settings, account, and invitations.
- Browser verification scripts for console workflows, responsive layouts, accessibility, screenshots, build assets, and fresh onboarding.
- Axe WCAG A and AA checks for the primary console screens.
- First-run setup page at `/setup` with owner creation and an automatic session.
- Dashboard shell with overview, sidebar navigation, command palette, theme toggle, and responsive mobile navigation.
- Approvals page, customers directory with profile pages, and customer conversation history.
- Support agent settings for purpose, scopes, tool modes, escalation keywords, threshold, model, and budgets.
- API key management with one-time secret reveal and revocation.
- Team invitations with single-use links, roles, expiry, and acceptance flow.
- Account settings with name change, password change, and other-session revocation.
- Workspace settings for organization name and member list.
- Analytics range selection, SVG trend chart, and live status mix.
- Audit filters, cursor pagination, payload inspection, and CSV export.
- Security headers, same-origin checks for unsafe requests, login page rate limits, and readiness endpoint.
- Widget CORS preflight support, history restore, cross-origin API calls, Shadow DOM styling, unread state, and mobile layout.
- Go-native Dockerfiles, Compose stack, migrate binary, `.dockerignore`, and worker concurrency configuration.
- Full documentation set, locked design system, and product screenshots.

### Changed

- The primary login, setup, onboarding, invitation, and dashboard routes now serve the React console. The legacy templ pages remain at `/legacy/...` for compatibility.
- The dashboard shell now uses an adaptive deep-umber rail, persisted collapse state, a responsive off-canvas navigation sheet, and both a visible dark-mode switch and a light/dark/system theme menu.
- Sidebar and metadata contrast now meet WCAG AA at small sizes.
- Tags, status chips, and avatars now use neutral hairlines, colored dots, and deep muted tones instead of light orange fills.
- AI, internal-note, handoff, and approval cards now use paper, surface, or cool tinted backgrounds with a thin brand or severity edge.
- Snoozed tickets leave the active queue and open counts until their snooze time, and new customer messages clear the snooze.
- The inbox context rail appears only when a ticket is open, leaving a calmer empty state on wide screens.
- Sending a reply to a resolved or closed conversation reopens it as human active and clears the old resolution timestamps.
- First-run setup now writes an authentication audit event when the owner session starts.
- Assignment now clears when `assignedUserId` is null and rejects users outside the workspace.
- The console embeds only local scripts, fonts, and styles, and uses lazy route chunks.
- Console state, settings, and account responses are served with `Cache-Control: no-store`.
- Rebuilt every dashboard page on one design system with self-hosted fonts.
- Moved the product copy and documentation away from em dashes.
- Standardized provider defaults, widget theme fields, and audit error handling.
- Wired the worker concurrency setting from `WORKER_CONCURRENCY`.
- Added cursor filters and CSV export to the audit page.
- Reworked the widget from a global CSS prototype into an isolated Shadow DOM product surface.

### Fixed

- Login and setup could redirect to the overview while onboarding was still incomplete.
- Inbox handoff evidence and customer details were not reachable on narrow screens.
- Inbox header actions squeezed the customer name on desktop and mobile.
- The old customer list showed duplicate status badges and non-functional inline copy controls.
- A scrollable action payload in the thread now accepts keyboard focus.
- The inbox thread no longer exposes a duplicate level-one heading alongside the inbox heading.
- Ticket priority, tags, snooze, and lifecycle changes now publish live updates and write audit events.
- OpenAI-compatible providers rejected dotted tool names; tool names are now sanitized on the wire and mapped back to the internal IDs.
- Multi-step tool turns sent assistant tool calls as plain JSON instead of native provider tool-call messages.
- DeepSeek thinking mode required assistant reasoning content to be replayed with tool calls; the OpenAI-compatible adapter now preserves and returns it.
- Settings section navigation let the Secrets panel scroll over the sticky links at short viewport heights.
- The admin shell and settings navigation could both appear active on unrelated routes.
- Long source names, customer names, and payloads now wrap or truncate without horizontal overflow at 320 px.
- HTMX polling losing its target wrapper after the first swap.
- Bulk-selected boolean attributes rendering as `selected="false"` and `checked="false"`.
- Sidebar and topbar menu visibility at desktop widths.
- Audit actor filtering against the enum type.
- Widget requests failing cross-origin because no CORS headers were sent.
- Widget history arriving after the SSE replay and reversing message order.
- Widget citation display when `sourceName` was missing.
- Copilot sent messages not advancing the conversation to human active.
- Action completion being marked before the audit and customer message were written.
- Failed action verification not escalating the conversation to a human.
- First response timestamps being written after the SSE reply instead of before it.
- Knowledge ingest audit rows being written after the source status update instead of in the same transaction.
- The empty static HTMX asset.

### Removed

- The primary HTMX dashboard and its screenshot script. HTMX remains for the frozen legacy pages.
- The stale Next.js and pnpm deployment path from the Docker and Compose files.
- The empty placeholder HTMX bundle.
- Em dashes from application copy, prompts, and documentation.

## [0.1.0] - 2026-09-01

Initial Go migration milestones: auth, tenancy, knowledge ingestion, retrieval, provider adapters, agent loop, approvals, inbox, audit, analytics, widget, and REST channel.
