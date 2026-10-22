// SPDX-License-Identifier: AGPL-3.0-only
import { useMutation } from "@tanstack/react-query";
import { Inbox, Mail, Send, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import type { SettingsResponse } from "@/lib/types";
import { api } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CopyButton } from "@/components/copy-button";
import { SectionHeader } from "./settings-shared";

export function EmailSection({ settings }: { settings: SettingsResponse }) {
  const test = useMutation({
    mutationFn: api.sendTestEmail,
    onSuccess: (result) => toast.success("Test email sent.", { description: `Delivered to ${result.to}.` }),
    onError: (error) => toast.error("The test email could not be sent", { description: error instanceof Error ? error.message : undefined }),
  });

  return (
    <div className="space-y-5">
      <SectionHeader
        title="Email channel"
        description="Inbound customer email becomes a ticket. Operator and AI replies are delivered through SMTP. Credentials are configured by the operator, never in the browser."
        actions={<Button variant="secondary" onClick={() => test.mutate()} loading={test.isPending} disabled={!settings.email.configured}><Send /> Send test email</Button>}
      />

      <div className="grid min-w-0 gap-4 md:grid-cols-2">
        <Card className="min-w-0 p-4 sm:p-5">
          <div className="flex items-start gap-3">
            <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><Mail className="size-4" /></span>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-[13.5px] font-semibold">Outbound delivery</h3>
                <Badge variant={settings.email.configured ? "success" : "outline"}>{settings.email.configured ? "SMTP configured" : "SMTP not configured"}</Badge>
              </div>
              <p className="mt-0.5 text-[11.5px] leading-5 text-muted-foreground">Transactional invitations and ticket replies are queued in Postgres and retried by the worker.</p>
              <dl className="mt-3 space-y-2 text-[11px]">
                <div className="flex items-center justify-between gap-2"><dt className="text-muted-foreground">From</dt><dd className="min-w-0 truncate text-right font-mono text-[10px] text-ink-2">{settings.email.from || "Not set"}</dd></div>
                <div className="flex items-center justify-between gap-2"><dt className="text-muted-foreground">SMTP</dt><dd className="min-w-0 break-words text-right text-ink-2">{settings.email.configured ? "Configured by environment" : "Set SMTP_HOST and SMTP_FROM"}</dd></div>
              </dl>
            </div>
          </div>
          <div className="mt-4 grid grid-cols-3 gap-2 border-t border-rule pt-3">
            <div><p className="font-mono text-[8.5px] uppercase tracking-[0.06em] text-subtle">Queued</p><p className="mt-1 font-mono text-[16px] tabular">{settings.email.queued}</p></div>
            <div><p className="font-mono text-[8.5px] uppercase tracking-[0.06em] text-subtle">Sent</p><p className="mt-1 font-mono text-[16px] tabular">{settings.email.sent}</p></div>
            <div><p className="font-mono text-[8.5px] uppercase tracking-[0.06em] text-subtle">Failed</p><p className="mt-1 font-mono text-[16px] tabular text-[var(--color-danger)]">{settings.email.failed}</p></div>
          </div>
        </Card>

        <Card className="min-w-0 p-4 sm:p-5">
          <div className="flex items-start gap-3">
            <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><Inbox className="size-4" /></span>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-[13.5px] font-semibold">Inbound email</h3>
                <Badge variant={settings.email.inboundConfigured ? "success" : "outline"}>{settings.email.inboundConfigured ? "Webhook ready" : "Webhook disabled"}</Badge>
              </div>
              <p className="mt-0.5 text-[11.5px] leading-5 text-muted-foreground">Point a Postmark inbound webhook at this endpoint. The address must match the configured support mailbox.</p>
              <dl className="mt-3 space-y-2 text-[11px]">
                <div className="flex items-center justify-between gap-2"><dt className="text-muted-foreground">Mailbox</dt><dd className="min-w-0 truncate text-right font-mono text-[10px] text-ink-2">{settings.email.inboundAddress || "Not set"}</dd></div>
                <div className="flex min-w-0 items-center justify-between gap-2"><dt className="text-muted-foreground">Webhook</dt><dd className="flex min-w-0 items-center gap-2 rounded-md border border-rule bg-surface-2/55 px-2 py-1.5"><code className="min-w-0 flex-1 truncate font-mono text-[9.5px]">/api/email/inbound/postmark</code><CopyButton value="/api/email/inbound/postmark" label="Copy inbound path" iconOnly /></dd></div>
              </dl>
            </div>
          </div>
          <p className="mt-4 flex items-start gap-1.5 border-t border-rule pt-3 text-[10.5px] leading-5 text-muted-foreground"><ShieldCheck className="mt-0.5 size-3.5 shrink-0 text-[var(--color-success)]" />The webhook requires <code className="font-mono text-[9.5px]">EMAIL_INBOUND_SECRET</code>. Raw message bodies are never written to logs.</p>
        </Card>
      </div>

      <Card className="border-[var(--color-info-rule)] bg-[var(--color-info-soft)]/45 p-4">
        <p className="text-[11px] leading-5 text-muted-foreground">Outbound email is processed by the worker every 15 seconds with bounded retries. Configure SPF, DKIM, and DMARC for the sending domain before production use. See <span className="font-medium text-ink-2">docs/SELF_HOSTING.md</span> for the SMTP and Postmark settings.</p>
      </Card>
    </div>
  );
}
