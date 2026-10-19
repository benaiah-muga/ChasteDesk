# Changelog

All notable changes to ChasteDesk. The format follows Keep a Changelog, and versions use semantic versioning once a stable API exists.

## [Unreleased]

### Added

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

- Rebuilt every dashboard page on one design system with self-hosted fonts.
- Moved the product copy and documentation away from em dashes.
- Standardized provider defaults, widget theme fields, and audit error handling.
- Wired the worker concurrency setting from `WORKER_CONCURRENCY`.
- Added cursor filters and CSV export to the audit page.
- Reworked the widget from a global CSS prototype into an isolated Shadow DOM product surface.

### Fixed

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
- The empty static HTMX asset.

### Removed

- The stale Next.js and pnpm deployment path from the Docker and Compose files.
- The empty placeholder HTMX bundle.
- Em dashes from application copy, prompts, and documentation.

## [0.1.0] - 2026-09-01

Initial Go migration milestones: auth, tenancy, knowledge ingestion, retrieval, provider adapters, agent loop, approvals, inbox, audit, analytics, widget, and REST channel.
