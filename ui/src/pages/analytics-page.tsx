// SPDX-License-Identifier: AGPL-3.0-only
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, ArrowDownRight, BarChart3, Clock3, RefreshCw, Sparkles, TimerReset, Trophy } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { duration, percent, shortDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { ErrorState } from "@/components/error-state";
import { EmptyState } from "@/components/empty-state";
import { TrendChart } from "@/components/trend-chart";
import { statusDotClass } from "@/components/status-badge";
import { PageSkeleton } from "@/components/boot-screen";

export function AnalyticsPage() {
  const queryClient = useQueryClient();
  const [days, setDays] = useState(14);
  const [showTable, setShowTable] = useState(false);
  const query = useQuery({ queryKey: ["analytics", days], queryFn: () => api.analytics(days), refetchInterval: 60_000 });
  const rollup = useMutation({
    mutationFn: api.rollup,
    onSuccess: () => {
      toast.success("Rollup queued.", { description: "The worker is recomputing the latest daily metrics." });
      window.setTimeout(() => {
        void queryClient.invalidateQueries({ queryKey: ["analytics"] });
        void queryClient.invalidateQueries({ queryKey: ["overview"] });
      }, 2200);
    },
    onError: (error) => toast.error("The rollup could not be queued", { description: error instanceof Error ? error.message : undefined }),
  });

  if (query.isLoading) return <PageSkeleton />;
  if (query.isError || !query.data) return <ErrorState error={query.error} onRetry={() => void query.refetch()} />;
  const data = query.data;
  const metrics = data.metrics;

  const cards = [
    { label: "Total conversations", value: metrics ? String(metrics.total_conversations) : "n/a", hint: "Created on the latest rolled-up day", icon: Activity },
    { label: "AI resolution rate", value: metrics ? percent(metrics.ai_resolution_rate) : "n/a", hint: "Resolved with no human reply or approved action", icon: Sparkles },
    { label: "Escalation rate", value: metrics ? percent(metrics.escalation_rate) : "n/a", hint: "Share of conversations handed to a human", icon: ArrowDownRight },
    { label: "Median first response", value: metrics ? duration(metrics.p50_first_response_seconds) : "n/a", hint: "From customer message to first agent reply", icon: Clock3 },
    { label: "Median resolution", value: metrics ? duration(metrics.p50_resolution_seconds) : "n/a", hint: "From conversation start to close", icon: TimerReset },
  ];

  return (
    <div className="page-enter space-y-6">
      <PageHeader
        eyebrow={<>Desk / Measurement</>}
        title="Analytics"
        description="How the desk is doing, measured from the same rows the product uses. Medians are computed per UTC day, never estimated."
        meta={
          <>
            <span className="flex items-center gap-1.5"><span className="size-1.5 rounded-full bg-[var(--color-success)]" />{data.openNow} open right now</span>
            <span className="text-subtle">/</span>
            <span>Latest rollup {data.latestDate ? shortDate(data.latestDate) : "not computed"}</span>
          </>
        }
        actions={
          <>
            <div className="inline-flex h-9 items-center gap-0.5 rounded-md border border-rule bg-surface-2 p-0.5">
              {[7, 14, 30].map((range) => (
                <button key={range} type="button" onClick={() => setDays(range)} className={cn("h-8 rounded-sm px-3 text-[11.5px] font-medium transition-colors", days === range ? "bg-surface text-ink shadow-[var(--shadow-whisper)]" : "text-muted-foreground hover:text-ink")}>{range} days</button>
              ))}
            </div>
            <Button variant="secondary" onClick={() => rollup.mutate()} loading={rollup.isPending}><RefreshCw /> Recompute now</Button>
          </>
        }
      />

      {!metrics ? (
        <Card>
          <EmptyState
            icon={BarChart3}
            title="No analytics yet."
            body="The rollup job has not written a day of metrics. Queue a recompute and the desk will measure itself from the conversations already here."
            actionLabel="Compute now"
            onAction={() => rollup.mutate()}
          />
        </Card>
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
            {cards.map((card) => (
              <Card key={card.label} className="px-4 py-3.5">
                <div className="flex items-start justify-between gap-2">
                  <p className="eyebrow">{card.label}</p>
                  <card.icon className="size-3.5 text-muted-foreground" />
                </div>
                <p className="mt-2 font-display text-[34px] leading-none tracking-[-0.02em]">{card.value}</p>
                <p className="mt-1.5 text-[10.5px] leading-4 text-muted-foreground">{card.hint}</p>
              </Card>
            ))}
          </div>

          <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1.5fr)_minmax(19rem,0.8fr)]">
            <Card className="overflow-hidden">
              <CardHeader className="px-5 py-4">
                <div>
                  <h2 className="text-[15px] font-semibold">Conversations per day</h2>
                  <p className="mt-0.5 text-[12px] text-muted-foreground">A {days}-day window of the daily rollups. Hover a point for the exact count.</p>
                </div>
                <div className="flex items-center gap-2">
                  <span className="hidden rounded border border-rule bg-surface-2 px-1.5 py-0.5 font-mono text-[8.5px] uppercase tracking-[0.07em] text-subtle sm:inline">UTC days</span>
                  <Button variant="ghost" size="sm" onClick={() => setShowTable((value) => !value)}>{showTable ? "Chart" : "Table"}</Button>
                </div>
              </CardHeader>
              <div className="px-4 pb-3">
                {showTable ? (
                  <div className="max-h-[280px] overflow-auto rounded-md border border-rule">
                    <table className="w-full text-left text-[12px]">
                      <thead className="sticky top-0 bg-surface-2 font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">
                        <tr><th className="px-3 py-2">Date</th><th className="px-3 py-2 text-right">Conversations</th></tr>
                      </thead>
                      <tbody className="divide-y divide-rule">
                        {data.trend.map((point) => (
                          <tr key={point.date}><td className="px-3 py-2 text-muted-foreground">{point.date}</td><td className="px-3 py-2 text-right font-mono tabular">{point.total}</td></tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <TrendChart data={data.trend} height={250} />
                )}
              </div>
              <div className="flex items-center justify-between border-t border-rule bg-surface-2/40 px-5 py-2.5 font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">
                <span>{data.trend.length} days measured</span>
                <span>No sampling</span>
              </div>
            </Card>

            <div className="space-y-5">
              <Card className="overflow-hidden">
                <CardHeader className="px-5 py-4">
                  <div>
                    <h2 className="text-[15px] font-semibold">Live status mix</h2>
                    <p className="mt-0.5 text-[12px] text-muted-foreground">All conversations right now, independent of the daily rollup.</p>
                  </div>
                  <Trophy className="size-4 text-accent" />
                </CardHeader>
                <div className="space-y-3 px-5 pb-5">
                  {data.statusMix.map((item) => (
                    <div key={item.status}>
                      <div className="flex items-center justify-between text-[11.5px]">
                        <span className="flex items-center gap-2 text-muted-foreground"><span className={cn("size-1.5 rounded-full", statusDotClass(item.status))} />{item.label}</span>
                        <span className="font-mono text-[10px] tabular">{item.count} <span className="text-subtle">/ {item.percent}%</span></span>
                      </div>
                      <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-rule">
                        <div className={cn("h-full rounded-full", statusDotClass(item.status))} style={{ width: `${Math.max(item.percent, item.count > 0 ? 2 : 0)}%` }} />
                      </div>
                    </div>
                  ))}
                </div>
                <div className="flex items-center justify-between border-t border-rule bg-surface-2/40 px-5 py-2.5">
                  <span className="font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">Open now</span>
                  <span className="font-mono text-[12px] font-semibold tabular">{data.openNow}</span>
                </div>
              </Card>

              <Card className="p-5">
                <p className="eyebrow">Reading the numbers</p>
                <p className="mt-2 text-[12px] leading-5 text-muted-foreground">
                  AI resolution counts closes with no teammate reply and no approved action. Escalation counts conversations a human had to pick up. Both are derived from the audit trail and message rows.
                </p>
                <div className="mt-3 flex items-center gap-2 border-t border-rule pt-3 text-[10.5px] text-subtle">
                  <Clock3 className="size-3.5" /> Recomputed per UTC day by the worker
                </div>
              </Card>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
