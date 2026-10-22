// SPDX-License-Identifier: AGPL-3.0-only
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import {
  ArrowRight,
  BadgeCheck,
  Check,
  ChevronRight,
  CircleSlash,
  Code2,
  MessageSquareText,
  NotebookPen,
  Search,
  ShieldAlert,
  Users,
  X,
} from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import type { ApprovalRow } from "@/lib/types";
import { relativeTime, titleCase } from "@/lib/format";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { Card, CardHeader } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Skeleton } from "@/components/ui/skeleton";
import { ErrorState } from "@/components/error-state";
import { EmptyState } from "@/components/empty-state";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { StatusBadge } from "@/components/status-badge";
import { CopyButton } from "@/components/copy-button";

function toolMeta(toolId: string) {
  switch (toolId) {
    case "conversations.add_note":
      return { label: "Add internal note", icon: NotebookPen, description: "Creates a private note on the conversation. The customer never sees it." };
    case "knowledge.search":
      return { label: "Search the knowledge base", icon: Search, description: "Reads the approved knowledge base and returns cited passages." };
    case "customers.lookup":
      return { label: "Look up a customer", icon: Users, description: "Reads the current customer record. It cannot cross organizations." };
    case "escalate":
      return { label: "Escalate to a human", icon: ShieldAlert, description: "Stops the AI run and builds a handoff for a teammate." };
    default:
      return { label: toolId, icon: Code2, description: "A tool call requested by the agent." };
  }
}

function argsEntries(args: unknown): Array<[string, unknown]> {
  if (args && typeof args === "object" && !Array.isArray(args)) {
    return Object.entries(args as Record<string, unknown>);
  }
  return [["args", args]];
}

function displayValue(value: unknown): string {
  if (typeof value === "string") return value;
  if (value === null || value === undefined) return "none";
  return JSON.stringify(value, null, 2);
}

function ApprovalCard({
  item,
  selected,
  onSelect,
  onDecide,
  busy,
}: {
  item: ApprovalRow;
  selected: boolean;
  onSelect: (checked: boolean) => void;
  onDecide: (decision: "approve" | "reject") => void;
  busy: boolean;
}) {
  const [showJson, setShowJson] = useState(false);
  const meta = toolMeta(item.toolId);
  const Icon = meta.icon;
  const entries = argsEntries(item.args);

  return (
    <article className={cn("border-b border-rule transition-colors last:border-b-0", selected && "bg-[var(--color-accent-soft)]/25")}>
      <div className="px-5 py-4">
        <div className="flex items-start gap-3">
          <Checkbox className="mt-1" checked={selected} onCheckedChange={(value) => onSelect(value === true)} aria-label={`Select ${meta.label} for ${item.customer.name}`} />
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div className="flex min-w-0 items-start gap-3">
                <span className="flex size-9 shrink-0 items-center justify-center rounded-md border border-[var(--color-warn-rule)] bg-[var(--color-warn-soft)] text-[var(--color-warn)]">
                  <Icon className="size-4" />
                </span>
                <div>
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="text-[13.5px] font-semibold">{meta.label}</h3>
                    <Badge variant="warn">Pending approval</Badge>
                  </div>
                  <p className="mt-0.5 text-[11.5px] leading-5 text-muted-foreground">{meta.description}</p>
                  <div className="mt-2 flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
                    <span className="font-mono text-[10px] text-subtle">{item.toolId}</span>
                    <span className="text-subtle">/</span>
                    <span>{item.customer.name}</span>
                    <span className="text-subtle">/</span>
                    <span>requested {relativeTime(item.createdAt)}</span>
                  </div>
                </div>
              </div>
              <Button variant="ghost" size="sm" onClick={() => setShowJson((value) => !value)} aria-expanded={showJson}>
                <Code2 /> {showJson ? "Hide JSON" : "View JSON"}
              </Button>
            </div>

            <div className="mt-3 rounded-lg border border-rule bg-surface-2/55 p-3.5">
              <div className="eyebrow mb-2">The agent will run</div>
              <dl className="space-y-2.5">
                {entries.map(([key, value]) => (
                  <div key={key} className="grid gap-1 sm:grid-cols-[7rem_minmax(0,1fr)] sm:gap-3">
                    <dt className="font-mono text-[9.5px] uppercase tracking-[0.07em] text-subtle">{key.replaceAll("_", " ")}</dt>
                    <dd className="text-[12px] leading-5 text-ink-2">
                      {typeof value === "string" && value.includes("\n") ? (
                        <pre className="whitespace-pre-wrap font-sans">{value}</pre>
                      ) : (
                        <span className={typeof value === "string" ? "" : "font-mono text-[11px]"}>{displayValue(value)}</span>
                      )}
                    </dd>
                  </div>
                ))}
              </dl>
              {showJson ? (
                <div className="relative mt-3 border-t border-rule pt-3">
                  <CopyButton value={JSON.stringify(item.args, null, 2)} iconOnly className="absolute right-0 top-2" />
                  <pre className="max-h-60 overflow-auto pr-9 font-mono text-[10.5px] leading-5 text-muted-foreground">{JSON.stringify(item.args, null, 2)}</pre>
                </div>
              ) : null}
            </div>

            <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <Button variant="ghost" size="sm" asChild>
                  <Link to={`/dashboard/inbox/${item.conversationId}`}><MessageSquareText /> Open conversation</Link>
                </Button>
                <CopyButton value={item.customer.email} label="Copy email" iconOnly className="border-0 bg-transparent" />
              </div>
              <div className="flex items-center gap-2">
                <Button variant="secondary" size="sm" onClick={() => onDecide("reject")} disabled={busy}><X /> Reject</Button>
                <Button size="sm" onClick={() => onDecide("approve")} loading={busy}><Check /> Approve</Button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </article>
  );
}

export function ApprovalsPage() {
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: ["approvals"], queryFn: api.approvals });
  const [tab, setTab] = useState<"pending" | "history">("pending");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [decision, setDecision] = useState<{ ids: string[]; kind: "approve" | "reject"; label: string } | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const pending = query.data?.pending ?? [];
  const history = query.data?.history ?? [];
  const allSelected = pending.length > 0 && selected.size === pending.length;

  const decide = useMutation({
    mutationFn: (payload: { ids: string[]; decision: "approve" | "reject" }) => api.decideApprovalsBulk(payload.ids, payload.decision),
    onSuccess: (result, variables) => {
      if (result.failed.length > 0) {
        toast.error(`${result.failed.length} approval${result.failed.length === 1 ? "" : "s"} could not be decided`, {
          description: result.failed[0]?.message,
        });
      }
      if (result.done.length > 0) {
        toast.success(variables.decision === "approve" ? "Approved. The agent is running it now." : "Rejected. Nothing was executed.", {
          description: `${result.done.length} ${result.done.length === 1 ? "decision" : "decisions"} recorded in the audit log.`,
        });
      }
      setSelected(new Set());
      setDecision(null);
      setBusyId(null);
      void queryClient.invalidateQueries({ queryKey: ["approvals"] });
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
      void queryClient.invalidateQueries({ queryKey: ["app-session"] });
      void queryClient.invalidateQueries({ queryKey: ["inbox"] });
    },
    onError: (error) => {
      setBusyId(null);
      toast.error("The decision could not be saved", { description: error instanceof Error ? error.message : undefined });
    },
  });

  function askDecision(ids: string[], kind: "approve" | "reject") {
    if (ids.length === 0) return;
    setDecision({
      ids,
      kind,
      label:
        ids.length === 1
          ? kind === "approve"
            ? `Approve ${toolMeta(pending.find((row) => row.id === ids[0])?.toolId ?? "").label.toLowerCase()}?`
            : "Reject this action?"
          : `${kind === "approve" ? "Approve" : "Reject"} ${ids.length} approvals?`,
    });
  }

  function confirmDecision() {
    if (!decision) return;
    if (decision.ids.length === 1) setBusyId(decision.ids[0] ?? null);
    decide.mutate({ ids: decision.ids, decision: decision.kind });
  }

  if (query.isLoading) {
    return (
      <div className="page-enter space-y-6">
        <div className="border-b border-rule pb-5"><Skeleton className="h-3 w-20" /><Skeleton className="mt-3 h-10 w-64" /><Skeleton className="mt-3 h-4 w-96" /></div>
        <Skeleton className="h-56" />
        <Skeleton className="h-40" />
      </div>
    );
  }
  if (query.isError || !query.data) return <ErrorState error={query.error} onRetry={() => void query.refetch()} />;

  return (
    <div className="page-enter space-y-6">
      <PageHeader
        eyebrow={<>Desk / Human in the loop</>}
        title="Approvals"
        description="Risky actions pause here. Approving executes the tool once, then a verification step decides whether the customer sees it as done."
        meta={
          <>
            <span className="flex items-center gap-1.5 text-[11px] text-muted-foreground"><span className="size-1.5 rounded-full bg-[var(--color-warn)]" />{pending.length} waiting</span>
            <span className="font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">Append-only record</span>
          </>
        }
        actions={
          pending.length > 0 ? (
            <Button
              variant={allSelected ? "default" : "secondary"}
              onClick={() => setSelected(allSelected ? new Set() : new Set(pending.map((row) => row.id)))}
            >
              <Check /> {allSelected ? "Clear selection" : "Select all"}
            </Button>
          ) : undefined
        }
      />

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="inline-flex h-8 items-center gap-1 rounded-md border border-rule bg-surface-2 p-0.5">
          <button type="button" onClick={() => setTab("pending")} className={cn("h-7 rounded-sm px-3 text-[12px] font-medium transition-colors", tab === "pending" ? "bg-surface text-ink shadow-[var(--shadow-whisper)]" : "text-muted-foreground hover:text-ink")}>
            Waiting <span className="ml-1 font-mono text-[10px]">{pending.length}</span>
          </button>
          <button type="button" onClick={() => setTab("history")} className={cn("h-7 rounded-sm px-3 text-[12px] font-medium transition-colors", tab === "history" ? "bg-surface text-ink shadow-[var(--shadow-whisper)]" : "text-muted-foreground hover:text-ink")}>
            History <span className="ml-1 font-mono text-[10px]">{history.length}</span>
          </button>
        </div>
        {selected.size > 0 ? (
          <div className="flex flex-wrap items-center gap-2 rounded-md border border-rule bg-surface px-2.5 py-1.5">
            <span className="font-mono text-[10px] uppercase tracking-[0.07em] text-muted-foreground">{selected.size} selected</span>
            <Button variant="secondary" size="sm" onClick={() => askDecision(Array.from(selected), "reject")} disabled={decide.isPending}><X /> Reject</Button>
            <Button size="sm" onClick={() => askDecision(Array.from(selected), "approve")} disabled={decide.isPending}><Check /> Approve</Button>
            <Button variant="ghost" size="icon-sm" onClick={() => setSelected(new Set())} aria-label="Clear selection"><X /></Button>
          </div>
        ) : null}
      </div>

      {tab === "pending" ? (
        <Card className="overflow-hidden">
          <CardHeader className="border-b border-rule px-5 py-4">
            <div className="flex items-start gap-3">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-[var(--color-warn-soft)] text-[var(--color-warn)]"><BadgeCheck className="size-4" /></span>
              <div>
                <h2 className="text-[15px] font-semibold">Waiting on a human</h2>
                <p className="mt-0.5 text-[12.5px] text-muted-foreground">Nothing here has executed yet. The payload is exactly what the agent asked to run.</p>
              </div>
            </div>
          </CardHeader>
          {pending.length ? (
            pending.map((item) => (
              <ApprovalCard
                key={item.id}
                item={item}
                selected={selected.has(item.id)}
                onSelect={(checked) =>
                  setSelected((current) => {
                    const next = new Set(current);
                    if (checked) next.add(item.id);
                    else next.delete(item.id);
                    return next;
                  })
                }
                onDecide={(kind) => askDecision([item.id], kind)}
                busy={busyId === item.id || (decide.isPending && decision?.ids.includes(item.id) === true)}
              />
            ))
          ) : (
            <EmptyState
              icon={Check}
              title="Nothing needs approval."
              body="Actions that can change customer records will appear here, with the exact payload, before they run."
              actionLabel="Open inbox"
              actionTo="/dashboard/inbox"
            />
          )}
          {pending.length > 0 ? (
            <div className="flex items-center justify-between border-t border-rule bg-surface-2/40 px-5 py-2.5">
              <span className="font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">One decision, one execution, one verification</span>
              <Link to="/dashboard/audit" className="flex items-center gap-1 text-[11px] font-medium text-muted-foreground hover:text-ink">Audit log <ChevronRight className="size-3" /></Link>
            </div>
          ) : null}
        </Card>
      ) : (
        <Card className="overflow-hidden">
          <CardHeader className="border-b border-rule px-5 py-4">
            <div>
              <h2 className="text-[15px] font-semibold">Recent decisions</h2>
              <p className="mt-0.5 text-[12.5px] text-muted-foreground">The latest decided actions, newest first.</p>
            </div>
            <span className="font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">Last {history.length}</span>
          </CardHeader>
          {history.length ? (
            <div className="divide-y divide-rule">
              {history.map((item) => {
                const meta = toolMeta(item.toolId);
                const Icon = meta.icon;
                return (
                  <Link key={item.id} to={`/dashboard/inbox/${item.conversationId}`} className="group flex items-center gap-3 px-5 py-3 transition-colors hover:bg-surface-2">
                    <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><Icon className="size-4" /></span>
                    <span className="min-w-0 flex-1">
                      <span className="flex flex-wrap items-center gap-2">
                        <span className="text-[12.5px] font-semibold">{meta.label}</span>
                        <StatusBadge status={item.status} label={titleCase(item.status)} />
                      </span>
                      <span className="mt-0.5 block truncate text-[11.5px] text-muted-foreground">
                        {item.customer.name} <span className="text-subtle">/</span> {item.decidedAt ? `decided ${relativeTime(item.decidedAt)}` : `requested ${relativeTime(item.createdAt)}`}
                      </span>
                    </span>
                    <ArrowRight className="size-3.5 text-subtle transition-transform group-hover:translate-x-0.5" />
                  </Link>
                );
              })}
            </div>
          ) : (
            <EmptyState icon={CircleSlash} title="No decisions yet." body="Approved and rejected actions will land here, with the decision time and the person who made it." />
          )}
        </Card>
      )}

      <ConfirmDialog
        open={Boolean(decision)}
        onOpenChange={(open) => { if (!open) setDecision(null); }}
        title={decision?.kind === "approve" ? "Approve and run this action?" : "Reject this action?"}
        description={
          decision?.kind === "approve"
            ? "The agent runs the tool once, then verifies the result. Approving is recorded in the audit log and cannot be undone."
            : "Nothing will execute. The agent will see the rejection and continue with a safe alternative. This is recorded in the audit log."
        }
        confirmLabel={decision?.kind === "approve" ? "Approve and run" : "Reject action"}
        destructive={decision?.kind === "reject"}
        loading={decide.isPending}
        onConfirm={confirmDecision}
      />
    </div>
  );
}
