// SPDX-License-Identifier: AGPL-3.0-only
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Cable, KeyRound, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { toast } from "sonner";
import type { SettingsResponse } from "@/lib/types";
import { api } from "@/lib/api";
import { relativeTime } from "@/lib/format";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { CopyButton } from "@/components/copy-button";
import { SectionHeader } from "./settings-shared";

export function SCIMSection({ settings }: { settings: SettingsResponse }) {
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [issuedToken, setIssuedToken] = useState<string | null>(null);
  const [revokeID, setRevokeID] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: () => api.createSCIMToken(name.trim()),
    onSuccess: (result) => {
      setIssuedToken(result.token);
      setName("");
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
    },
    onError: (error) => toast.error("The SCIM token could not be created", { description: error instanceof Error ? error.message : undefined }),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => api.revokeSCIMToken(id),
    onSuccess: () => {
      setRevokeID(null);
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
      toast.success("SCIM token revoked.", { description: "The identity provider can no longer use it." });
    },
    onError: (error) => toast.error("The SCIM token could not be revoked", { description: error instanceof Error ? error.message : undefined }),
  });

  function closeDialog(openState: boolean) {
    setOpen(openState);
    if (!openState) {
      setIssuedToken(null);
      setName("");
    }
  }

  return (
    <div className="space-y-5">
      <SectionHeader
        title="SCIM provisioning"
        description="Let an identity provider create, update, deactivate, and reactivate teammates over SCIM 2.0. Tokens are organization-scoped and shown once."
        actions={settings.scim.canManage ? <Button onClick={() => setOpen(true)}><Plus /> Generate token</Button> : undefined}
      />

      <Card className="overflow-hidden">
        <div className="border-b border-rule px-4 py-3.5 sm:px-5">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2"><Cable className="size-4 text-muted-foreground" /><h3 className="text-[13.5px] font-semibold">SCIM 2.0 endpoint</h3></div>
            <Badge variant="outline">Users only</Badge>
          </div>
          <div className="mt-3 flex items-center gap-2 rounded-md border border-rule bg-surface-2/55 px-2.5 py-2">
            <code className="min-w-0 flex-1 truncate font-mono text-[11px] text-ink-2">{settings.scim.baseUrl}</code>
            <CopyButton value={settings.scim.baseUrl} label="Copy SCIM base URL" iconOnly />
          </div>
          <p className="mt-2 text-[11px] leading-5 text-muted-foreground">Send <code className="font-mono text-[10px]">Authorization: Bearer &lt;token&gt;</code>. Supported resources: <code className="font-mono text-[10px]">/Users</code>, <code className="font-mono text-[10px]">/ServiceProviderConfig</code>, and <code className="font-mono text-[10px]">/ResourceTypes</code>. Roles are read from the SCIM <code className="font-mono text-[10px]">roles</code> attribute and must be owner, admin, or agent.</p>
        </div>
        <div className="divide-y divide-rule">
          {settings.scim.tokens.length === 0 ? (
            <div className="px-4 py-8 text-center sm:px-5">
              <KeyRound className="mx-auto size-5 text-subtle" />
              <p className="mt-2 text-[12px] font-medium">No SCIM tokens yet.</p>
              <p className="mt-1 text-[11px] text-muted-foreground">{settings.scim.canManage ? "Generate a token in your identity provider and paste it into the SCIM connector." : "Only an owner can issue provisioning tokens."}</p>
            </div>
          ) : settings.scim.tokens.map((token) => (
            <div key={token.id} className="flex flex-wrap items-center gap-3 px-4 py-3 sm:px-5">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><KeyRound className="size-3.5" /></span>
              <div className="min-w-0 flex-1">
                <p className="truncate text-[12.5px] font-semibold">{token.name}</p>
                <p className="mt-0.5 font-mono text-[9px] uppercase tracking-[0.05em] text-subtle">Created {relativeTime(token.createdAt)}{token.lastUsedAt ? ` · last used ${relativeTime(token.lastUsedAt)}` : " · never used"}</p>
              </div>
              <Button size="sm" variant="ghost" className="text-[var(--color-danger)] hover:bg-[var(--color-danger-soft)]" onClick={() => setRevokeID(token.id)}><Trash2 /> Revoke</Button>
            </div>
          ))}
        </div>
      </Card>

      <Card className="border-[var(--color-info-rule)] bg-[var(--color-info-soft)]/45 p-4">
        <p className="flex items-center gap-2 text-[11.5px] font-semibold"><ShieldCheck className="size-3.5 text-[var(--color-info)]" />Activation and deactivation</p>
        <p className="mt-1 text-[11px] leading-5 text-muted-foreground">A SCIM-created user is added to the workspace immediately and receives a pending activation invitation. Copy that invitation link from Workspace, or activate the user directly from the invitation. Deactivation removes membership and revokes every session. Tokens are hashed at rest and can be revoked at any time.</p>
      </Card>

      <Dialog open={open} onOpenChange={closeDialog}>
        <DialogContent>
          {issuedToken ? (
            <>
              <DialogHeader>
                <DialogTitle>Copy this SCIM token now.</DialogTitle>
                <DialogDescription>The full token is shown once. Paste it into your identity provider's SCIM connector.</DialogDescription>
              </DialogHeader>
              <div className="space-y-3">
                <div className="rounded-md border border-rule bg-surface-2/55 p-3">
                  <p className="font-mono text-[8.5px] uppercase tracking-[0.08em] text-subtle">Bearer token</p>
                  <div className="mt-1.5 flex items-center gap-2">
                    <code className="min-w-0 flex-1 break-all font-mono text-[11px]">{issuedToken}</code>
                    <CopyButton value={issuedToken} label="Copy SCIM token" iconOnly />
                  </div>
                </div>
                <div className="rounded-md border border-rule bg-surface-2/55 p-3">
                  <p className="font-mono text-[8.5px] uppercase tracking-[0.08em] text-subtle">SCIM base URL</p>
                  <code className="mt-1 block break-all font-mono text-[11px]">{settings.scim.baseUrl}</code>
                </div>
              </div>
              <DialogFooter><Button onClick={() => closeDialog(false)}>I have copied it</Button></DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>Generate a SCIM token.</DialogTitle>
                <DialogDescription>Name it after the identity provider or environment so operators know where it is used.</DialogDescription>
              </DialogHeader>
              <form onSubmit={(event) => { event.preventDefault(); create.mutate(); }} className="space-y-3">
                <div className="space-y-1.5"><Label htmlFor="scim-token-name">Token name</Label><Input id="scim-token-name" placeholder="Okta production" value={name} onChange={(event) => setName(event.target.value)} maxLength={80} required autoFocus /></div>
                <DialogFooter>
                  <Button type="button" variant="secondary" onClick={() => closeDialog(false)}>Cancel</Button>
                  <Button type="submit" loading={create.isPending} disabled={!name.trim()}>Generate</Button>
                </DialogFooter>
              </form>
            </>
          )}
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(revokeID)}
        onOpenChange={(state) => { if (!state) setRevokeID(null); }}
        title="Revoke this SCIM token?"
        description="Requests using this token will fail immediately. The identity provider will need a new token to sync users."
        confirmLabel="Revoke token"
        destructive
        loading={revoke.isPending}
        onConfirm={() => { if (revokeID) revoke.mutate(revokeID); }}
      />
    </div>
  );
}
