// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import {
  ArrowRight,
  ArrowUpRight,
  BadgeCheck,
  BookOpen,
  Bot,
  Check,
  CircleAlert,
  Clock3,
  ClockAlert,
  Globe2,
  Inbox,
  Sparkles,
  TriangleAlert,
} from "lucide-react";
import { api } from "@/lib/api";
import type { AttentionItem, ConversationListItem } from "@/lib/types";
import { longDate, percent, relativeTime, duration, titleCase } from "@/lib/format";
import { PageHeader } from "@/components/page-header";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { ErrorState } from "@/components/error-state";
import { EmptyState } from "@/components/empty-state";
import { Sparkline } from "@/components/trend-chart";
import { StatusBadge, statusDotClass } from "@/components/status-badge";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { PageSkeleton } from "@/components/boot-screen";
import { avatarTone, cn } from "@/lib/utils";

function AttentionIcon({ kind }: { kind: AttentionItem["kind"] }) {
  switch (kind) {
    case "approval":
      return <BadgeCheck className="size-4" />;
    case "human":
      return <TriangleAlert className="size-4" />;
    case "source":
      return <BookOpen className="size-4" />;
    case "sla":
      return <ClockAlert className="size-4" />;
    case "provider":
      return <Bot className="size-4" />;
    default:
      return <Globe2 className="size-4" />;
  }
}

function AttentionQueue({ items }: { items: AttentionItem[] }) {
  return (
    <Card className="overflow-hidden">
      <CardHeader className="border-b border-rule px-5 py-4">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-[15px] font-semibold tracking-[-0.01em]">Needs you</h2>
            <span className="inline-flex h-5 items-center gap-1.5 rounded-full border border-rule bg-surface px-2 font-mono text-[9px] font-semibold uppercase tracking-[0.07em] text-ink-2"><span className="size-1.5 rounded-full bg-[var(--color-warn)]" />
              {items.length} queue
            </span>
          </div>
          <p className="mt-0.5 text-[12.5px] text-muted-foreground">What the AI deliberately did not do.</p>
        </div>
      </CardHeader>
      {items.length === 0 ? (
        <EmptyState
          icon={Check}
          title="The desk is clear."
          body="No approvals, handoffs, or broken knowledge sources are waiting. The agent is handling the rest."
          compact
          actionLabel="Open inbox"
          actionTo="/dashboard/inbox"
        />
      ) : (
        <div className="divide-y divide-rule">
          {items.map((item, index) => (
            <Link
              key={item.kind}
              to={item.href}
              className="group flex items-start gap-4 px-5 py-4 transition-colors hover:bg-surface-2"
            >
              <span className="mt-0.5 font-mono text-[10px] tabular text-subtle">{String(index + 1).padStart(2, "0")}</span>
              <span
                className={cn(
                  "flex size-8 shrink-0 items-center justify-center rounded-md border",
                  item.severity === "danger"
                    ? "border-[var(--color-danger-rule)] bg-[var(--color-danger-soft)] text-[var(--color-danger)]"
                    : item.severity === "warn"
                      ? "border-[var(--color-warn-rule)] bg-[var(--color-warn-soft)] text-[var(--color-warn)]"
                      : "border-[var(--color-info-rule)] bg-[var(--color-info-soft)] text-[var(--color-info)]",
                )}
              >
                <AttentionIcon kind={item.kind} />
              </span>
              <span className="min-w-0 flex-1">
                <span className="flex items-start justify-between gap-3">
                  <span className="text-[13px] font-semibold text-ink">{item.title}</span>
                  <ArrowUpRight className="mt-0.5 size-3.5 shrink-0 text-subtle transition-transform group-hover:-translate-y-0.5 group-hover:translate-x-0.5 group-hover:text-ink" />
                </span>
                <span className="mt-0.5 block text-[12px] leading-5 text-muted-foreground">{item.body}</span>
              </span>
            </Link>
          ))}
        </div>
      )}
      <div className="flex items-center justify-between border-t border-rule bg-surface-2/40 px-5 py-2.5">
        <span className="font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">Human in the loop</span>
        <Link to="/dashboard/approvals" className="flex items-center gap-1 text-[11.5px] font-semibold text-accent-ink hover:underline">
          Review approvals <ArrowRight className="size-3" />
        </Link>
      </div>
    </Card>
  );
}

function RecentRow({ item }: { item: ConversationListItem }) {
  return (
    <Link to={`/dashboard/inbox/${item.id}`} className="group grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 px-5 py-3 transition-colors hover:bg-surface-2">
      <Avatar className="size-8">
        <AvatarFallback className={avatarTone(item.customer.name)}>{item.customer.initial}</AvatarFallback>
      </Avatar>
      <span className="min-w-0">
        <span className="flex items-center gap-2">
          <span className="truncate text-[12.5px] font-semibold text-ink">{item.customer.name}</span>
          <span className={cn("size-1.5 shrink-0 rounded-full", statusDotClass(item.status))} />
          <span className="hidden shrink-0 font-mono text-[9px] uppercase tracking-[0.07em] text-subtle sm:inline">{item.channelLabel}</span>
        </span>
        <span className="mt-0.5 block truncate text-[11.5px] text-muted-foreground">{item.preview}</span>
      </span>
      <span className="flex items-center gap-3">
        <StatusBadge status={item.status} label={titleCase(item.status)} className="hidden sm:inline-flex" />
        {item.pendingApprovals > 0 ? <BadgeCheck className="size-3.5 text-[var(--color-warn)]" aria-label="Approval waiting" /> : null}
        <span className="w-16 text-right font-mono text-[9.5px] tabular text-subtle">{relativeTime(item.updatedAt)}</span>
      </span>
    </Link>
  );
}

export function OverviewPage() {
  const overview = useQuery({ queryKey: ["overview"], queryFn: api.overview, refetchInterval: 60_000 });
  if (overview.isLoading) return <PageSkeleton />;
  if (overview.isError || !overview.data) return <ErrorState error={overview.error} onRetry={() => void overview.refetch()} />;
  const data = overview.data;
  const attention = data.attention;
  const readinessDone = data.readiness.filter((item) => item.done).length;
  const totalStatus = data.statusMix.reduce((total, item) => total + item.count, 0);

  return (
    <div className="page-enter space-y-6">
      <PageHeader
        eyebrow={<><span>{longDate(data.today)}</span><span className="text-subtle">/</span><span>Desk brief</span></>}
        title={data.greeting}
        description="A live read on what the desk handled, what it needs from you, and where it is still learning."
        actions={
          <>
            <Button variant="secondary" asChild>
              <Link to="/dashboard/knowledge?add=1"><BookOpen /> Add knowledge</Link>
            </Button>
            <Button asChild>
              <Link to="/dashboard/inbox">Open inbox <ArrowRight /></Link>
            </Button>
          </>
        }
      />

      <div className="flex flex-col gap-2 border-b border-rule pb-4 text-[12px] sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-start gap-2.5">
          <span className={cn("mt-1.5 size-1.5 shrink-0 rounded-full", data.status.ok ? "bg-[var(--color-success)] live-dot" : "bg-[var(--color-warn)]")} />
          <p className="text-ink-2">{data.status.text}</p>
        </div>
        <div className="flex shrink-0 items-center gap-4 font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">
          <span className="flex items-center gap-1.5"><span className="size-1.5 rounded-full bg-[var(--color-success)]" /> Live</span>
          <span>{data.counts.openConversations} open</span>
          <span>{data.counts.readySources} sources ready</span>
        </div>
      </div>

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.3fr)_minmax(22rem,0.9fr)]">
        <div className="min-w-0 space-y-5">
          <AttentionQueue items={attention} />

          <Card className="overflow-hidden">
            <CardHeader className="border-b border-rule px-5 py-4">
              <div>
                <h2 className="text-[15px] font-semibold tracking-[-0.01em]">Recent conversations</h2>
                <p className="mt-0.5 text-[12.5px] text-muted-foreground">Newest activity across the widget and the API.</p>
              </div>
              <Link to="/dashboard/inbox" className="flex items-center gap-1 text-[11.5px] font-semibold text-accent-ink hover:underline">
                View all <ArrowRight className="size-3" />
              </Link>
            </CardHeader>
            {data.recent.length ? (
              <div className="divide-y divide-rule">
                {data.recent.map((item) => <RecentRow key={item.id} item={item} />)}
              </div>
            ) : (
              <EmptyState
                icon={Inbox}
                title="No conversations yet."
                body="The first widget or API conversation will land here. Add a knowledge source first so the agent has something to stand on."
                compact
                actionLabel="Add knowledge"
                actionTo="/dashboard/knowledge?add=1"
              />
            )}
            <div className="border-t border-rule bg-surface-2/40 px-5 py-2.5">
              <span className="flex items-center gap-1.5 font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">
                <Clock3 className="size-3" /> Agent handoffs include evidence and next steps
              </span>
            </div>
          </Card>
        </div>

        <div className="min-w-0 space-y-5">
          <Card className="overflow-hidden">
            <CardHeader className="px-5 py-4">
              <div>
                <h2 className="text-[15px] font-semibold tracking-[-0.01em]">Desk performance</h2>
                <p className="mt-0.5 text-[12.5px] text-muted-foreground">Latest rolled-up day, computed from real rows.</p>
              </div>
              <Button variant="ghost" size="icon-sm" asChild aria-label="Open analytics">
                <Link to="/dashboard/analytics"><ArrowUpRight /></Link>
              </Button>
            </CardHeader>
            <CardContent className="px-5 pb-5">
              <div className="flex items-start justify-between gap-4 border-b border-rule pb-4">
                <div>
                  <p className="eyebrow">AI resolution rate</p>
                  <div className="mt-1 flex items-end gap-2">
                    <span className="font-display text-[48px] leading-none tracking-[-0.03em]">{percent(data.metrics.aiResolutionRate)}</span>
                    <span className="mb-1 font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">No human reply</span>
                  </div>
                </div>
                <Sparkline data={data.trend} className="mt-2" />
              </div>
              <div className="grid grid-cols-2 divide-x divide-rule pt-4">
                <div className="pr-4">
                  <p className="eyebrow">First response</p>
                  <p className="mt-1 font-display text-[27px] leading-none">{duration(data.metrics.p50FirstResponseSeconds)}</p>
                  <p className="mt-1 text-[10.5px] text-muted-foreground">Median handoff</p>
                </div>
                <div className="pl-4">
                  <p className="eyebrow">Resolution</p>
                  <p className="mt-1 font-display text-[27px] leading-none">{duration(data.metrics.p50ResolutionSeconds)}</p>
                  <p className="mt-1 text-[10.5px] text-muted-foreground">Median close</p>
                </div>
              </div>
              <div className="mt-4 flex items-end justify-between gap-3 border-t border-rule pt-4">
                <div>
                  <p className="eyebrow">Latest day volume</p>
                  <p className="mt-1 font-display text-[27px] leading-none">{data.metrics.totalConversations ?? "n/a"}</p>
                </div>
                <Link to="/dashboard/analytics" className="text-[11.5px] font-semibold text-accent-ink hover:underline">Full report</Link>
              </div>
            </CardContent>
          </Card>

          <Card className="overflow-hidden">
            <CardHeader className="px-5 py-4">
              <div>
                <h2 className="text-[15px] font-semibold tracking-[-0.01em]">Agent readiness</h2>
                <p className="mt-0.5 text-[12.5px] text-muted-foreground">{readinessDone} of {data.readiness.length} checks in place.</p>
              </div>
              <span className="font-mono text-[10px] tabular text-muted-foreground">{Math.round((readinessDone / data.readiness.length) * 100)}%</span>
            </CardHeader>
            <div className="px-5">
              <div className="h-1 overflow-hidden rounded-full bg-rule">
                <div className="h-full rounded-full bg-[var(--color-success)] transition-[width] duration-500" style={{ width: `${(readinessDone / data.readiness.length) * 100}%` }} />
              </div>
            </div>
            <div className="mt-3 divide-y divide-rule px-5">
              {data.readiness.map((item) => (
                <div key={item.key} className="flex items-start gap-3 py-3">
                  <span className={cn("mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full", item.done ? "bg-[var(--color-success-soft)] text-[var(--color-success)]" : "bg-surface-3 text-muted-foreground")}>
                    {item.done ? <Check className="size-3" /> : <CircleAlert className="size-3" />}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-[12px] font-semibold">{item.title}</span>
                    <span className="mt-0.5 block text-[11px] leading-4 text-muted-foreground">{item.body}</span>
                  </span>
                  <Link to={item.href} className="mt-0.5 shrink-0 text-[10.5px] font-semibold text-accent-ink hover:underline">{item.action}</Link>
                </div>
              ))}
            </div>
          </Card>

          <Card className="overflow-hidden">
            <CardHeader className="px-5 py-4">
              <div>
                <h2 className="text-[15px] font-semibold tracking-[-0.01em]">Live status mix</h2>
                <p className="mt-0.5 text-[12.5px] text-muted-foreground">All conversations right now.</p>
              </div>
              <Sparkles className="size-4 text-accent" />
            </CardHeader>
            <div className="space-y-2.5 px-5 pb-4">
              {data.statusMix.map((item) => {
                const width = totalStatus > 0 ? Math.round((item.count / totalStatus) * 100) : 0;
                return (
                  <div key={item.status} className="grid grid-cols-[7rem_minmax(0,1fr)_1.5rem] items-center gap-2">
                    <span className="text-[11px] text-muted-foreground">{item.label}</span>
                    <span className="h-1.5 overflow-hidden rounded-full bg-rule">
                      <span className={cn("block h-full rounded-full", statusDotClass(item.status))} style={{ width: `${width}%` }} />
                    </span>
                    <span className="text-right font-mono text-[10px] tabular text-muted-foreground">{item.count}</span>
                  </div>
                );
              })}
            </div>
            <div className="flex items-center justify-between border-t border-rule bg-surface-2/40 px-5 py-2.5">
              <span className="font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">Open now</span>
              <span className="font-mono text-[11px] font-semibold tabular">{data.counts.openConversations}</span>
            </div>
          </Card>
        </div>
      </div>

      <div className="grid gap-4 border-t border-rule pt-4 sm:grid-cols-3">
        {[
          { icon: Bot, label: "AI provider", value: data.counts.enabledProviders ? `${data.counts.enabledProviders} connected` : "Not connected", href: "/dashboard/settings/providers" },
          { icon: BookOpen, label: "Knowledge", value: `${data.counts.readySources} of ${data.counts.sources} ready`, href: "/dashboard/knowledge" },
          { icon: Globe2, label: "Widget origins", value: `${data.counts.widgetOrigins} allowed`, href: "/dashboard/settings/widget" },
        ].map((item) => (
          <Link key={item.label} to={item.href} className="group flex items-center gap-3 rounded-lg border border-rule bg-surface px-4 py-3 transition-colors hover:border-rule-strong">
            <span className="flex size-8 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><item.icon className="size-4" /></span>
            <span className="min-w-0 flex-1">
              <span className="block font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">{item.label}</span>
              <span className="block truncate text-[12.5px] font-semibold">{item.value}</span>
            </span>
            <ArrowUpRight className="size-3.5 text-subtle transition-transform group-hover:-translate-y-0.5 group-hover:translate-x-0.5" />
          </Link>
        ))}
      </div>
    </div>
  );
}
