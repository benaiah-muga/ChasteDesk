// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { Check, KeyRound, LogOut, ShieldCheck, UserRound } from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
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
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { CopyButton } from "@/components/copy-button";

export function AccountPage() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const query = useQuery({ queryKey: ["account"], queryFn: api.account });
  const [name, setName] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [passwordError, setPasswordError] = useState("");
  const [twoFactorOpen, setTwoFactorOpen] = useState(false);
  const [twoFactorSetup, setTwoFactorSetup] = useState<{ secret: string; otpauthUri: string } | null>(null);
  const [twoFactorCode, setTwoFactorCode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const [disableOpen, setDisableOpen] = useState(false);
  const [disablePassword, setDisablePassword] = useState("");
  const [disableCode, setDisableCode] = useState("");

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

  const twoFactorSetupMutation = useMutation({
    mutationFn: api.setupTwoFactor,
    onSuccess: (result) => {
      setTwoFactorSetup(result);
      setTwoFactorCode("");
    },
    onError: (error) => toast.error("Two-factor setup could not start", { description: error instanceof Error ? error.message : undefined }),
  });

  const twoFactorEnable = useMutation({
    mutationFn: () => api.enableTwoFactor(twoFactorCode.trim()),
    onSuccess: (result) => {
      setRecoveryCodes(result.recoveryCodes);
      setTwoFactorSetup(null);
      setTwoFactorCode("");
      void queryClient.invalidateQueries({ queryKey: ["account"] });
      toast.success("Two-factor authentication enabled.", { description: "Store the recovery codes somewhere safe." });
    },
    onError: (error) => toast.error("Two-factor authentication could not be enabled", { description: error instanceof Error ? error.message : undefined }),
  });

  const twoFactorDisable = useMutation({
    mutationFn: () => api.disableTwoFactor(disablePassword, disableCode.trim()),
    onSuccess: () => {
      setDisableOpen(false);
      setDisablePassword("");
      setDisableCode("");
      void queryClient.invalidateQueries({ queryKey: ["account"] });
      toast.success("Two-factor authentication disabled.", { description: "Password sign-in is active again for this account." });
    },
    onError: (error) => toast.error("Two-factor authentication could not be disabled", { description: error instanceof Error ? error.message : undefined }),
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

          <Card className="p-4 sm:p-5">
            <div className="flex items-start gap-3">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><ShieldCheck className="size-4" /></span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="text-[13.5px] font-semibold">Two-factor authentication</h2>
                  <Badge variant={account.twoFactorEnabled ? "success" : "outline"}>{account.twoFactorEnabled ? "Enabled" : "Not enabled"}</Badge>
                </div>
                <p className="mt-0.5 text-[11.5px] leading-5 text-muted-foreground">Require a rotating code from an authenticator app when this account signs in. Recovery codes are issued once when you enable it.</p>
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  {account.twoFactorEnabled ? (
                    <>
                      <span className="flex items-center gap-1.5 text-[10.5px] text-[var(--color-success)]"><ShieldCheck className="size-3.5" /> Protected by an authenticator app</span>
                      <Button variant="secondary" size="sm" onClick={() => setDisableOpen(true)}>Disable</Button>
                    </>
                  ) : (
                    <Button variant="secondary" size="sm" onClick={() => { setTwoFactorOpen(true); if (!twoFactorSetup) twoFactorSetupMutation.mutate(); }} loading={twoFactorSetupMutation.isPending}>Set up authenticator</Button>
                  )}
                </div>
              </div>
            </div>
          </Card>

          <Card className={cn("border-[var(--color-info-rule)] bg-[var(--color-info-soft)]/45 p-4")}>
            <p className="flex items-center gap-2 text-[11.5px] font-semibold"><ShieldCheck className="size-3.5 text-[var(--color-info)]" />Account security</p>
            <p className="mt-1 text-[11px] leading-5 text-muted-foreground">ChasteDesk stores only a password hash. Sessions are HTTP-only cookies and expire after seven days. Password changes are recorded in the audit log.</p>
          </Card>
        </div>
      </div>

      <Dialog open={twoFactorOpen} onOpenChange={(open) => { setTwoFactorOpen(open); if (!open) { setTwoFactorSetup(null); setRecoveryCodes(null); setTwoFactorCode(""); } }}>
        <DialogContent>
          {recoveryCodes ? (
            <>
              <DialogHeader>
                <DialogTitle>Save your recovery codes.</DialogTitle>
                <DialogDescription>These ten single-use codes can sign you in if you lose your authenticator app. They are shown once.</DialogDescription>
              </DialogHeader>
              <div className="grid grid-cols-2 gap-2 rounded-lg border border-rule bg-surface-2/55 p-3">
                {recoveryCodes.map((recovery) => <code key={recovery} className="rounded border border-rule bg-surface px-2 py-1 text-center font-mono text-[11px] tracking-[0.06em]">{recovery}</code>)}
              </div>
              <DialogFooter>
                <CopyButton value={recoveryCodes.join("\n")} label="Copy all codes" />
                <Button onClick={() => setTwoFactorOpen(false)}><Check /> I have saved them</Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>Set up an authenticator.</DialogTitle>
                <DialogDescription>Scan this code with your authenticator app, then enter the six-digit code it shows.</DialogDescription>
              </DialogHeader>
              {twoFactorSetup ? (
                <div className="space-y-4">
                  <div className="flex justify-center rounded-lg border border-rule bg-[#f8f4ec] p-4">
                    <QRCodeSVG value={twoFactorSetup.otpauthUri} size={168} bgColor="#f8f4ec" fgColor="#211b16" level="M" />
                  </div>
                  <div className="rounded-md border border-rule bg-surface-2/55 px-3 py-2">
                    <p className="font-mono text-[8.5px] uppercase tracking-[0.08em] text-subtle">Setup key</p>
                    <div className="mt-1 flex items-center gap-2">
                      <code className="min-w-0 flex-1 break-all font-mono text-[11px] text-ink-2">{twoFactorSetup.secret}</code>
                      <CopyButton value={twoFactorSetup.secret} label="Copy key" iconOnly />
                    </div>
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="two-factor-code">Authentication code</Label>
                    <Input id="two-factor-code" inputMode="numeric" autoComplete="one-time-code" placeholder="123456" value={twoFactorCode} onChange={(event) => setTwoFactorCode(event.target.value)} className="h-10 font-mono tracking-[0.12em]" maxLength={6} autoFocus />
                  </div>
                </div>
              ) : (
                <div className="flex items-center justify-center py-8 text-[11.5px] text-muted-foreground">Preparing a secure setup key...</div>
              )}
              <DialogFooter>
                <Button variant="secondary" onClick={() => setTwoFactorOpen(false)}>Cancel</Button>
                <Button onClick={() => twoFactorEnable.mutate()} loading={twoFactorEnable.isPending} disabled={!twoFactorSetup || twoFactorCode.trim().length < 6}>Verify and enable</Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={disableOpen} onOpenChange={(open) => { setDisableOpen(open); if (!open) { setDisablePassword(""); setDisableCode(""); } }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Disable two-factor authentication?</DialogTitle>
            <DialogDescription>Confirm your password and a current authenticator or recovery code. Other sessions will be signed out.</DialogDescription>
          </DialogHeader>
          <form onSubmit={(event) => { event.preventDefault(); twoFactorDisable.mutate(); }} className="space-y-3">
            <div className="space-y-1.5"><Label htmlFor="disable-password">Current password</Label><Input id="disable-password" type="password" autoComplete="current-password" value={disablePassword} onChange={(event) => setDisablePassword(event.target.value)} required autoFocus /></div>
            <div className="space-y-1.5"><Label htmlFor="disable-code">Authentication code</Label><Input id="disable-code" inputMode="numeric" autoComplete="one-time-code" placeholder="123456 or recovery code" value={disableCode} onChange={(event) => setDisableCode(event.target.value)} required /></div>
            <DialogFooter>
              <Button type="button" variant="secondary" onClick={() => setDisableOpen(false)}>Cancel</Button>
              <Button type="submit" loading={twoFactorDisable.isPending} disabled={!disablePassword || !disableCode.trim()}>Disable</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
