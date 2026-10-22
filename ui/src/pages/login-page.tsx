// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { ArrowUpRight, Eye, EyeOff, LockKeyhole, ShieldCheck } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { sessionKey, useSession } from "@/hooks/use-session";
import { BootScreen } from "@/components/boot-screen";
import { Brand } from "@/components/logo";
import { ThemeSwitch } from "@/components/theme-switch";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function LoginPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [searchParams] = useSearchParams();
  const session = useSession();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [challengeToken, setChallengeToken] = useState<string | null>(null);
  const [code, setCode] = useState("");
  const codeRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (session.data?.authenticated) {
      const next = searchParams.get("next");
      navigate(session.data.onboarding && !session.data.onboarding.complete ? "/onboarding" : next?.startsWith("/dashboard") ? next : "/dashboard", { replace: true });
    } else if (session.data?.setupRequired) {
      navigate("/setup", { replace: true });
    }
  }, [navigate, searchParams, session.data]);

  if (session.isLoading) return <BootScreen />;
  if (session.isError) {
    return (
      <div className="flex min-h-dvh items-center justify-center bg-paper px-6">
        <div className="max-w-sm text-center">
          <p className="eyebrow">Connection</p>
          <h1 className="mt-2 font-display text-[34px]">The desk is not answering.</h1>
          <p className="mt-2 text-[13px] text-muted-foreground">The web process could not reach its database. Retry in a moment, or ask the operator to check readiness.</p>
          <Button className="mt-5" onClick={() => void session.refetch()}>Try again</Button>
        </div>
      </div>
    );
  }

  async function finishSignIn() {
    await queryClient.invalidateQueries({ queryKey: sessionKey });
    const nextSession = await queryClient.fetchQuery({ queryKey: sessionKey, queryFn: api.session, staleTime: 0 });
    if (nextSession.onboarding && !nextSession.onboarding.complete) {
      navigate("/onboarding", { replace: true });
    } else {
      const next = searchParams.get("next");
      navigate(next?.startsWith("/dashboard") ? next : "/dashboard", { replace: true });
    }
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      if (challengeToken) {
        await api.verifyTwoFactor(challengeToken, code.trim());
        await finishSignIn();
        return;
      }
      const result = await api.login({ email: email.trim(), password });
      if (result.twoFactorRequired && result.challengeToken) {
        setChallengeToken(result.challengeToken);
        setPassword("");
        setCode("");
        window.setTimeout(() => codeRef.current?.focus(), 20);
        return;
      }
      await finishSignIn();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "The desk could not sign you in.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex min-h-dvh flex-col bg-paper text-ink">
      <header className="mx-auto flex w-full max-w-6xl items-center justify-between px-5 py-5 sm:px-8">
        <Link to="/login" aria-label="ChasteDesk home"><Brand /></Link>
        <div className="flex items-center gap-5">
          <a
            href="https://github.com/benaiah-muga/ChasteDesk#readme"
            target="_blank"
            rel="noreferrer noopener"
            className="hidden items-center gap-1.5 text-[11px] font-medium text-muted-foreground transition-colors hover:text-ink sm:flex"
          >
            Read the docs <ArrowUpRight className="size-3" />
          </a>
          <ThemeSwitch />
        </div>
      </header>

      <main className="flex flex-1 items-center justify-center px-5 py-8 sm:py-12">
        <div className="w-full max-w-[26rem] animate-[page-enter_260ms_var(--ease-out)_both]">
          <span className="mb-6 block h-px w-10 bg-accent" aria-hidden="true" />
          {challengeToken ? (
            <>
              <p className="eyebrow">Security check</p>
              <h1 className="mt-2 font-display text-[clamp(2.5rem,6vw,3.15rem)] leading-[1.02] tracking-[-0.025em]">One more step.</h1>
              <p className="mt-2 text-[12.5px] leading-5 text-muted-foreground">Enter the six-digit code from your authenticator app for <span className="font-medium text-ink">{email}</span>. A recovery code works too.</p>
              <form onSubmit={submit} className="mt-7 space-y-4" noValidate>
                {error ? <div role="alert" className="rounded-md border border-[var(--color-danger-rule)] bg-[var(--color-danger-soft)] px-3 py-2.5 text-[12px] leading-5 text-[var(--color-danger)]">{error}</div> : null}
                <div className="space-y-1.5">
                  <Label htmlFor="auth-code">Authentication code</Label>
                  <Input
                    ref={codeRef}
                    id="auth-code"
                    name="code"
                    inputMode="text"
                    autoComplete="one-time-code"
                    placeholder="123456 or recovery code"
                    value={code}
                    onChange={(event) => setCode(event.target.value)}
                    required
                    autoFocus
                    className="h-11 font-mono text-[15px] tracking-[0.14em]"
                  />
                </div>
                <Button type="submit" className="h-10 w-full" loading={submitting}>
                  {submitting ? "Verifying..." : "Verify and sign in"}
                  {!submitting ? <ShieldCheck className="size-3.5" /> : null}
                </Button>
                <button
                  type="button"
                  onClick={() => { setChallengeToken(null); setCode(""); setError(""); }}
                  className="block w-full text-center text-[11px] font-medium text-muted-foreground hover:text-ink"
                >
                  Use a different account
                </button>
              </form>
            </>
          ) : (
            <>
              <p className="eyebrow">Welcome back</p>
              <h1 className="mt-2 font-display text-[clamp(2.5rem,6vw,3.15rem)] leading-[1.02] tracking-[-0.025em]">Sign in to your desk.</h1>
              <p className="mt-2 text-[12.5px] leading-5 text-muted-foreground">Pick up where the AI left off. Sessions last seven days on this browser.</p>
              <form onSubmit={submit} className="mt-7 space-y-4" noValidate>
                {error ? <div role="alert" className="rounded-md border border-[var(--color-danger-rule)] bg-[var(--color-danger-soft)] px-3 py-2.5 text-[12px] leading-5 text-[var(--color-danger)]">{error}</div> : null}
                <div className="space-y-1.5">
                  <Label htmlFor="email">Email</Label>
                  <Input id="email" name="email" type="email" autoComplete="username" placeholder="you@company.com" value={email} onChange={(event) => setEmail(event.target.value)} required autoFocus className="h-10" />
                </div>
                <div className="space-y-1.5">
                  <div className="flex items-center justify-between">
                    <Label htmlFor="password">Password</Label>
                    <span className="font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">Encrypted in transit</span>
                  </div>
                  <div className="relative">
                    <Input id="password" name="password" type={showPassword ? "text" : "password"} autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required className="h-10 pr-10" />
                    <button type="button" className="absolute right-1 top-1 flex size-8 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-surface-3 hover:text-ink" onClick={() => setShowPassword((value) => !value)} aria-label={showPassword ? "Hide password" : "Show password"}>
                      {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                    </button>
                  </div>
                </div>
                <Button type="submit" className="h-10 w-full" loading={submitting}>
                  {submitting ? "Signing in..." : "Sign in"}
                  {!submitting ? <LockKeyhole className="size-3.5" /> : null}
                </Button>
                <div className="flex items-center justify-between border-t border-rule pt-3 text-[11.5px] text-muted-foreground">
                  <span>Single-organization install</span>
                  {session.data?.setupRequired ? <Link to="/setup" className="font-semibold text-accent-ink hover:underline">First run? Create the desk</Link> : <span className="font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">No public sign-up</span>}
                </div>
              </form>
              <p className="mt-5 text-[10.5px] leading-5 text-muted-foreground">Access is invite-only. Ask an owner or admin for a single-use invitation.</p>
            </>
          )}
          <p className="mt-8 text-center font-mono text-[9px] uppercase tracking-[0.11em] text-subtle">Self-hosted · AGPL-3.0-only</p>
        </div>
      </main>
    </div>
  );
}
