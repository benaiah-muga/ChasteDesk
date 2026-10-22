// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, BadgeCheck, Building2, Eye, EyeOff } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { sessionKey, useSession } from "@/hooks/use-session";
import { AuthLayout, FieldError } from "@/components/auth-layout";
import { BootScreen, PageSkeleton } from "@/components/boot-screen";
import { ErrorState } from "@/components/error-state";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";

export function InvitePage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const session = useSession();
  const invite = useQuery({ queryKey: ["invitation", id], queryFn: () => api.invitation(id), retry: false });
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (session.data?.authenticated) navigate("/dashboard", { replace: true });
  }, [navigate, session.data?.authenticated]);

  if (session.isLoading) return <BootScreen />;

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    const errors: Record<string, string> = {};
    if (!name.trim()) errors.name = "Enter your name.";
    if (password.length < 8) errors.password = "Use at least eight characters.";
    setFieldErrors(errors);
    setError("");
    if (Object.keys(errors).length) return;
    setSubmitting(true);
    try {
      const accepted = await api.acceptInvitation(id, { name: name.trim(), password });
      queryClient.setQueryData(sessionKey, accepted);
      navigate("/dashboard", { replace: true });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "The invitation could not be accepted.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <AuthLayout
      eyebrow="Invitation"
      title={invite.data ? `Join ${invite.data.orgName}.` : "Join the desk."}
      description="Set your name and a password. This single-use link will stop working once you accept."
    >
      {invite.isLoading ? (
        <PageSkeleton />
      ) : invite.isError || !invite.data ? (
        <div className="rounded-lg border border-rule bg-surface p-5">
          <ErrorState error={invite.error} title="This invitation is not valid" onRetry={() => void invite.refetch()} />
        </div>
      ) : (
        <>
          <div className="mb-5 grid gap-2 rounded-lg border border-rule bg-surface p-3.5 sm:grid-cols-2">
            <div className="flex items-center gap-2.5">
              <span className="flex size-8 items-center justify-center rounded-md bg-[var(--color-accent-soft)] text-[var(--color-accent-ink)]"><Building2 className="size-4" /></span>
              <span>
                <span className="block font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">Workspace</span>
                <span className="block text-[12.5px] font-semibold">{invite.data.orgName}</span>
              </span>
            </div>
            <div className="flex items-center gap-2.5">
              <span className="flex size-8 items-center justify-center rounded-md bg-surface-3 text-muted-foreground"><BadgeCheck className="size-4" /></span>
              <span>
                <span className="block font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">Role</span>
                <span className="block text-[12.5px] font-semibold capitalize">{invite.data.role}</span>
              </span>
            </div>
          </div>
          <form onSubmit={submit} className="space-y-4" noValidate>
            {error ? (
              <div role="alert" className="rounded-md border border-[var(--color-danger-rule)] bg-[var(--color-danger-soft)] px-3 py-2.5 text-[12px] leading-5 text-[var(--color-danger)]">
                {error}
              </div>
            ) : null}
            <div className="space-y-1.5">
              <Label>Email</Label>
              <div className="flex h-10 items-center rounded-md border border-rule bg-surface-2 px-3 text-[13px] text-muted-foreground">{invite.data.email}</div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="name">Your name</Label>
              <Input id="name" value={name} onChange={(event) => setName(event.target.value)} autoComplete="name" required autoFocus className="h-10" aria-invalid={Boolean(fieldErrors.name)} />
              <FieldError>{fieldErrors.name}</FieldError>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="password">Create a password</Label>
              <div className="relative">
                <Input
                  id="password"
                  type={showPassword ? "text" : "password"}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  autoComplete="new-password"
                  minLength={8}
                  required
                  className="h-10 pr-10"
                  aria-invalid={Boolean(fieldErrors.password)}
                />
                <button
                  type="button"
                  className="absolute right-1 top-1 flex size-8 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-surface-3 hover:text-ink"
                  onClick={() => setShowPassword((value) => !value)}
                  aria-label={showPassword ? "Hide password" : "Show password"}
                >
                  {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                </button>
              </div>
              <FieldError>{fieldErrors.password}</FieldError>
            </div>
            <Button type="submit" className="h-10 w-full" loading={submitting}>
              Accept invitation <ArrowRight className="size-3.5" />
            </Button>
            <p className="text-center text-[11px] text-muted-foreground">Invited as <Badge variant="outline" className="ml-1 align-middle">{invite.data.role}</Badge></p>
          </form>
        </>
      )}
    </AuthLayout>
  );
}
