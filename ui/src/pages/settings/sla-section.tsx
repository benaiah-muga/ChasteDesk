// SPDX-License-Identifier: AGPL-3.0-only
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Clock3, Save } from "lucide-react";
import { toast } from "sonner";
import type { SettingsResponse, SLAPolicy } from "@/lib/types";
import { api } from "@/lib/api";
import { priorityMeta } from "@/lib/tickets";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { SectionHeader } from "./settings-shared";

function minutesLabel(minutes: number): string {
  if (minutes < 60) return `${minutes} ${minutes === 1 ? "minute" : "minutes"}`;
  if (minutes % 60 === 0) return `${minutes / 60} ${minutes === 60 ? "hour" : "hours"}`;
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}

type EditablePolicy = Pick<SLAPolicy, "priority" | "firstResponseMinutes" | "resolutionMinutes" | "enabled">;

export function SLASection({ settings }: { settings: SettingsResponse }) {
  const queryClient = useQueryClient();
  const [policies, setPolicies] = useState<EditablePolicy[]>(() =>
    settings.slaPolicies.map(({ priority, firstResponseMinutes, resolutionMinutes, enabled }) => ({ priority, firstResponseMinutes, resolutionMinutes, enabled })),
  );
  const dirty = JSON.stringify(policies) !== JSON.stringify(settings.slaPolicies.map(({ priority, firstResponseMinutes, resolutionMinutes, enabled }) => ({ priority, firstResponseMinutes, resolutionMinutes, enabled })));
  const invalid = policies.some((policy) => policy.firstResponseMinutes < 1 || policy.firstResponseMinutes > 525600 || policy.resolutionMinutes < 1 || policy.resolutionMinutes > 525600);

  const save = useMutation({
    mutationFn: () => api.saveSLAPolicies(policies.map((policy) => ({ ...policy }))),
    onSuccess: () => {
      toast.success("SLA policies saved.", { description: "Open tickets now use the new response and resolution targets." });
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
      void queryClient.invalidateQueries({ queryKey: ["inbox"] });
      void queryClient.invalidateQueries({ queryKey: ["conversation"] });
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
    },
    onError: (error) => toast.error("The SLA policies could not be saved", { description: error instanceof Error ? error.message : undefined }),
  });

  function update(priority: string, patch: Partial<EditablePolicy>) {
    setPolicies((current) => current.map((policy) => policy.priority === priority ? { ...policy, ...patch } : policy));
  }

  return (
    <div className="space-y-5">
      <SectionHeader
        title="SLA policies"
        description="Set the first-response and resolution targets for each priority. Changing a policy recalculates the targets for open tickets from their original creation time."
        actions={<Button onClick={() => save.mutate()} loading={save.isPending} disabled={!dirty || invalid}><Save /> Save policies</Button>}
      />

      <Card className="overflow-hidden">
        <div className="border-b border-rule px-4 py-3.5 sm:px-5">
          <div className="flex items-center gap-2">
            <Clock3 className="size-4 text-muted-foreground" />
            <h3 className="text-[13.5px] font-semibold">Targets by priority</h3>
          </div>
          <p className="mt-1 text-[11.5px] leading-5 text-muted-foreground">First response measures the first human or AI reply. Resolution measures the time until the ticket is resolved or closed. Disabled targets are not shown as breaches.</p>
        </div>
        <div className="divide-y divide-rule">
          {policies.map((policy) => {
            const meta = priorityMeta(policy.priority);
            return (
              <div key={policy.priority} className="grid gap-4 px-4 py-4 sm:px-5 md:grid-cols-[10rem_1fr_1fr_6rem] md:items-end">
                <div className="flex items-center gap-2.5 md:pb-2.5">
                  <span className={cn("size-2 rounded-full", meta.dot)} />
                  <div>
                    <p className="text-[12.5px] font-semibold">{meta.label}</p>
                    <p className="font-mono text-[9px] uppercase tracking-[0.06em] text-subtle">{policy.enabled ? "Active" : "Disabled"}</p>
                  </div>
                </div>
                <label className="block">
                  <span className="mb-1.5 block font-mono text-[9px] uppercase tracking-[0.07em] text-muted-foreground">First response</span>
                  <div className="relative">
                    <input
                      type="number"
                      min={1}
                      max={525600}
                      value={policy.firstResponseMinutes}
                      onChange={(event) => update(policy.priority, { firstResponseMinutes: Number(event.target.value) })}
                      disabled={!policy.enabled}
                      className="h-9 w-full rounded-md border border-rule-strong bg-surface px-2.5 pr-12 font-mono text-[12px] tabular outline-none focus:ring-2 focus:ring-ring/20 disabled:opacity-50"
                      aria-label={`${meta.label} first response minutes`}
                    />
                    <span className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 font-mono text-[9px] text-subtle">min</span>
                  </div>
                  <span className="mt-1 block text-[10px] text-subtle">{minutesLabel(policy.firstResponseMinutes)}</span>
                </label>
                <label className="block">
                  <span className="mb-1.5 block font-mono text-[9px] uppercase tracking-[0.07em] text-muted-foreground">Resolution</span>
                  <div className="relative">
                    <input
                      type="number"
                      min={1}
                      max={525600}
                      value={policy.resolutionMinutes}
                      onChange={(event) => update(policy.priority, { resolutionMinutes: Number(event.target.value) })}
                      disabled={!policy.enabled}
                      className="h-9 w-full rounded-md border border-rule-strong bg-surface px-2.5 pr-12 font-mono text-[12px] tabular outline-none focus:ring-2 focus:ring-ring/20 disabled:opacity-50"
                      aria-label={`${meta.label} resolution minutes`}
                    />
                    <span className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 font-mono text-[9px] text-subtle">min</span>
                  </div>
                  <span className="mt-1 block text-[10px] text-subtle">{minutesLabel(policy.resolutionMinutes)}</span>
                </label>
                <label className="flex items-center justify-between gap-2 md:pb-2.5 md:justify-end">
                  <span className="font-mono text-[9px] uppercase tracking-[0.06em] text-muted-foreground md:hidden">Enabled</span>
                  <Switch checked={policy.enabled} onCheckedChange={(enabled) => update(policy.priority, { enabled })} aria-label={`${meta.label} SLA enabled`} />
                </label>
              </div>
            );
          })}
        </div>
        <div className="flex flex-wrap items-center justify-between gap-2 border-t border-rule bg-surface-2/45 px-4 py-2.5 text-[10.5px] text-muted-foreground sm:px-5">
          <span>Breaches are recorded for audit and sent to the configured alert webhook once per minute.</span>
          {invalid ? <span className="text-[var(--color-danger)]">Targets must be between 1 minute and 365 days.</span> : <span className="font-mono text-[9px] uppercase tracking-[0.06em] text-subtle">Changes apply to open tickets on save</span>}
        </div>
      </Card>
    </div>
  );
}
