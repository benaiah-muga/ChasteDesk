// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { AlertTriangle, Check, KeyRound, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import type { SettingsResponse } from "@/lib/types";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { EmptyState } from "@/components/empty-state";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { CopyButton } from "@/components/copy-button";
import { SectionHeader } from "./settings-shared";

export function APIKeysSection({ settings }: { settings: SettingsResponse }) {
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [secret, setSecret] = useState("");
  const [revokeId, setRevokeId] = useState<string | null>(null);

  useEffect(() => {
    if (searchParams.get("create") === "1") {
      setCreateOpen(true);
      setSearchParams((current) => {
        const next = new URLSearchParams(current);
        next.delete("create");
        return next;
      }, { replace: true });
    }
  }, [searchParams, setSearchParams]);

  const create = useMutation({
    mutationFn: () => api.createApiKey(name.trim()),
    onSuccess: (result) => {
      setSecret(result.secret);
      setName("");
      setCreateOpen(false);
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
    },
    onError: (error) => toast.error("The API key could not be created", { description: error instanceof Error ? error.message : undefined }),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => api.revokeApiKey(id),
    onSuccess: () => {
      setRevokeId(null);
      toast.success("API key revoked.", { description: "Requests using this key will fail immediately." });
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
    },
    onError: (error) => toast.error("The API key could not be revoked", { description: error instanceof Error ? error.message : undefined }),
  });

  return (
    <div className="space-y-5">
      <SectionHeader
        title="API keys"
        description="Keys for the REST channel. Each key belongs to this workspace and can be revoked instantly. The secret is shown once."
        actions={<Button onClick={() => setCreateOpen(true)}><Plus /> Create API key</Button>}
      />

      <Card className="overflow-hidden">
        {settings.apiKeys.length === 0 ? (
          <EmptyState icon={KeyRound} title="No API keys yet." body="Create a key when a script or integration needs to open conversations through the REST channel." actionLabel="Create the first key" onAction={() => setCreateOpen(true)} />
        ) : (
          <div className="divide-y divide-rule">
            {settings.apiKeys.map((key) => (
              <div key={key.id} className="flex flex-wrap items-center gap-3 px-4 py-3.5 sm:px-5">
                <span className="flex size-8 items-center justify-center rounded-md border border-rule bg-surface-2 text-muted-foreground"><KeyRound className="size-3.5" /></span>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2"><span className="text-[12.5px] font-semibold">{key.name}</span><span className="rounded border border-rule bg-surface-2 px-1.5 py-0.5 font-mono text-[9px] text-muted-foreground">{key.prefix}••••</span></div>
                  <p className="mt-0.5 text-[10.5px] text-muted-foreground">Created {key.createdAt} · Last used {key.lastUsed === "never" ? "never" : key.lastUsed}</p>
                </div>
                <Button variant="ghost" size="sm" onClick={() => setRevokeId(key.id)}><Trash2 /> Revoke</Button>
              </div>
            ))}
          </div>
        )}
        <div className="flex items-center justify-between border-t border-rule bg-surface-2/40 px-5 py-2.5 font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">
          <span>{settings.apiKeys.length} active {settings.apiKeys.length === 1 ? "key" : "keys"}</span>
          <span>Keys are hashed at rest</span>
        </div>
      </Card>

      <Card className="border-[var(--color-info-rule)] bg-[var(--color-info-soft)]/50 p-4">
        <div className="flex items-start gap-2.5">
          <AlertTriangle className="mt-0.5 size-4 shrink-0 text-[var(--color-info)]" />
          <p className="text-[11px] leading-5 text-muted-foreground">API keys grant access to the REST channel. Treat them like passwords: store them in a secret manager, rotate them on a schedule, and revoke immediately if one leaks.</p>
        </div>
      </Card>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create an API key</DialogTitle>
            <DialogDescription>Name it for the integration that will use it. The secret appears only once, immediately after creation.</DialogDescription>
          </DialogHeader>
          <form onSubmit={(event) => { event.preventDefault(); if (!name.trim()) return; create.mutate(); }} className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="key-name">Key name</Label>
              <Input id="key-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="Warehouse sync" maxLength={80} autoFocus />
              <p className="text-[10.5px] text-muted-foreground">1 to 80 characters. Use a name another teammate will recognize.</p>
            </div>
            <DialogFooter>
              <Button type="button" variant="secondary" onClick={() => setCreateOpen(false)}>Cancel</Button>
              <Button type="submit" loading={create.isPending} disabled={!name.trim()}><Check /> Create key</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(secret)} onOpenChange={(open) => { if (!open) setSecret(""); }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Copy this key now.</DialogTitle>
            <DialogDescription>It will not be shown again. Store it somewhere safe before closing this dialog.</DialogDescription>
          </DialogHeader>
          <div className="rounded-lg border border-[var(--color-warn-rule)] bg-[var(--color-warn-soft)]/60 p-3">
            <div className="flex items-start gap-2">
              <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-[var(--color-warn)]" />
              <p className="text-[11px] leading-5 text-muted-foreground">If you lose it, revoke the key and create a new one. There is no recovery.</p>
            </div>
            <div className="mt-2 flex items-center gap-2 rounded-md border border-rule bg-surface px-2.5 py-2">
              <code className="min-w-0 flex-1 break-all font-mono text-[10.5px]">{secret}</code>
              <CopyButton value={secret} label="Copy key" />
            </div>
          </div>
          <DialogFooter>
            <Button onClick={() => setSecret("")}><Check /> I have stored it</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(revokeId)}
        onOpenChange={(open) => { if (!open) setRevokeId(null); }}
        title="Revoke this API key?"
        description="Requests using this key will start failing immediately. This cannot be undone, but you can create a new key in its place."
        confirmLabel="Revoke key"
        destructive
        loading={revoke.isPending}
        onConfirm={() => { if (revokeId) revoke.mutate(revokeId); }}
      />
    </div>
  );
}
