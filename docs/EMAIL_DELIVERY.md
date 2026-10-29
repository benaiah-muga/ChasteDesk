# Production email delivery

This runbook covers SMTP outbound delivery, Postmark-compatible inbound email, and the DNS records required for deliverability. The repository can validate the application path with a local SMTP capture, but only the operator can configure the public mailbox, provider, and DNS zone.

## Outbound SMTP

Set these in the deployment environment, not in the database:

```dotenv
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USERNAME=apikey
SMTP_PASSWORD=<secret>
SMTP_FROM=support@example.com
SMTP_TLS=starttls
```

`SMTP_TLS=tls` uses implicit TLS, normally on port 465. `SMTP_TLS=none` is accepted only for a localhost relay in production. The worker delivers queued mail every 15 seconds with five bounded attempts. Check **Settings, Email** for queued, sent, and failed counts, then use **Send test email** to verify the path.

Use a dedicated sending subdomain such as `mail.example.com` when possible, so reputation and DNS changes do not affect the primary domain.

## Inbound mailbox

Set:

```dotenv
EMAIL_INBOUND_ADDRESS=support@example.com
EMAIL_INBOUND_SECRET=<32-or-more-random-bytes>
```

Create a Postmark inbound webhook pointing at:

```text
https://desk.example.com/api/email/inbound/postmark?token=<EMAIL_INBOUND_SECRET>
```

The endpoint also accepts the secret as `Authorization: Bearer <secret>` or `X-ChasteDesk-Email-Secret`. Query-string secrets can appear in proxy logs, so restrict access logs or rotate the token if the URL is exposed. The mailbox must match `EMAIL_INBOUND_ADDRESS`. The app rejects self-addressed mail, automatically submitted mail, and explicit SPF, DKIM, or DMARC failures.

Send a test message to the support address and confirm it becomes an email ticket. The AI reply and any operator reply are delivered to the captured sender. Do not enable email for public traffic until the provider's authentication and spam controls are active.

## DNS authentication

Use the exact records Postmark shows for the sending domain. Do not copy values from another provider.

1. **SPF**: publish one SPF TXT record at the sending domain. Merge the provider's `include:` value into the existing record rather than creating a second SPF record. A typical shape is `v=spf1 include:<provider-include> ~all`, but use the value Postmark supplies.
2. **DKIM**: add the DKIM TXT or CNAME records Postmark displays, including the selector and key. Enable DKIM signing for the sending stream.
3. **Return-Path**: add the provider's bounce-domain CNAME and align it with the sending domain where the provider supports it.
4. **DMARC**: start with monitoring, for example `_dmarc.example.com TXT "v=DMARC1; p=none; rua=mailto:dmarc@example.com"`. Review aggregate reports, then move to `p=quarantine` and eventually `p=reject`. Verify SPF and DKIM alignment, not just pass results.
5. **PTR**: if sending directly from your own IP, set forward and reverse DNS consistently. A provider relay is usually simpler.

Verify with DNS lookups and an external authentication checker before sending production traffic. Keep the DMARC report mailbox monitored.

## Local production-mode validation

The repository's validation uses a local SMTP capture and an authenticated Postmark-shaped webhook. It proves the application path but not public DNS, provider reputation, or deliverability. The validation performed for this release produced:

- an inbound email conversation with Message-ID deduplication and ticket threading;
- an operator reply delivered through SMTP with a `Message-ID`, `In-Reply-To`, and ticket reference;
- a transactional invitation delivered through SMTP;
- both outbox rows marked `sent`;
- successful backup, restore, and application boot against the restored database.

SPF, DKIM, and DMARC were not tested because no public domain or DNS zone was available.

## Operational checks

- Rotate `EMAIL_INBOUND_SECRET` and immediately update the webhook if the URL leaks.
- Keep the SMTP password in a secret manager, not in `.env` committed to source control.
- Watch failed outbox counts and provider bounce/complaint metrics.
- Pause email sending before a bulk migration or a sending-domain change.
