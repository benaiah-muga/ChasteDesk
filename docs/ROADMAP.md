# Roadmap

ChasteDesk is an M1 preview. The core loop is complete and self-hostable. This is the honest list of what comes next.

## Now

- Cited answers from hybrid retrieval
- Policy-controlled agent with budgets and escalation
- Approval queue with execution verification
- Inbox, customers, knowledge, approvals, analytics, audit
- First-run setup, onboarding, team invitations, account settings
- Embeddable widget and REST channel
- Docker Compose deployment with migrations and health checks

## Next

### Communication
- Email delivery for invitations and handoffs
- Email channel for inbound support
- Notification preferences for owners and admins
- Webhook signing for integration events

### Team and access
- Two-factor authentication
- SSO via SAML and OIDC
- SCIM provisioning
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
- Retention policies and scheduled purge jobs
- Per-tenant encryption keys
- Object storage for uploads
- Metrics endpoint and tracing hooks
- Migration rollback tooling

## Later

- Automation opportunity detection from resolved conversations
- SLA tracking and breach alerts
- Customer satisfaction surveys
- Hosted offering with the same AGPL core
- Plugin API for channels and tools

## Explicit non-goals

- Training a proprietary model
- A marketing automation suite
- A general-purpose workflow builder
- Selling customer data or telemetry

Roadmap items are not commitments. They are the current direction, and they can change after operator feedback.
