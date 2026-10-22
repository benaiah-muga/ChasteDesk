// SPDX-License-Identifier: AGPL-3.0-only
import { useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Code2, Globe2, MessageCircle, Plus, Sparkles, X } from "lucide-react";
import { toast } from "sonner";
import type { SettingsResponse } from "@/lib/types";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { CopyButton } from "@/components/copy-button";
import { SectionHeader, SaveBar } from "./settings-shared";

type WidgetForm = SettingsResponse["widget"];

function originValid(value: string): boolean {
  try {
    const parsed = new URL(value);
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
}

export function WidgetSection({ settings }: { settings: SettingsResponse }) {
  const queryClient = useQueryClient();
  const initial = useMemo(() => settings.widget, [settings.widget]);
  const [form, setForm] = useState<WidgetForm>(initial);
  const [baseline, setBaseline] = useState<WidgetForm>(initial);
  const [originInput, setOriginInput] = useState("");
  const [error, setError] = useState("");
  const dirty = JSON.stringify(form) !== JSON.stringify(baseline);

  const save = useMutation({
    mutationFn: () =>
      api.saveWidget({
        origins: form.origins,
        enabled: form.enabled,
        displayName: form.displayName,
        greeting: form.greeting,
        primaryColor: form.primaryColor,
        position: form.position,
      }),
    onSuccess: () => {
      setBaseline(form);
      toast.success("Widget settings saved.", { description: "The new origin rules apply to the next widget request." });
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
      void queryClient.invalidateQueries({ queryKey: ["app-session"] });
    },
    onError: (err) => toast.error("Widget settings could not be saved", { description: err instanceof Error ? err.message : undefined }),
  });

  function addOrigin() {
    setError("");
    const value = originInput.trim();
    if (!value) return;
    if (!originValid(value)) {
      setError("Use a full URL, such as https://example.com. Paths are ignored.");
      return;
    }
    const parsed = new URL(value);
    const normalized = `${parsed.protocol}//${parsed.host}`;
    if (form.origins.includes(normalized)) {
      setError("That origin is already allowed.");
      return;
    }
    if (form.origins.length >= 20) {
      setError("Allow at most 20 origins.");
      return;
    }
    setForm((current) => ({ ...current, origins: [...current.origins, normalized] }));
    setOriginInput("");
  }

  return (
    <div className="space-y-5">
      <SectionHeader
        title="Chat widget"
        description="The customer-facing launcher and the only origins allowed to load it. A widget request from an unlisted origin is refused."
      />

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="space-y-4">
          <Card className="p-4 sm:p-5">
            <div className="flex items-center justify-between gap-4 border-b border-rule pb-4">
              <div className="flex items-center gap-3">
                <span className="flex size-8 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><MessageCircle className="size-4" /></span>
                <div>
                  <p className="text-[12.5px] font-semibold">Widget enabled</p>
                  <p className="text-[10.5px] text-muted-foreground">When off, the embed script does nothing.</p>
                </div>
              </div>
              <Switch checked={form.enabled} onCheckedChange={(checked) => setForm((current) => ({ ...current, enabled: checked }))} aria-label="Enable widget" />
            </div>
            <div className="mt-4 grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="widget-name">Display name</Label>
                <Input id="widget-name" value={form.displayName} onChange={(event) => setForm((current) => ({ ...current, displayName: event.target.value }))} maxLength={60} placeholder={settings.org.name} />
                <p className="text-[10px] text-subtle">{form.displayName.length}/60</p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="widget-greeting">Greeting</Label>
                <Input id="widget-greeting" value={form.greeting} onChange={(event) => setForm((current) => ({ ...current, greeting: event.target.value }))} maxLength={160} placeholder="Hi, how can we help?" />
                <p className="text-[10px] text-subtle">{form.greeting.length}/160</p>
              </div>
            </div>
            <div className="mt-4 grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="widget-color">Accent colour</Label>
                <div className="flex items-center gap-2">
                  <input id="widget-color-picker" type="color" value={form.primaryColor} onChange={(event) => setForm((current) => ({ ...current, primaryColor: event.target.value }))} className="size-9 cursor-pointer rounded-md border border-rule bg-surface p-1" aria-label="Pick accent colour" />
                  <Input id="widget-color" value={form.primaryColor} onChange={(event) => setForm((current) => ({ ...current, primaryColor: event.target.value }))} maxLength={7} className="font-mono text-[12px]" />
                </div>
              </div>
              <div className="space-y-1.5">
                <Label>Launcher position</Label>
                <div className="inline-flex h-9 items-center gap-0.5 rounded-md border border-rule bg-surface-2 p-0.5">
                  {(["right", "left"] as const).map((position) => (
                    <button key={position} type="button" onClick={() => setForm((current) => ({ ...current, position }))} className={cn("h-8 rounded-sm px-3 text-[11.5px] capitalize transition-colors", form.position === position ? "bg-surface text-ink shadow-[var(--shadow-whisper)]" : "text-muted-foreground hover:text-ink")}>{position}</button>
                  ))}
                </div>
              </div>
            </div>
          </Card>

          <Card className="p-4 sm:p-5">
            <div className="flex items-start gap-3">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><Globe2 className="size-4" /></span>
              <div className="min-w-0 flex-1">
                <h3 className="text-[13.5px] font-semibold">Allowed origins</h3>
                <p className="mt-0.5 text-[11.5px] leading-5 text-muted-foreground">Full host origins only, up to 20. A staging URL and a production URL are separate allowances.</p>
                <form onSubmit={(event) => { event.preventDefault(); addOrigin(); }} className="mt-3 flex gap-2">
                  <Input value={originInput} onChange={(event) => { setOriginInput(event.target.value); setError(""); }} placeholder="https://example.com" aria-label="Allowed origin" />
                  <Button type="submit" variant="secondary"><Plus /> Add</Button>
                </form>
                {error ? <p className="mt-1.5 text-[11px] text-[var(--color-danger)]">{error}</p> : null}
                <div className="mt-3 space-y-1.5">
                  {form.origins.length ? form.origins.map((origin) => (
                    <div key={origin} className="flex items-center gap-2 rounded-md border border-rule bg-surface-2/55 px-2.5 py-1.5">
                      <Globe2 className="size-3.5 shrink-0 text-muted-foreground" />
                      <code className="min-w-0 flex-1 truncate font-mono text-[10.5px] text-ink-2">{origin}</code>
                      <button type="button" onClick={() => setForm((current) => ({ ...current, origins: current.origins.filter((item) => item !== origin) }))} className="flex size-6 items-center justify-center rounded text-muted-foreground hover:bg-surface-3 hover:text-ink" aria-label={`Remove ${origin}`}><X className="size-3.5" /></button>
                    </div>
                  )) : <p className="rounded-md border border-dashed border-rule px-3 py-3 text-center text-[11px] text-muted-foreground">No origins allowed. The widget will refuse to load.</p>}
                </div>
              </div>
            </div>
          </Card>

          <Card className="overflow-hidden">
            <div className="flex items-center justify-between border-b border-rule px-4 py-3">
              <div className="flex items-center gap-2"><Code2 className="size-3.5 text-muted-foreground" /><span className="text-[12.5px] font-semibold">Embed snippet</span></div>
              <CopyButton value={settings.widget.embedSnippet} label="Copy snippet" />
            </div>
            <pre className="max-h-32 overflow-auto whitespace-pre-wrap break-all bg-surface-2/55 p-4 font-mono text-[10px] leading-5 text-muted-foreground">{settings.widget.embedSnippet}</pre>
            <div className="border-t border-rule px-4 py-2 text-[10px] text-muted-foreground">Place this before the closing body tag. The token is signed for this workspace and these origins.</div>
          </Card>
        </div>

        <div className="space-y-3">
          <div className="rounded-xl border border-rule bg-surface-2/50 p-4">
            <div className="flex items-center justify-between pb-3">
              <span className="eyebrow">Live preview</span>
              <span className="font-mono text-[8.5px] uppercase tracking-[0.08em] text-subtle">Launcher {form.position}</span>
            </div>
            <div className="relative flex h-[22rem] flex-col overflow-hidden rounded-lg border border-rule bg-paper">
              <div className="border-b border-rule bg-surface px-3 py-2.5">
                <div className="flex items-center gap-2">
                  <span className="flex size-6 items-center justify-center rounded-full text-[9px] font-semibold text-white" style={{ backgroundColor: form.primaryColor }}>{form.displayName.slice(0, 1) || "C"}</span>
                  <div><p className="text-[10.5px] font-semibold">{form.displayName || settings.org.name}</p><p className="flex items-center gap-1 text-[8.5px] text-muted-foreground"><span className="size-1 rounded-full bg-[var(--color-success)]" />Online</p></div>
                </div>
              </div>
              <div className="flex flex-1 flex-col justify-end gap-2 p-3">
                <div className="max-w-[85%] rounded-xl rounded-bl-sm border border-rule bg-surface px-2.5 py-2 text-[10px] leading-4 text-ink">{form.greeting || "Hi, how can we help?"}</div>
                <div className="ml-auto max-w-[85%] rounded-xl rounded-br-sm px-2.5 py-2 text-[10px] leading-4 text-white" style={{ backgroundColor: form.primaryColor }}>I need help with an invoice.</div>
              </div>
              <div className={cn("absolute bottom-3", form.position === "right" ? "right-3" : "left-3")}>
                <span className="flex size-9 items-center justify-center rounded-full text-white shadow-[var(--shadow-float)]" style={{ backgroundColor: form.primaryColor }}><MessageCircle className="size-4" /></span>
              </div>
            </div>
            <p className="mt-2 flex items-center gap-1.5 text-[10px] text-muted-foreground"><Sparkles className="size-3 text-accent" /> Preview uses the unsaved form.</p>
          </div>
          <Card className="p-3.5">
            <p className="eyebrow">Good to know</p>
            <p className="mt-1.5 text-[11px] leading-5 text-muted-foreground">The widget is isolated in a closed shadow root. It will not inherit host page styles, and host scripts cannot read the conversation DOM.</p>
          </Card>
        </div>
      </div>

      <SaveBar dirty={dirty} saving={save.isPending} onSave={() => save.mutate()} onDiscard={() => setForm(baseline)} label="Save widget" />
    </div>
  );
}
