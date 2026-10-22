// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Bot, CircleHelp, Gauge, MessageSquareLock, NotebookPen, Search, ShieldAlert, Sparkles, Users } from "lucide-react";
import { toast } from "sonner";
import type { AgentForm, SettingsResponse } from "@/lib/types";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { SectionHeader, SaveBar } from "./settings-shared";

type FormState = {
  purpose: string;
  publicScope: boolean;
  internalScope: boolean;
  tools: Record<string, string>;
  keywords: string;
  threshold: number;
  providerId: string;
  modelId: string;
  maxSteps: string;
  maxToolCalls: string;
  maxInputTokens: string;
  maxOutputTokens: string;
};

function fromAgent(agent: AgentForm): FormState {
  const tools: Record<string, string> = {};
  for (const tool of agent.tools) tools[tool.id] = tool.mode;
  return {
    purpose: agent.purpose,
    publicScope: agent.publicScope,
    internalScope: agent.internalScope,
    tools,
    keywords: agent.keywords,
    threshold: Number(agent.threshold) || 0.35,
    providerId: agent.providerId || "auto",
    modelId: agent.modelId,
    maxSteps: agent.maxSteps,
    maxToolCalls: agent.maxToolCalls,
    maxInputTokens: agent.maxInputTokens,
    maxOutputTokens: agent.maxOutputTokens,
  };
}

const toolIcons = {
  "knowledge.search": Search,
  "customers.lookup": Users,
  "conversations.add_note": NotebookPen,
  escalate: ShieldAlert,
} as const;

function ToolModeLabel({ mode }: { mode: string }) {
  if (mode === "autonomous") return <BadgeLike tone="success">Runs on its own</BadgeLike>;
  if (mode === "approval_required") return <BadgeLike tone="warn">Needs approval</BadgeLike>;
  return <BadgeLike tone="default">Off</BadgeLike>;
}

function BadgeLike({ children, tone }: { children: React.ReactNode; tone: "success" | "warn" | "default" }) {
  const dot = tone === "success" ? "bg-[var(--color-success)]" : tone === "warn" ? "bg-[var(--color-warn)]" : "bg-[var(--color-muted)]";
  return (
    <span className="inline-flex h-5 items-center gap-1.5 rounded-full border border-rule bg-surface px-2 font-mono text-[8.5px] font-medium uppercase tracking-[0.06em] text-muted-foreground">
      <span className={cn("size-1.5 rounded-full", dot)} />
      {children}
    </span>
  );
}

export function AgentSection({ settings }: { settings: SettingsResponse }) {
  const queryClient = useQueryClient();
  const initial = useMemo(() => fromAgent(settings.agent), [settings.agent]);
  const [form, setForm] = useState<FormState>(initial);
  const [baseline, setBaseline] = useState<FormState>(initial);

  useEffect(() => {
    if (JSON.stringify(form) === JSON.stringify(baseline)) {
      setForm(initial);
      setBaseline(initial);
    }
    // Only re-seed the form when it is clean.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initial]);

  const dirty = JSON.stringify(form) !== JSON.stringify(baseline);
  const providerModels = settings.agent.providerModels.find((group) => group.providerId === form.providerId)?.models ?? [];
  const hasProvider = form.providerId !== "auto";
  const selectedToolModes = Object.values(form.tools);

  const save = useMutation({
    mutationFn: () =>
      api.saveAgent({
        purpose: form.purpose,
        publicScope: form.publicScope,
        internalScope: form.internalScope,
        tools: form.tools,
        keywords: form.keywords,
        threshold: form.threshold,
        providerId: form.providerId === "auto" ? "" : form.providerId,
        modelId: form.modelId,
        maxSteps: Number(form.maxSteps),
        maxToolCalls: Number(form.maxToolCalls),
        maxInputTokens: Number(form.maxInputTokens),
        maxOutputTokens: Number(form.maxOutputTokens),
      }),
    onSuccess: () => {
      setBaseline(form);
      toast.success("Agent policy saved.", { description: "New runs use the updated scope, tools, and budget." });
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
      void queryClient.invalidateQueries({ queryKey: ["app-session"] });
    },
    onError: (error) => toast.error("The policy could not be saved", { description: error instanceof Error ? error.message : undefined }),
  });

  function validateAndSave() {
    if (!form.purpose.trim()) return toast.error("Describe what the agent is for.");
    if (!form.publicScope && !form.internalScope) return toast.error("Enable at least one knowledge scope.");
    if (form.threshold < 0 || form.threshold > 1) return toast.error("The low-confidence threshold must be between 0 and 1.");
    save.mutate();
  }

  return (
    <div className="space-y-5">
      <SectionHeader
        title="Support agent"
        description="What the agent is for, what it may read, and where it has to stop. These rules become part of every run."
        actions={<div className="flex items-center gap-2 rounded-md border border-rule bg-surface px-2.5 py-1.5 text-[10.5px] text-muted-foreground"><span className={cn("size-1.5 rounded-full", settings.agent.configuredProviders ? "bg-[var(--color-success)]" : "bg-[var(--color-warn)]")} />{settings.agent.configuredProviders ? "Model ready" : "No provider connected"}</div>}
      />

      <Card className="p-4 sm:p-5">
        <div className="mb-4 flex items-start gap-3">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><Bot className="size-4" /></span>
          <div>
            <h3 className="text-[13.5px] font-semibold">Purpose</h3>
            <p className="mt-0.5 text-[11.5px] leading-5 text-muted-foreground">A short brief the model reads before every run. Be specific about tone and what you do not want it to guess.</p>
          </div>
        </div>
        <Textarea value={form.purpose} onChange={(event) => setForm((current) => ({ ...current, purpose: event.target.value }))} maxLength={4000} className="min-h-28" placeholder="Answer billing questions from the approved knowledge base and escalate refunds..." />
        <div className="mt-1 flex justify-between font-mono text-[9px] text-subtle"><span>Measured at save time</span><span>{form.purpose.length} / 4000</span></div>
      </Card>

      <Card className="p-4 sm:p-5">
        <div className="mb-4 flex items-start gap-3">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><Search className="size-4" /></span>
          <div>
            <h3 className="text-[13.5px] font-semibold">Knowledge access</h3>
            <p className="mt-0.5 text-[11.5px] leading-5 text-muted-foreground">Internal sources are never quoted to customers. Public sources can appear as citations.</p>
          </div>
        </div>
        <div className="grid gap-2 sm:grid-cols-2">
          {[
            { key: "public" as const, label: "Public knowledge", body: "Search public sources and quote them to customers.", checked: form.publicScope },
            { key: "internal" as const, label: "Internal knowledge", body: "Use internal source material for context, but never quote it.", checked: form.internalScope },
          ].map((scope) => (
            <label key={scope.key} className={cn("flex cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors", scope.checked ? "border-rule-strong bg-surface-2/65" : "border-rule bg-surface hover:border-rule-strong")}>
              <Checkbox checked={scope.checked} onCheckedChange={(value) => setForm((current) => ({ ...current, [scope.key === "public" ? "publicScope" : "internalScope"]: value === true }))} className="mt-0.5" aria-label={scope.label} />
              <span>
                <span className="block text-[12px] font-semibold">{scope.label}</span>
                <span className="mt-0.5 block text-[10.5px] leading-4 text-muted-foreground">{scope.body}</span>
              </span>
            </label>
          ))}
        </div>
      </Card>

      <Card className="overflow-hidden">
        <div className="border-b border-rule p-4 sm:p-5">
          <div className="flex items-start gap-3">
            <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><MessageSquareLock className="size-4" /></span>
            <div>
              <h3 className="text-[13.5px] font-semibold">Tools and approvals</h3>
              <p className="mt-0.5 text-[11.5px] leading-5 text-muted-foreground">A tool that writes to a customer record should usually wait for a human. Read-only lookups can run on their own.</p>
            </div>
          </div>
        </div>
        <div className="divide-y divide-rule">
          {settings.agent.tools.map((tool) => {
            const Icon = toolIcons[tool.id as keyof typeof toolIcons] ?? ShieldAlert;
            const mode = form.tools[tool.id] ?? "disabled";
            return (
              <div key={tool.id} className="flex flex-col gap-3 px-4 py-3.5 sm:flex-row sm:items-center sm:px-5">
                <div className="flex min-w-0 flex-1 items-start gap-3">
                  <span className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md border border-rule bg-surface-2 text-muted-foreground"><Icon className="size-3.5" /></span>
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2"><span className="font-mono text-[10.5px] font-medium text-ink">{tool.id}</span><ToolModeLabel mode={mode} /></div>
                    <p className="mt-0.5 text-[11px] leading-5 text-muted-foreground">{tool.description}</p>
                  </div>
                </div>
                <Select value={mode} onValueChange={(value) => setForm((current) => ({ ...current, tools: { ...current.tools, [tool.id]: value } }))}>
                  <SelectTrigger className="w-full text-[11.5px] sm:w-[180px]" aria-label={`Permission for ${tool.id}`}><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="autonomous">Run on its own</SelectItem>
                    <SelectItem value="approval_required">Needs approval</SelectItem>
                    <SelectItem value="disabled">Turned off</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            );
          })}
        </div>
        <div className="border-t border-rule bg-surface-2/40 px-4 py-2.5 text-[10.5px] text-muted-foreground sm:px-5">
          {selectedToolModes.filter((mode) => mode === "approval_required").length} tool{selectedToolModes.filter((mode) => mode === "approval_required").length === 1 ? "" : "s"} require approval. Approvals are recorded in the audit log.
        </div>
      </Card>

      <Card className="p-4 sm:p-5">
        <div className="flex items-start gap-3">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><Gauge className="size-4" /></span>
          <div className="flex-1">
            <h3 className="text-[13.5px] font-semibold">Escalation</h3>
            <p className="mt-0.5 text-[11.5px] leading-5 text-muted-foreground">The agent hands off when confidence drops below the threshold or a keyword appears.</p>
            <div className="mt-4 grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="keywords">Keywords</Label>
                <Input id="keywords" value={form.keywords} onChange={(event) => setForm((current) => ({ ...current, keywords: event.target.value }))} placeholder="refund, legal, cancel my account" />
                <p className="text-[10px] leading-4 text-subtle">Comma separated. Up to 20 words.</p>
              </div>
              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <Label htmlFor="threshold">Low-confidence threshold</Label>
                  <span className="font-mono text-[10px] text-ink-2">{form.threshold.toFixed(2)}</span>
                </div>
                <input id="threshold" type="range" min="0" max="1" step="0.01" value={form.threshold} onChange={(event) => setForm((current) => ({ ...current, threshold: Number(event.target.value) }))} className="w-full accent-[var(--color-accent)]" />
                <div className="flex justify-between font-mono text-[8.5px] uppercase tracking-[0.07em] text-subtle"><span>More answers</span><span>More handoffs</span></div>
              </div>
            </div>
          </div>
        </div>
      </Card>

      <Card className="p-4 sm:p-5">
        <div className="flex items-start gap-3">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><Sparkles className="size-4" /></span>
          <div className="min-w-0 flex-1">
            <h3 className="text-[13.5px] font-semibold">Model and budgets</h3>
            <p className="mt-0.5 text-[11.5px] leading-5 text-muted-foreground">Auto lets the desk use its active model. A larger budget allows more investigation but costs more tokens.</p>
            <div className="mt-4 grid gap-3 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label>Provider</Label>
                <Select value={form.providerId} onValueChange={(value) => setForm((current) => ({ ...current, providerId: value, modelId: "" }))}>
                  <SelectTrigger aria-label="Model provider"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="auto">Auto model</SelectItem>
                    {settings.agent.providers.map((provider) => <SelectItem key={provider.id} value={provider.id}>{provider.label}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label>Model</Label>
                {hasProvider && providerModels.length > 0 ? (
                  <Select value={form.modelId || "__auto__"} onValueChange={(value) => setForm((current) => ({ ...current, modelId: value === "__auto__" ? "" : value }))}>
                    <SelectTrigger aria-label="Model"><SelectValue placeholder="Choose a model" /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="__auto__">Provider default</SelectItem>
                      {providerModels.map((model) => <SelectItem key={model.id} value={model.id}>{model.displayName || model.id}</SelectItem>)}
                    </SelectContent>
                  </Select>
                ) : (
                  <Input value={form.modelId} onChange={(event) => setForm((current) => ({ ...current, modelId: event.target.value }))} placeholder={hasProvider ? "Model id, if not cached" : "Auto"} disabled={!hasProvider} />
                )}
              </div>
            </div>
            <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
              {[
                { key: "maxSteps" as const, label: "Max steps", hint: "1-12" },
                { key: "maxToolCalls" as const, label: "Max tools", hint: "0-12" },
                { key: "maxInputTokens" as const, label: "Input tokens", hint: "500-64k" },
                { key: "maxOutputTokens" as const, label: "Output tokens", hint: "100-16k" },
              ].map((budget) => (
                <div key={budget.key} className="space-y-1.5">
                  <Label htmlFor={budget.key}>{budget.label}</Label>
                  <Input id={budget.key} type="number" value={form[budget.key]} onChange={(event) => setForm((current) => ({ ...current, [budget.key]: event.target.value }))} />
                  <p className="font-mono text-[8.5px] uppercase tracking-[0.07em] text-subtle">{budget.hint}</p>
                </div>
              ))}
            </div>
            {settings.agent.configuredProviders === 0 ? (
              <div className="mt-4 flex items-start gap-2 rounded-md border border-[var(--color-warn-rule)] bg-[var(--color-warn-soft)] p-3 text-[11px] leading-5 text-muted-foreground">
                <CircleHelp className="mt-0.5 size-3.5 shrink-0 text-[var(--color-warn)]" />
                Connect a provider before a run can use this policy. Until then every conversation goes to a human.
              </div>
            ) : null}
          </div>
        </div>
      </Card>

      <SaveBar
        dirty={dirty}
        saving={save.isPending}
        onSave={validateAndSave}
        onDiscard={() => setForm(baseline)}
        label="Save policy"
      />
    </div>
  );
}
