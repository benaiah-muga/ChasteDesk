// SPDX-License-Identifier: AGPL-3.0-only
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, KeyRound, Pencil, Plus, RefreshCw, ServerCog, Trash2 } from "lucide-react";
import { toast } from "sonner";
import type { SettingsResponse } from "@/lib/types";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { SectionHeader } from "./settings-shared";

interface ProviderForm {
  providerId: string;
  displayName: string;
  apiKey: string;
  baseUrl: string;
}

const emptyForm: ProviderForm = { providerId: "openai", displayName: "", apiKey: "", baseUrl: "" };

export function ProvidersSection({ settings }: { settings: SettingsResponse }) {
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState<ProviderForm>(emptyForm);
  const [error, setError] = useState("");
  const [testingId, setTestingId] = useState<string | null>(null);
  const [removeId, setRemoveId] = useState<string | null>(null);

  const selectedOption = settings.providerOptions.find((option) => option.id === form.providerId);

  function openCreate(providerId?: string, existing?: SettingsResponse["providers"][number]) {
    setError("");
    if (existing) {
      setForm({ providerId: existing.providerId, displayName: existing.displayName, apiKey: "", baseUrl: existing.baseUrl ?? "" });
    } else if (providerId) {
      const option = settings.providerOptions.find((item) => item.id === providerId);
      setForm({ providerId, displayName: option?.label ?? "", apiKey: "", baseUrl: "" });
    } else {
      setForm(emptyForm);
    }
    setOpen(true);
  }

  const save = useMutation({
    mutationFn: () => api.saveProvider({ providerId: form.providerId, displayName: form.displayName, apiKey: form.apiKey, baseUrl: form.baseUrl }),
    onSuccess: () => {
      toast.success("Provider saved.", { description: form.apiKey ? "The key is sealed and will not be shown again." : "Provider settings updated." });
      setOpen(false);
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
      void queryClient.invalidateQueries({ queryKey: ["app-session"] });
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
    },
    onError: (err) => setError(err instanceof Error ? err.message : "The provider could not be saved."),
  });

  async function test(id: string) {
    setTestingId(id);
    try {
      const result = await api.testProvider(id);
      if (result.ok) {
        toast.success("Connection works.", { description: `${result.models?.length ?? 0} models found and cached.` });
        void queryClient.invalidateQueries({ queryKey: ["settings"] });
      } else {
        toast.error("Connection failed.", { description: result.error ?? "Check the key and base URL." });
      }
    } catch (err) {
      toast.error("Connection failed.", { description: err instanceof Error ? err.message : undefined });
    } finally {
      setTestingId(null);
    }
  }

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteProvider(id),
    onSuccess: () => {
      setRemoveId(null);
      toast.success("Provider removed.");
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
      void queryClient.invalidateQueries({ queryKey: ["app-session"] });
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
    },
    onError: (err) => toast.error("The provider could not be removed", { description: err instanceof Error ? err.message : undefined }),
  });

  return (
    <div className="space-y-5">
      <SectionHeader
        title="AI providers"
        description="Connect one or more providers. The support agent chooses the model; embeddings use whichever provider supports them."
        actions={<Button onClick={() => openCreate()}><Plus /> Add provider</Button>}
      />

      {settings.providers.length === 0 ? (
        <Card className="p-5">
          <div className="flex flex-col items-start gap-4 sm:flex-row sm:items-center">
            <span className="flex size-10 items-center justify-center rounded-full border border-rule bg-surface-2 text-muted-foreground"><KeyRound className="size-4.5" /></span>
            <div className="flex-1">
              <h3 className="font-display text-[24px] leading-tight">No provider connected.</h3>
              <p className="mt-1 text-[12.5px] leading-5 text-muted-foreground">Every conversation routes to a human until the agent has a model to think with. Keys stay encrypted and masked.</p>
            </div>
            <Button onClick={() => openCreate()}><Plus /> Connect a provider</Button>
          </div>
        </Card>
      ) : (
        <div className="space-y-3">
          {settings.providers.map((provider) => {
            const option = settings.providerOptions.find((item) => item.id === provider.providerId);
            return (
              <Card key={provider.id} className="overflow-hidden">
                <div className="flex flex-wrap items-start gap-4 p-4">
                  <span className="flex size-10 shrink-0 items-center justify-center rounded-md border border-rule bg-surface-2 font-display text-[20px]">{provider.displayName.slice(0, 1)}</span>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <h3 className="text-[13.5px] font-semibold">{provider.displayName}</h3>
                      <Badge variant={provider.enabled ? "success" : "default"}>{provider.enabled ? "Enabled" : "Disabled"}</Badge>
                      {option?.keyless ? <Badge variant="outline">Keyless</Badge> : null}
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-2 font-mono text-[9.5px] text-muted-foreground">
                      <span>{provider.providerId}</span>
                      <span className="text-subtle">/</span>
                      <span>{provider.keyLast4 ? `••••${provider.keyLast4}` : "no key stored"}</span>
                      <span className="text-subtle">/</span>
                      <span>{provider.modelCount} models cached</span>
                      {provider.baseUrl ? <><span className="text-subtle">/</span><span className="truncate">{provider.baseUrl}</span></> : null}
                    </div>
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    <Button variant="secondary" size="sm" onClick={() => void test(provider.id)} loading={testingId === provider.id}><RefreshCw /> Test</Button>
                    <Button variant="ghost" size="sm" onClick={() => openCreate(provider.providerId, provider)}><Pencil /> Edit</Button>
                    <Button variant="ghost" size="icon-sm" onClick={() => setRemoveId(provider.id)} aria-label={`Remove ${provider.displayName}`}><Trash2 /></Button>
                  </div>
                </div>
              </Card>
            );
          })}
        </div>
      )}

      <div className="grid gap-3 md:grid-cols-3">
        {settings.providerOptions
          .filter((option) => !settings.providers.some((provider) => provider.providerId === option.id))
          .map((option) => (
            <button key={option.id} type="button" onClick={() => openCreate(option.id)} className="group rounded-lg border border-dashed border-rule-strong bg-surface px-4 py-3.5 text-left transition-colors hover:border-accent hover:bg-surface-2">
              <div className="flex items-center justify-between">
                <span className="text-[12.5px] font-semibold">{option.label}</span>
                <Plus className="size-3.5 text-subtle transition-colors group-hover:text-accent" />
              </div>
              <p className="mt-1 text-[10.5px] leading-4 text-muted-foreground">{option.description}</p>
            </button>
          ))}
      </div>

      <Card className="border-[var(--color-info-rule)] bg-[var(--color-info-soft)]/50 p-4">
        <div className="flex items-start gap-2.5">
          <ServerCog className="mt-0.5 size-4 shrink-0 text-[var(--color-info)]" />
          <div>
            <p className="text-[12px] font-semibold text-ink">Keys never leave the server.</p>
            <p className="mt-0.5 text-[11px] leading-5 text-muted-foreground">Provider keys are sealed with AES-256-GCM derived from APP_SECRET. They are redacted from logs and never included in page HTML. Rotating APP_SECRET requires re-entering them.</p>
          </div>
        </div>
      </Card>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{settings.providers.some((provider) => provider.providerId === form.providerId) ? "Update provider" : "Connect a provider"}</DialogTitle>
            <DialogDescription>Use a provider key with model and embedding access. Leave the key blank to keep the stored one.</DialogDescription>
          </DialogHeader>
          <form onSubmit={(event) => { event.preventDefault(); setError(""); save.mutate(); }} className="space-y-4">
            {error ? <div role="alert" className="rounded-md border border-[var(--color-danger-rule)] bg-[var(--color-danger-soft)] px-3 py-2.5 text-[12px] text-[var(--color-danger)]">{error}</div> : null}
            <div className="space-y-1.5">
              <Label>Provider</Label>
              <Select value={form.providerId} onValueChange={(value) => { const option = settings.providerOptions.find((item) => item.id === value); setForm((current) => ({ ...current, providerId: value, displayName: current.displayName || option?.label || "" })); }}>
                <SelectTrigger aria-label="Provider"><SelectValue /></SelectTrigger>
                <SelectContent>{settings.providerOptions.map((option) => <SelectItem key={option.id} value={option.id}>{option.label}{option.keyless ? " · keyless" : ""}</SelectItem>)}</SelectContent>
              </Select>
              {selectedOption ? <p className="text-[10.5px] text-muted-foreground">{selectedOption.description}</p> : null}
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5"><Label htmlFor="provider-name">Display name</Label><Input id="provider-name" value={form.displayName} onChange={(event) => setForm((current) => ({ ...current, displayName: event.target.value }))} placeholder={selectedOption?.label} maxLength={100} /></div>
              <div className="space-y-1.5"><Label htmlFor="provider-key">API key {selectedOption?.keyless ? <span className="font-normal text-subtle">optional</span> : null}</Label><Input id="provider-key" type="password" autoComplete="off" value={form.apiKey} onChange={(event) => setForm((current) => ({ ...current, apiKey: event.target.value }))} placeholder={settings.providers.some((provider) => provider.providerId === form.providerId) ? "Stored key stays in place" : "sk-..."} /></div>
            </div>
            <div className="space-y-1.5"><Label htmlFor="provider-url">Base URL <span className="font-normal text-subtle">optional</span></Label><Input id="provider-url" type="url" value={form.baseUrl} onChange={(event) => setForm((current) => ({ ...current, baseUrl: event.target.value }))} placeholder="https://api.openai.com/v1" /><p className="text-[10.5px] text-muted-foreground">Required for local Ollama and most OpenAI-compatible endpoints.</p></div>
            <DialogFooter>
              <Button type="button" variant="secondary" onClick={() => setOpen(false)}>Cancel</Button>
              <Button type="submit" loading={save.isPending}><Check /> Save provider</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(removeId)}
        onOpenChange={(next) => { if (!next) setRemoveId(null); }}
        title="Remove this provider?"
        description="The key is deleted. The agent will pause any model call until another provider is connected or the same one is reconnected."
        confirmLabel="Remove provider"
        destructive
        loading={remove.isPending}
        onConfirm={() => { if (removeId) remove.mutate(removeId); }}
      />
    </div>
  );
}
