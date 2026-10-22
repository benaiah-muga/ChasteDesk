// SPDX-License-Identifier: AGPL-3.0-only
import { useMemo, useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import {
  BookOpen,
  Bot,
  Download,
  KeyRound,
  MessageSquare,
  ScrollText,
  Search,
  Settings2,
  ShieldCheck,
  UserRound,
  X,
} from "lucide-react";
import { api } from "@/lib/api";
import type { AuditEvent } from "@/lib/types";
import { cn } from "@/lib/utils";
import { dateTime, relativeTime, titleCase } from "@/lib/format";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { ErrorState } from "@/components/error-state";
import { EmptyState } from "@/components/empty-state";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { CopyButton } from "@/components/copy-button";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { PageSkeleton } from "@/components/boot-screen";

function eventIcon(type: string) {
  if (type.startsWith("auth")) return UserRound;
  if (type.startsWith("settings") || type.startsWith("agent")) return Settings2;
  if (type.startsWith("knowledge")) return BookOpen;
  if (type.startsWith("run") || type.includes("agent")) return Bot;
  if (type.startsWith("action")) return ShieldCheck;
  if (type.startsWith("apikey")) return KeyRound;
  if (type.startsWith("conversation") || type.startsWith("message")) return MessageSquare;
  return ScrollText;
}

function actorTone(type: string): string {
  if (type === "user") return "bg-[var(--color-info-soft)] text-[var(--color-info)]";
  if (type === "agent") return "bg-[var(--color-accent-soft)] text-[var(--color-accent-ink)]";
  return "bg-surface-3 text-muted-foreground";
}

export function AuditPage() {
  const [search, setSearch] = useState("");
  const [typeFilter, setTypeFilter] = useState("");
  const [actorFilter, setActorFilter] = useState("");
  const [selected, setSelected] = useState<AuditEvent | null>(null);
  const debounced = useDebouncedValue(search, 250);
  const filters = useMemo(() => {
    const params = new URLSearchParams();
    if (debounced.trim()) params.set("q", debounced.trim());
    if (typeFilter) params.set("type", typeFilter);
    if (actorFilter) params.set("actor", actorFilter);
    return params;
  }, [actorFilter, debounced, typeFilter]);

  const query = useInfiniteQuery({
    queryKey: ["audit", filters.toString()],
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams(filters);
      if (pageParam) params.set("before", String(pageParam));
      return api.audit(params);
    },
    initialPageParam: 0,
    getNextPageParam: (last) => (last.hasMore ? last.nextBefore : undefined),
  });

  const events = query.data?.pages.flatMap((page) => page.events) ?? [];
  const types = query.data?.pages[0]?.types ?? [];
  const members = query.data?.pages[0]?.members ?? [];
  const activeFilters = Boolean(debounced.trim() || typeFilter || actorFilter);
  const exportHref = `/api/app/audit/export${filters.toString() ? `?${filters.toString()}` : ""}`;

  if (query.isLoading) return <PageSkeleton />;
  if (query.isError) return <ErrorState error={query.error} onRetry={() => void query.refetch()} />;

  return (
    <div className="page-enter space-y-6">
      <PageHeader
        eyebrow={<>Desk / Append-only</>}
        title="Audit log"
        description="Every security-relevant action, preserved in the order it happened. Rows cannot be changed or deleted, by anyone."
        meta={
          <>
            <span className="flex items-center gap-1.5"><ShieldCheck className="size-3.5 text-[var(--color-success)]" />Append-only record</span>
            <span className="text-subtle">/</span>
            <span>{events.length} events loaded</span>
            <span className="text-subtle">/</span>
            <span>{types.length} event types</span>
          </>
        }
        actions={
          <Button variant="secondary" asChild>
            <a href={exportHref} download><Download /> Export CSV</a>
          </Button>
        }
      />

      <div className="flex flex-col gap-3 border-b border-rule pb-4 lg:flex-row lg:items-center lg:justify-between">
        <div className="relative max-w-md flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-subtle" />
          <input type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search event type, target, or id..." className="h-9 w-full rounded-md border border-rule-strong bg-surface pl-9 pr-9 text-[13px] outline-none placeholder:text-subtle focus:ring-2 focus:ring-ring/20" aria-label="Search audit events" />
          {search ? <button type="button" onClick={() => setSearch("")} className="absolute right-2 top-1/2 flex size-6 -translate-y-1/2 items-center justify-center rounded text-muted-foreground hover:bg-surface-3 hover:text-ink" aria-label="Clear search"><X className="size-3.5" /></button> : null}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Select value={typeFilter || "all"} onValueChange={(value) => setTypeFilter(value === "all" ? "" : value)}>
            <SelectTrigger className="h-9 w-[190px] text-[12px]" aria-label="Filter by event type"><SelectValue placeholder="Every event type" /></SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Every event type</SelectItem>
              {types.map((type) => <SelectItem key={type} value={type}>{type}</SelectItem>)}
            </SelectContent>
          </Select>
          <Select value={actorFilter || "all"} onValueChange={(value) => setActorFilter(value === "all" ? "" : value)}>
            <SelectTrigger className="h-9 w-[170px] text-[12px]" aria-label="Filter by actor"><SelectValue placeholder="Every actor" /></SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Every actor</SelectItem>
              <SelectItem value="user">People</SelectItem>
              <SelectItem value="agent">Desk AI</SelectItem>
              <SelectItem value="system">System</SelectItem>
              {members.map((member) => <SelectItem key={member.id} value={member.id}>{member.name}</SelectItem>)}
            </SelectContent>
          </Select>
          {activeFilters ? <Button variant="ghost" size="sm" onClick={() => { setSearch(""); setTypeFilter(""); setActorFilter(""); }}><X /> Clear</Button> : null}
        </div>
      </div>

      {events.length === 0 ? (
        <Card>
          <EmptyState icon={ScrollText} title={activeFilters ? "No events match." : "The log is empty."} body={activeFilters ? "Try a wider search or clear the filters. The audit record itself has not changed." : "Security-relevant actions will appear here as teammates and the agent work."} actionLabel={activeFilters ? "Clear filters" : undefined} onAction={() => { setSearch(""); setTypeFilter(""); setActorFilter(""); }} />
        </Card>
      ) : (
        <Card className="overflow-hidden">
          <div className="hidden grid-cols-[8.5rem_8rem_minmax(0,1.3fr)_minmax(0,1fr)_5rem] gap-4 border-b border-rule bg-surface-2/50 px-5 py-2.5 font-mono text-[9px] uppercase tracking-[0.08em] text-subtle md:grid">
            <span>When</span><span>Actor</span><span>Event</span><span>Target</span><span className="text-right">Payload</span>
          </div>
          <div className="divide-y divide-rule">
            {events.map((event) => {
              const Icon = eventIcon(event.type);
              return (
                <button key={event.id} type="button" onClick={() => setSelected(event)} className="group grid w-full grid-cols-[minmax(0,1fr)_4.5rem] items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-surface-2 md:grid-cols-[8.5rem_8rem_minmax(0,1.3fr)_minmax(0,1fr)_5rem] md:gap-4 md:px-5">
                  <span className="flex items-center gap-2 font-mono text-[10px] text-muted-foreground">
                    <span className="hidden text-subtle md:inline">{dateTime(event.createdAt).split(",")[0]}</span>
                    <span className="md:hidden">{relativeTime(event.createdAt)}</span>
                  </span>
                  <span className="flex items-center gap-2">
                    <span className={cn("flex size-6 shrink-0 items-center justify-center rounded-full", actorTone(event.actorType))}><Icon className="size-3" /></span>
                    <span className="truncate text-[11px] text-ink-2">{event.actorName}</span>
                  </span>
                  <span className="min-w-0">
                    <span className="flex items-center gap-2">
                      <span className="truncate font-mono text-[10.5px] font-medium text-ink">{event.type}</span>
                      <span className="hidden shrink-0 rounded border border-rule px-1 py-0.5 font-mono text-[8px] uppercase tracking-[0.06em] text-subtle lg:inline">{event.actorType}</span>
                    </span>
                    <span className="mt-0.5 block truncate text-[10.5px] text-muted-foreground">{relativeTime(event.createdAt)}</span>
                  </span>
                  <span className="hidden min-w-0 md:block">
                    <span className="block truncate text-[11px] text-muted-foreground">{event.targetType}{event.targetId ? <span className="font-mono text-[9.5px] text-subtle"> {event.targetId}</span> : null}</span>
                  </span>
                  <span className="text-right font-mono text-[9.5px] text-subtle">{event.payload && Object.keys(event.payload as object).length > 0 ? `${Object.keys(event.payload as object).length} keys` : "empty"}</span>
                </button>
              );
            })}
          </div>
          <div className="flex items-center justify-between border-t border-rule bg-surface-2/40 px-5 py-3">
            <span className="font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">Newest first · cursor paginated</span>
            {query.hasNextPage ? <Button variant="secondary" size="sm" onClick={() => void query.fetchNextPage()} loading={query.isFetchingNextPage}>Load older events</Button> : <span className="font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">End of record</span>}
          </div>
        </Card>
      )}

      <Sheet open={Boolean(selected)} onOpenChange={(open) => { if (!open) setSelected(null); }}>
        <SheetContent className="w-[min(94vw,32rem)]">
          {selected ? (
            <>
              <div className="border-b border-rule px-5 py-5 pr-12">
                <p className="eyebrow">Audit event #{selected.id}</p>
                <SheetTitle className="mt-1 font-mono text-[16px] text-ink">{selected.type}</SheetTitle>
                <p className="mt-1 text-[12px] text-muted-foreground">{dateTime(selected.createdAt)}</p>
              </div>
              <div className="min-h-0 flex-1 overflow-y-auto px-5 py-5">
                <dl className="grid grid-cols-2 gap-3">
                  {[
                    ["Actor", selected.actorName],
                    ["Actor type", titleCase(selected.actorType)],
                    ["Actor id", selected.actorId ?? "none"],
                    ["Target type", selected.targetType],
                    ["Target id", selected.targetId ?? "none"],
                    ["Record id", String(selected.id)],
                  ].map(([label, value]) => (
                    <div key={label} className="rounded-md border border-rule bg-surface-2/55 p-2.5">
                      <dt className="font-mono text-[8.5px] uppercase tracking-[0.07em] text-subtle">{label}</dt>
                      <dd className="mt-1 truncate font-mono text-[10.5px] text-ink-2" title={value}>{value}</dd>
                    </div>
                  ))}
                </dl>
                <div className="mt-5">
                  <div className="flex items-center justify-between">
                    <p className="eyebrow">Payload</p>
                    <CopyButton value={JSON.stringify(selected.payload, null, 2)} label="Copy JSON" />
                  </div>
                  <pre className="mt-2 max-h-[46vh] overflow-auto rounded-lg border border-rule bg-surface-2/55 p-3 font-mono text-[10.5px] leading-5 text-ink-2">{JSON.stringify(selected.payload, null, 2)}</pre>
                </div>
                <div className="mt-5 flex items-start gap-2 rounded-md border border-rule bg-surface-2/55 p-3 text-[11px] leading-5 text-muted-foreground">
                  <ShieldCheck className="mt-0.5 size-3.5 shrink-0 text-[var(--color-success)]" />
                  This record is append-only. The database rejects updates and deletes.
                </div>
              </div>
            </>
          ) : null}
        </SheetContent>
      </Sheet>
    </div>
  );
}
