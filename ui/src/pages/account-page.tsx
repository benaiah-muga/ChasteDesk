// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { Check, KeyRound, LogOut, ShieldCheck, UserRound } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { sessionKey } from "@/hooks/use-session";
import { avatarTone, cn } from "@/lib/utils";
import { PageHeader } from "@/components/page-header";
import { ErrorState } from "@/components/error-state";
import { PageSkeleton } from "@/components/boot-screen";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";

export function AccountPage() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const query = useQuery({ queryKey: ["account"], queryFn: api.account });
  const [name, setName] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [passwordError, setPasswordError] = useState("");

  useEffect(() => {
    if (query.data) setName(query.data.name);
  }, [query.data]);

  const profile = useMutation({
    mutationFn: () => api.saveAccount({ name: name.trim() }),
    onSuccess: () => {
      toast.success("Profile updated.");
      void queryClient.invalidateQueries({ queryKey: ["account"] });
      void queryClient.invalidateQueries({ queryKey: sessionKey });
    },
    onError: (error) => toast.error("The profile could not be saved", { description: error instanceof Error ? error.message : undefined }),
  });

  const password = useMutation({
    mutationFn: () => api.saveAccount({ name: name.trim(), currentPassword, newPassword }),
    onSuccess: () => {
      toast.success("Password changed.", { description: "Other sessions were signed out." });
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      void queryClient.invalidateQueries({ queryKey: sessionKey });
    },
    onError: (error) => toast.error("The password could not be changed", { description: error instanceof Error ? error.message : undefined }),
  });

  async function signOut() {
    try {
      await api.logout();
      queryClient.setQueryData(sessionKey, { authenticated: false, setupRequired: false });
      navigate("/login", { replace: true });
    } catch {
      toast.error("Sign out failed. Try again.");
    }
  }

  if (query.isLoading) return <PageSkeleton />;
  if (query.isError || !query.data) return <ErrorState error={query.error} onRetry={() => void query.refetch()} />;
  const account = query.data;

  return (
    <div className="page-enter space-y-6">
      <PageHeader
        eyebrow={<>Desk / You</>}
        title="Account"
        description="Your profile in this workspace. Your name appears on replies and audit records; your email is the sign-in identity."
        meta={<><span className="flex items-center gap-1.5"><ShieldCheck className="size-3.5 text-[var(--color-success)]" />Session protected</span><span className="text-subtle">/</span><span className="capitalize">{account.role}</span></>}
        actions={<Button variant="secondary" onClick={() => void signOut()}><LogOut /> Sign out</Button>}
      />

      <div className="grid gap-5 lg:grid-cols-[19rem_minmax(0,1fr)]">
        <Card className="p-5">
          <div className="flex items-center gap-3">
            <Avatar className="size-12"><AvatarFallback className={`${avatarTone(account.name)} text-[13px]`}>{account.initial}</AvatarFallback></Avatar>
            <div className="min-w-0">
              <p className="truncate text-[15px] font-semibold">{account.name}</p>
              <p className="truncate text-[11.5px] text-muted-foreground">{account.email}</p>
            </div>
          </div>
          <div className="mt-5 flex items-center justify-between border-t border-rule pt-3">
            <span className="font-mono text-[9px] uppercase tracking-[0.07em] text-subtle">Role</span>
            <span className="rounded-full border border-rule bg-surface-2 px-2 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-[0.07em] capitalize">{account.role}</span>
          </div>
          <div className="mt-2 flex items-center justify-between border-t border-rule pt-3">
            <span className="font-mono text-[9px] uppercase tracking-[0.07em] text-subtle">Sessions</span>
            <span className="text-[11px] text-muted-foreground">7-day browser session</span>
          </div>
          <p className="mt-4 text-[10.5px] leading-5 text-muted-foreground">Changing your password signs out every other browser. Your current session stays active.</p>
        </Card>

        <div className="space-y-5">
          <Card className="p-4 sm:p-5">
            <div className="flex items-start gap-3">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><UserRound className="size-4" /></span>
              <div className="min-w-0 flex-1">
                <h2 className="text-[13.5px] font-semibold">Profile</h2>
                <p className="mt-0.5 text-[11.5px] leading-5 text-muted-foreground">This is the name teammates see on replies and in the audit log.</p>
                <div className="mt-3 max-w-md space-y-3">
                  <div className="space-y-1.5"><Label htmlFor="account-name">Name</Label><Input id="account-name" value={name} onChange={(event) => setName(event.target.value)} maxLength={100} /></div>
                  <div className="space-y-1.5"><Label>Email</Label><div className="flex h-9 items-center rounded-md border border-rule bg-surface-2 px-3 text-[13px] text-muted-foreground">{account.email}</div><p className="text-[10px] text-subtle">The sign-in identity cannot be changed from this screen.</p></div>
                  <Button onClick={() => profile.mutate()} loading={profile.isPending} disabled={!name.trim() || name.trim() === account.name}><Check /> Save profile</Button>
                </div>
              </div>
            </div>
          </Card>

          <Card className="p-4 sm:p-5">
            <div className="flex items-start gap-3">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><KeyRound className="size-4" /></span>
              <div className="min-w-0 flex-1">
                <h2 className="text-[13.5px] font-semibold">Password</h2>
                <p className="mt-0.5 text-[11.5px] leading-5 text-muted-foreground">Use at least eight characters. All other sessions are revoked when the password changes.</p>
                <form
                  onSubmit={(event) => {
                    event.preventDefault();
                    setPasswordError("");
                    if (newPassword.length < 8) return setPasswordError("The new password must be at least eight characters.");
                    if (newPassword !== confirmPassword) return setPasswordError("The new passwords do not match.");
                    password.mutate();
                  }}
                  className="mt-3 grid max-w-md gap-3 sm:grid-cols-2"
                >
                  <div className="space-y-1.5 sm:col-span-2"><Label htmlFor="current-password">Current password</Label><Input id="current-password" type="password" autoComplete="current-password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} /></div>
                  <div className="space-y-1.5"><Label htmlFor="new-password">New password</Label><Input id="new-password" type="password" autoComplete="new-password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} /></div>
                  <div className="space-y-1.5"><Label htmlFor="confirm-password">Confirm new password</Label><Input id="confirm-password" type="password" autoComplete="new-password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} /></div>
                  {passwordError ? <p className="text-[11px] text-[var(--color-danger)] sm:col-span-2">{passwordError}</p> : null}
                  <div className="sm:col-span-2">
                    <Button type="submit" variant="secondary" loading={password.isPending} disabled={!currentPassword || !newPassword || !confirmPassword}><KeyRound /> Change password</Button>
                  </div>
                </form>
              </div>
            </div>
          </Card>

          <Card className={cn("border-[var(--color-info-rule)] bg-[var(--color-info-soft)]/45 p-4")}>
            <p className="flex items-center gap-2 text-[11.5px] font-semibold"><ShieldCheck className="size-3.5 text-[var(--color-info)]" />Account security</p>
            <p className="mt-1 text-[11px] leading-5 text-muted-foreground">ChasteDesk stores only a password hash. Sessions are HTTP-only cookies and expire after seven days. Password changes are recorded in the audit log.</p>
          </Card>
        </div>
      </div>
    </div>
  );
}
