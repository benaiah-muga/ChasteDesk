// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { Check, LogOut, MailPlus, MoreHorizontal, Plus, ShieldCheck, Trash2, UserMinus, UserPlus } from "lucide-react";
import { toast } from "sonner";
import type { SettingsResponse } from "@/lib/types";
import { api } from "@/lib/api";
import { avatarTone, cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { CopyButton } from "@/components/copy-button";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { SectionHeader, SaveBar } from "./settings-shared";

export function WorkspaceSection({ settings }: { settings: SettingsResponse }) {
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const [name, setName] = useState(settings.org.name);
  const [baseline, setBaseline] = useState(settings.org.name);
  const [inviteOpen, setInviteOpen] = useState(false);
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteRole, setInviteRole] = useState("agent");
  const [createdInvite, setCreatedInvite] = useState<{ email: string; link: string } | null>(null);
  const [revokeId, setRevokeId] = useState<string | null>(null);
  const [removeTarget, setRemoveTarget] = useState<{ id: string; name: string; email: string } | null>(null);
  const canRename = settings.user.role === "owner";

  useEffect(() => {
    setName(settings.org.name);
    setBaseline(settings.org.name);
  }, [settings.org.name]);

  useEffect(() => {
    if (searchParams.get("invite") === "1") {
      setInviteOpen(true);
      setSearchParams((current) => {
        const next = new URLSearchParams(current);
        next.delete("invite");
        return next;
      }, { replace: true });
    }
  }, [searchParams, setSearchParams]);

  const dirty = name.trim() !== baseline;
  const save = useMutation({
    mutationFn: () => api.saveWorkspace(name.trim()),
    onSuccess: () => {
      setBaseline(name.trim());
      toast.success("Workspace name saved.");
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
      void queryClient.invalidateQueries({ queryKey: ["app-session"] });
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
    },
    onError: (error) => toast.error("The workspace could not be saved", { description: error instanceof Error ? error.message : undefined }),
  });

  const invite = useMutation({
    mutationFn: () => api.invite(inviteEmail.trim(), inviteRole),
    onSuccess: (result) => {
      setCreatedInvite({ email: result.email, link: result.link });
      setInviteEmail("");
      setInviteOpen(false);
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
      toast.success("Invitation ready.", { description: "Copy the single-use link and send it to your teammate." });
    },
    onError: (error) => toast.error("The invitation could not be created", { description: error instanceof Error ? error.message : undefined }),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => api.revokeInvite(id),
    onSuccess: () => {
      setRevokeId(null);
      toast.success("Invitation revoked.");
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
    },
    onError: (error) => toast.error("The invitation could not be revoked", { description: error instanceof Error ? error.message : undefined }),
  });

  const roleLabel = useMemo(() => (role: string) => role === "owner" ? "Owner" : role === "admin" ? "Admin" : "Agent", []);

  const memberRole = useMutation({
    mutationFn: ({ id, role }: { id: string; role: string }) => api.updateMemberRole(id, role),
    onSuccess: (result) => {
      toast.success("Member role updated.", { description: result.sessionsRevoked > 0 ? "Their sessions were signed out so the new role takes effect." : undefined });
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
      void queryClient.invalidateQueries({ queryKey: ["app-session"] });
    },
    onError: (error) => toast.error("The member role could not be changed", { description: error instanceof Error ? error.message : undefined }),
  });

  const memberRemove = useMutation({
    mutationFn: (id: string) => api.removeMember(id),
    onSuccess: () => {
      setRemoveTarget(null);
      toast.success("Member removed.", { description: "Their sessions were revoked. The account can be invited again." });
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
      void queryClient.invalidateQueries({ queryKey: ["app-session"] });
    },
    onError: (error) => toast.error("The member could not be removed", { description: error instanceof Error ? error.message : undefined }),
  });

  const memberSessions = useMutation({
    mutationFn: (id: string) => api.revokeMemberSessions(id),
    onSuccess: (result) => {
      toast.success(result.revoked > 0 ? `${result.revoked} session${result.revoked === 1 ? "" : "s"} revoked.` : "No active sessions were found.");
    },
    onError: (error) => toast.error("The sessions could not be revoked", { description: error instanceof Error ? error.message : undefined }),
  });

  return (
    <div className="space-y-5">
      <SectionHeader
        title="Workspace"
        description="The name on the desk, the people who can work it, and the invitations still waiting to be accepted."
        actions={settings.canManage ? <Button onClick={() => setInviteOpen(true)}><UserPlus /> Invite teammate</Button> : undefined}
      />

      <Card className="p-4 sm:p-5">
        <div className="flex items-start gap-3">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><ShieldCheck className="size-4" /></span>
          <div className="min-w-0 flex-1">
            <h3 className="text-[13.5px] font-semibold">Workspace name</h3>
            <p className="mt-0.5 text-[11.5px] leading-5 text-muted-foreground">Shown in the sidebar and used for the widget display name when one is not set.</p>
            <div className="mt-3 flex max-w-md flex-col gap-2 sm:flex-row">
              <Input value={name} onChange={(event) => setName(event.target.value)} maxLength={100} disabled={!canRename} aria-label="Workspace name" />
              <Button type="button" onClick={() => save.mutate()} loading={save.isPending} disabled={!dirty || !canRename}><Check /> Save</Button>
            </div>
            {!canRename ? <p className="mt-1.5 text-[10.5px] text-subtle">Only the workspace owner can rename it. You can invite teammates as an admin.</p> : null}
          </div>
        </div>
        <SaveBar dirty={dirty && canRename} saving={save.isPending} onSave={() => save.mutate()} onDiscard={() => setName(baseline)} label="Save name" />
      </Card>

      <Card className="overflow-hidden">
        <div className="flex items-center justify-between border-b border-rule px-4 py-3.5 sm:px-5">
          <div>
            <h3 className="text-[13.5px] font-semibold">Members</h3>
            <p className="mt-0.5 text-[11px] text-muted-foreground">Everyone shares the same customer data and inbox. Owners can change roles, revoke sessions, or remove a member.</p>
          </div>
          <Badge variant="outline">{settings.members.length} people</Badge>
        </div>
        <div className="divide-y divide-rule">
          {settings.members.map((member) => (
            <div key={member.email} className="flex flex-wrap items-center gap-3 px-4 py-3 sm:px-5">
              <span className={cn("flex size-8 items-center justify-center rounded-full border border-rule font-mono text-[10px] font-semibold", avatarTone(member.name))}>{member.name.slice(0, 1).toUpperCase()}</span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2"><span className="truncate text-[12.5px] font-semibold">{member.name}</span>{member.self ? <Badge variant="outline">You</Badge> : null}</div>
                <p className="truncate text-[10.5px] text-muted-foreground">{member.email}</p>
              </div>
              <span className="hidden font-mono text-[9px] uppercase tracking-[0.07em] text-subtle sm:inline">joined {member.joined}</span>
              {settings.canAdmin && !member.self ? (
                <Select value={member.role} onValueChange={(role) => memberRole.mutate({ id: member.id, role })} disabled={memberRole.isPending}>
                  <SelectTrigger className="h-7 w-[6.6rem] text-[10.5px]" aria-label={`Role for ${member.name}`}><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="agent">Agent</SelectItem>
                    <SelectItem value="admin">Admin</SelectItem>
                    <SelectItem value="owner">Owner</SelectItem>
                  </SelectContent>
                </Select>
              ) : (
                <Badge variant={member.role === "owner" ? "ink" : member.role === "admin" ? "info" : "outline"}>{roleLabel(member.role)}</Badge>
              )}
              {settings.canAdmin && !member.self ? (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="icon-sm" aria-label={`Manage ${member.name}`}><MoreHorizontal /></Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-52">
                    <DropdownMenuItem onClick={() => memberSessions.mutate(member.id)} disabled={memberSessions.isPending}><LogOut /> Revoke sessions</DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem destructive onClick={() => setRemoveTarget({ id: member.id, name: member.name, email: member.email })}><UserMinus /> Remove member</DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              ) : null}
            </div>
          ))}
        </div>
      </Card>

      <Card className="overflow-hidden">
        <div className="flex items-center justify-between border-b border-rule px-4 py-3.5 sm:px-5">
          <div>
            <h3 className="text-[13.5px] font-semibold">Pending invitations</h3>
            <p className="mt-0.5 text-[11px] text-muted-foreground">Single-use links that expire after seven days.</p>
          </div>
          <Badge variant="outline">{settings.invitations.length} open</Badge>
        </div>
        {settings.invitations.length === 0 ? (
          <div className="px-5 py-8 text-center">
            <MailPlus className="mx-auto size-5 text-subtle" />
            <p className="mt-2 font-display text-[22px]">No one is waiting.</p>
            <p className="mt-1 text-[11.5px] text-muted-foreground">Invite a teammate when you are ready to share the desk.</p>
          </div>
        ) : (
          <div className="divide-y divide-rule">
            {settings.invitations.map((invitation) => (
              <div key={invitation.id} className="flex flex-wrap items-center gap-3 px-4 py-3 sm:px-5">
                <span className="flex size-8 items-center justify-center rounded-md border border-rule bg-surface-2 text-muted-foreground"><MailPlus className="size-3.5" /></span>
                <div className="min-w-0 flex-1"><p className="truncate text-[12px] font-semibold">{invitation.email}</p><p className="text-[10.5px] text-muted-foreground"><span className="capitalize">{invitation.role}</span> · expires {invitation.expires}</p></div>
                <CopyButton value={invitation.link} label="Copy link" />
                <Button variant="ghost" size="sm" onClick={() => setRevokeId(invitation.id)}><Trash2 /> Revoke</Button>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Dialog open={inviteOpen} onOpenChange={setInviteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Invite a teammate</DialogTitle>
            <DialogDescription>Invitations are single-use and expire after seven days. The person chooses their own name and password.</DialogDescription>
          </DialogHeader>
          <form onSubmit={(event) => { event.preventDefault(); if (!inviteEmail.trim()) return; invite.mutate(); }} className="space-y-4">
            <div className="space-y-1.5"><Label htmlFor="invite-email">Email</Label><Input id="invite-email" type="email" value={inviteEmail} onChange={(event) => setInviteEmail(event.target.value)} placeholder="teammate@company.com" autoFocus required /></div>
            <div className="space-y-1.5"><Label>Role</Label>
              <Select value={inviteRole} onValueChange={setInviteRole}>
                <SelectTrigger aria-label="Invitation role"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="agent">Agent · inbox and knowledge</SelectItem>
                  <SelectItem value="admin">Admin · inbox, knowledge, settings, audit</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <DialogFooter>
              <Button type="button" variant="secondary" onClick={() => setInviteOpen(false)}>Cancel</Button>
              <Button type="submit" loading={invite.isPending} disabled={!inviteEmail.trim()}><Plus /> Create invite link</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(createdInvite)} onOpenChange={(open) => { if (!open) setCreatedInvite(null); }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Invite link ready.</DialogTitle>
            <DialogDescription>Send this single-use link to {createdInvite?.email}. It expires in seven days and stops working after acceptance.</DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2 rounded-md border border-rule bg-surface-2/55 px-2.5 py-2">
            <code className="min-w-0 flex-1 break-all font-mono text-[10.5px] text-muted-foreground">{createdInvite?.link}</code>
            <CopyButton value={createdInvite?.link ?? ""} label="Copy link" />
          </div>
          <DialogFooter><Button onClick={() => setCreatedInvite(null)}><Check /> Done</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(revokeId)}
        onOpenChange={(open) => { if (!open) setRevokeId(null); }}
        title="Revoke this invitation?"
        description="The link will stop working immediately. You can create a fresh invitation at any time."
        confirmLabel="Revoke invitation"
        destructive
        loading={revoke.isPending}
        onConfirm={() => { if (revokeId) revoke.mutate(revokeId); }}
      />

      <ConfirmDialog
        open={Boolean(removeTarget)}
        onOpenChange={(open) => { if (!open) setRemoveTarget(null); }}
        title={`Remove ${removeTarget?.name ?? "this member"}?`}
        description={`${removeTarget?.email ?? "This account"} will lose access to the workspace immediately. Their active sessions will be revoked. You can invite the account again later.`}
        confirmLabel="Remove member"
        destructive
        loading={memberRemove.isPending}
        onConfirm={() => { if (removeTarget) memberRemove.mutate(removeTarget.id); }}
      />
    </div>
  );
}
