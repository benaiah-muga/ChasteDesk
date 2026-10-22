// SPDX-License-Identifier: AGPL-3.0-only
import { useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowLeft,
  ArrowRight,
  ArrowUpRight,
  ChevronDown,
  Copy,
  Search,
  SlidersHorizontal,
  UserRound,
  Users,
  X,
} from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { avatarTone, cn } from "@/lib/utils";
import { copyText, dateTime, relativeTime, titleCase, truncate } from "@/lib/format";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { ErrorState } from "@/components/error-state";
import { EmptyState } from "@/components/empty-state";
import { StatusBadge, statusDotClass } from "@/components/status-badge";
import { CopyButton } from "@/components/copy-button";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { PageSkeleton } from "@/components/boot-screen";

export function CustomersPage() {
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
  const [sort, setSort] = useState("recent");
  const debounced = useDebouncedValue(search, 250);
  const params = useMemo(() => {
    const next = new URLSearchParams();
    if (debounced.trim()) next.set("q", debounced.trim());
    if (status !== "all") next.set("status", status);
    if (sort) next.set("sort", sort);
    return next;
  }, [debounced, sort, status]);
  const customers = useQuery({ queryKey: ["customers", params.toString()], queryFn: () => api.customers(params) });

  if (customers.isLoading) return <PageSkeleton />;
  if (customers.isError || !customers.data) return <ErrorState error={customers.error} onRetry={() => void customers.refetch()} />;
  const data = customers.data;

  return (
    <div className="page-enter space-y-6">
      <PageHeader
        eyebrow={<>Desk / Directory</>}
        title="Customers"
        description="Everyone who has ever started a conversation. Attributes arrive from the widget or API and travel with the customer, never across organizations."
        meta={
          <>
            <span>{data.customers.length} customers</span>
            <span className="text-subtle">/</span>
            <span>{data.totalOpen} open conversations</span>
          </>
        }
      />

      <div className="flex flex-col gap-3 border-b border-rule pb-4 md:flex-row md:items-center md:justify-between">
        <div className="relative max-w-md flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-subtle" />
          <input
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search by name or email"
            className="h-9 w-full rounded-md border border-rule-strong bg-surface pl-9 pr-9 text-[13px] outline-none placeholder:text-subtle focus:ring-2 focus:ring-ring/20"
            aria-label="Search customers"
          />
          {search ? <button type="button" onClick={() => setSearch("")} className="absolute right-2 top-1/2 flex size-6 -translate-y-1/2 items-center justify-center rounded text-muted-foreground hover:bg-surface-3 hover:text-ink" aria-label="Clear search"><X className="size-3.5" /></button> : null}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <div className="inline-flex h-8 items-center gap-0.5 rounded-md border border-rule bg-surface-2 p-0.5">
            {[
              { key: "all", label: "Everyone" },
              { key: "open", label: "Has open" },
            ].map((filter) => (
              <button key={filter.key} type="button" onClick={() => setStatus(filter.key)} className={cn("h-7 rounded-sm px-2.5 text-[11.5px] font-medium transition-colors", status === filter.key ? "bg-surface text-ink shadow-[var(--shadow-whisper)]" : "text-muted-foreground hover:text-ink")}>
                {filter.label}
              </button>
            ))}
          </div>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="secondary" size="sm"><SlidersHorizontal /> Sort <ChevronDown className="size-3" /></Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuLabel>Order by</DropdownMenuLabel>
              <DropdownMenuItem onClick={() => setSort("recent")}>Last seen</DropdownMenuItem>
              <DropdownMenuItem onClick={() => setSort("name")}>Name</DropdownMenuItem>
              <DropdownMenuItem onClick={() => setSort("newest")}>Newest customer</DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {data.customers.length === 0 ? (
        <Card>
          <EmptyState
            icon={Users}
            title={debounced || status !== "all" ? "No customers match." : "No customers yet."}
            body={debounced || status !== "all" ? "Try a different name, email, or filter." : "The first widget or API conversation will create a customer record automatically."}
            actionLabel={debounced || status !== "all" ? "Clear filters" : "Open the inbox"}
            onAction={() => { setSearch(""); setStatus("all"); }}
            actionTo="/dashboard/inbox"
          />
        </Card>
      ) : (
        <Card className="overflow-hidden">
          <div className="hidden grid-cols-[minmax(0,1.5fr)_9rem_7rem_6rem_8rem] gap-4 border-b border-rule bg-surface-2/50 px-5 py-2.5 font-mono text-[9px] uppercase tracking-[0.08em] text-subtle md:grid">
            <span>Customer</span><span>Plan</span><span>Conversations</span><span>Open now</span><span>Last seen</span>
          </div>
          <div className="divide-y divide-rule">
            {data.customers.map((customer) => (
              <div key={customer.id} className="group relative grid gap-3 px-4 py-3.5 transition-colors hover:bg-surface-2 md:grid-cols-[minmax(0,1.5fr)_9rem_7rem_6rem_8rem] md:items-center md:gap-4 md:px-5">
                <Link to={`/dashboard/customers/${customer.id}`} className="absolute inset-0 z-0" aria-label={`Open ${customer.name}`} />
                <div className="relative z-10 flex min-w-0 items-center gap-3 pointer-events-none">
                  <Avatar className="size-9"><AvatarFallback className={avatarTone(customer.name)}>{customer.initial}</AvatarFallback></Avatar>
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-[12.5px] font-semibold text-ink">{customer.name}</span>
                      {customer.lastStatus ? <span className={cn("size-1.5 shrink-0 rounded-full", statusDotClass(customer.lastStatus))} /> : null}
                    </div>
                    <span className="mt-0.5 block truncate text-[11px] text-muted-foreground">{customer.email}</span>
                  </div>
                </div>
                <div className="relative z-10 flex items-center gap-2 md:block pointer-events-none">
                  {customer.attributes.plan ? <span className="rounded border border-rule bg-surface-2 px-1.5 py-0.5 font-mono text-[9px] uppercase tracking-[0.06em] text-muted-foreground">{customer.attributes.plan}</span> : <span className="text-[11px] text-subtle">No plan</span>}
                </div>
                <div className="relative z-10 flex items-center gap-1.5 text-[11.5px] text-muted-foreground md:block pointer-events-none">
                  <span className="md:hidden">Conversations:</span> {customer.total}
                  {customer.lastChannel ? <span className="ml-1 text-[10px] text-subtle"> · {customer.lastChannel === "widget" ? "chat" : customer.lastChannel}</span> : null}
                </div>
                <div className="relative z-10 flex items-center gap-1.5 md:block pointer-events-none">
                  {customer.open > 0 ? <span className="inline-flex items-center gap-1.5 font-mono text-[10px] font-semibold text-[var(--color-warn)]"><span className="size-1.5 rounded-full bg-[var(--color-warn)]" />{customer.open}</span> : <span className="text-[11px] text-subtle">None</span>}
                </div>
                <div className="relative z-10 flex items-center justify-between gap-2">
                  <span className="pointer-events-none font-mono text-[10px] text-muted-foreground">{relativeTime(customer.lastSeen) || "No activity"}</span>
                  <span className="flex items-center gap-1">
                    <CopyButton value={customer.email} iconOnly className="border-0 bg-transparent md:opacity-0 md:transition-opacity md:group-hover:opacity-100" />
                    <ArrowUpRight className="pointer-events-none size-3.5 text-subtle" />
                  </span>
                </div>
              </div>
            ))}
          </div>
          <div className="flex items-center justify-between border-t border-rule bg-surface-2/40 px-5 py-2.5 font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">
            <span>{data.customers.length} records</span>
            <span>Searched on name, email, and message history</span>
          </div>
        </Card>
      )}

      {data.availablePlans.length > 0 ? (
        <div className="flex flex-wrap gap-2">
          <span className="eyebrow mr-1 self-center">Plans on this desk</span>
          {data.availablePlans.map((plan) => <span key={plan} className="rounded-full border border-rule bg-surface px-2.5 py-1 font-mono text-[9px] uppercase tracking-[0.07em] text-muted-foreground">{plan}</span>)}
        </div>
      ) : null}
    </div>
  );
}

export function CustomerDetailPage() {
  const { id = "" } = useParams();
  const customer = useQuery({ queryKey: ["customer", id], queryFn: () => api.customer(id), enabled: Boolean(id) });
  if (customer.isLoading) return <PageSkeleton />;
  if (customer.isError || !customer.data) return <ErrorState error={customer.error} onRetry={() => void customer.refetch()} />;
  const data = customer.data;
  const latest = data.conversations[0];

  return (
    <div className="page-enter space-y-6">
      <PageHeader
        eyebrow={
          <Link to="/dashboard/customers" className="flex items-center gap-1.5 hover:text-ink"><ArrowLeft className="size-3" /> Customers</Link>
        }
        title={data.customer.name || data.customer.email}
        description={data.customer.email}
        meta={
          <>
            <span className="flex items-center gap-1.5"><span className={cn("size-1.5 rounded-full", data.openCount > 0 ? "bg-[var(--color-warn)]" : "bg-[var(--color-success)]")} />{data.openCount} open</span>
            <span className="text-subtle">/</span>
            <span>{data.totalCount} conversations</span>
            <span className="text-subtle">/</span>
            <span>customer since {relativeTime(data.customer.createdAt)}</span>
          </>
        }
        actions={
          <>
            <Button variant="secondary" onClick={() => void copyText(data.customer.email).then(() => toast.success("Email copied.")).catch(() => toast.error("Could not copy email."))}><Copy /> Copy email</Button>
            {latest ? <Button asChild><Link to={`/dashboard/inbox/${latest.id}`}>Open latest <ArrowRight /></Link></Button> : null}
          </>
        }
      />

      <div className="grid gap-5 lg:grid-cols-[19rem_minmax(0,1fr)]">
        <div className="space-y-4">
          <Card className="p-5">
            <div className="flex items-center gap-3">
              <Avatar className="size-12"><AvatarFallback className={`${avatarTone(data.customer.name)} text-[13px]`}>{data.customer.initial}</AvatarFallback></Avatar>
              <div className="min-w-0">
                <p className="truncate text-[15px] font-semibold">{data.customer.name}</p>
                <p className="truncate text-[11.5px] text-muted-foreground">{data.customer.email}</p>
              </div>
            </div>
            <dl className="mt-5 space-y-3">
              {Object.entries(data.customer.attributes).map(([key, value]) => (
                <div key={key} className="flex items-start justify-between gap-3 border-t border-rule pt-2.5 first:border-t-0 first:pt-0">
                  <dt className="font-mono text-[9px] uppercase tracking-[0.07em] text-subtle">{key}</dt>
                  <dd className="max-w-[60%] text-right text-[11.5px] text-ink-2">{String(value)}</dd>
                </div>
              ))}
              <div className="flex items-center justify-between gap-3 border-t border-rule pt-2.5">
                <dt className="font-mono text-[9px] uppercase tracking-[0.07em] text-subtle">First seen</dt>
                <dd className="text-[11.5px] text-ink-2">{dateTime(data.customer.createdAt)}</dd>
              </div>
            </dl>
          </Card>
          <Card className="p-4">
            <div className="flex items-start gap-2.5">
              <span className="mt-0.5 flex size-7 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><UserRound className="size-3.5" /></span>
              <div>
                <p className="text-[12px] font-semibold">Organization scoped</p>
                <p className="mt-0.5 text-[11px] leading-5 text-muted-foreground">The AI sees only this customer's record, and never another organization's conversations.</p>
              </div>
            </div>
          </Card>
        </div>

        <Card className="overflow-hidden">
          <CardHeader className="border-b border-rule px-5 py-4">
            <div>
              <h2 className="text-[15px] font-semibold">Conversations</h2>
              <p className="mt-0.5 text-[12px] text-muted-foreground">A complete history for this customer, newest first.</p>
            </div>
            <span className="font-mono text-[9px] uppercase tracking-[0.07em] text-subtle">{data.totalCount} total</span>
          </CardHeader>
          {data.conversations.length === 0 ? (
            <EmptyState icon={Users} title="No conversations yet." body="This customer exists but has not written in." />
          ) : (
            <div className="divide-y divide-rule">
              {data.conversations.map((conversation) => (
                <Link key={conversation.id} to={`/dashboard/inbox/${conversation.id}`} className="group flex items-start gap-3 px-5 py-3.5 transition-colors hover:bg-surface-2">
                  <span className={cn("mt-1.5 size-1.5 rounded-full", statusDotClass(conversation.status))} />
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <StatusBadge status={conversation.status} label={titleCase(conversation.status)} />
                      <span className="font-mono text-[9px] uppercase tracking-[0.07em] text-subtle">{conversation.channel}</span>
                      <span className="ml-auto font-mono text-[9px] text-subtle">{relativeTime(conversation.updatedAt)}</span>
                    </div>
                    <p className="mt-1.5 text-[12px] leading-5 text-ink-2">{truncate(conversation.preview, 220) || "No messages yet."}</p>
                  </div>
                  <ArrowRight className="mt-1 size-3.5 text-subtle transition-transform group-hover:translate-x-0.5" />
                </Link>
              ))}
            </div>
          )}
        </Card>
      </div>
    </div>
  );
}
