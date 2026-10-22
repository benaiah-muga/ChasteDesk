# Roadmap

ChasteDesk is an M1 preview. The core loop is complete and self-hostable. This is the honest list of what comes next. See [Production readiness](PRODUCTION_READINESS.md) for the deployment profile and the remaining blockers.

## Now

- Cited answers from hybrid retrieval, with optional local or hosted embeddings and full-text fallback
- Policy-controlled agent with budgets and escalation
- Approval queue with execution verification
- Inbox, customers, knowledge, approvals, analytics, audit
- Ticket numbers, priority, tags, snooze, close and reopen, filters, bulk actions, and quick replies
- First-run setup, onboarding, team invitations, account settings
- Embeddable widget and REST channel
- Docker Compose deployment with migrations and health checks

## Production hardening sequence

These are the production gaps that remain after the ticket operations and retrieval work. The order reflects dependency and risk, not a promise of dates.

1. **Observability and System Doctor.** Metrics export, request IDs, queue health, and operator webhook alerts are implemented. The System Doctor, dashboards, and span tracing remain.
2. **Team administration.** Role changes, member deactivation, and session revocation for other users are implemented. SCIM and SSO remain.
3. **SLA policies.** Response and resolution targets, due timestamps, breach visibility, filters, and webhook alerts are implemented. Dashboard templates and historical breach reporting remain.
4. **Identity and access.** TOTP two-factor authentication and SCIM user provisioning are implemented. SCIM Groups and SSO remain.
5. **Email.** Transactional email for invitations and notifications, then an inbound email channel with signed webhook or IMAP ingestion.

## Next

### Communication
- Email delivery for invitations and handoffs
- Email channel for inbound support
- Notification preferences for owners and admins
- Webhook signing for integration events

### Ticket operations
- Historical SLA reporting and breach dashboards
- Ticket forms, custom fields, and required fields per channel
- Saved views and routing rules by priority, tag, and customer tier
- Customer merge and data subject export or erasure

### Team and access
- SCIM Groups
- SSO via SAML and OIDC
- SCIM provisioning
- Member role changes, deactivation, and session revocation
- Multiple organizations per installation with a workspace switcher
- Granular agent assignment rules

### Knowledge
- Site-wide crawling with sitemap support
- Source version diff and rollback UI
- Chunk browser and retrieval inspector
- Re-embedding when the embedding model changes
- Connectors for common help centers and document stores

### Agent quality
- Evaluation suite with golden conversations
- Retrieval regression tests
- Tool playground and dry-run mode
- Custom HTTP tools with explicit approval modes
- Draft-only mode for teams that are not ready for autonomous replies

### Operations
- OpenAPI specification and generated client examples
- Retention policies, customer export, and scheduled purge jobs
- Per-tenant encryption keys
- Object storage for uploads
- Metrics endpoint and tracing hooks
- Migration rollback tooling

## Later

- Automation opportunity detection from resolved conversations
- Customer satisfaction surveys
- Hosted offering with the same AGPL core
- Plugin API for channels and tools

## Explicit non-goals

- Training a proprietary model
- A marketing automation suite
- A general-purpose workflow builder
- Selling customer data or telemetry

Roadmap items are not commitments. They are the current direction, and they can change after operator feedback.
