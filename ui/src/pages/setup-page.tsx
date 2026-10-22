// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { ArrowRight, Check, Eye, EyeOff, KeyRound, Sparkles } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { sessionKey, useSession } from "@/hooks/use-session";
import { AuthLayout, FieldError } from "@/components/auth-layout";
import { BootScreen } from "@/components/boot-screen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function SetupPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const session = useSession();
  const [orgName, setOrgName] = useState("");
  const [userName, setUserName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (session.data?.authenticated) navigate(session.data.onboarding && !session.data.onboarding.complete ? "/onboarding" : "/dashboard", { replace: true });
    else if (session.data && !session.data.setupRequired) navigate("/login", { replace: true });
  }, [navigate, session.data]);

  if (session.isLoading) return <BootScreen />;

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    const errors: Record<string, string> = {};
    if (!orgName.trim()) errors.orgName = "Give the workspace a name.";
    if (!userName.trim()) errors.userName = "Tell us what to call you.";
    if (!email.trim() || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim())) errors.email = "Enter a valid email address.";
    if (password.length < 8) errors.password = "Use at least eight characters.";
    setFieldErrors(errors);
    setFormError("");
    if (Object.keys(errors).length > 0) return;
    setSubmitting(true);
    try {
      const created = await api.setup({
        orgName: orgName.trim(),
        userName: userName.trim(),
        email: email.trim(),
        password,
      });
      queryClient.setQueryData(sessionKey, created);
      navigate("/onboarding", { replace: true });
    } catch (error) {
      setFormError(error instanceof ApiError ? error.message : "The desk could not be created.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <AuthLayout
      eyebrow="First run"
      title="Create your desk."
      description="This installation is empty. The first account becomes the owner, with full access to settings, audit, and the inbox."
      wide
    >
      <form onSubmit={submit} className="space-y-4" noValidate>
        {formError ? (
          <div role="alert" className="rounded-md border border-[var(--color-danger-rule)] bg-[var(--color-danger-soft)] px-3 py-2.5 text-[12px] leading-5 text-[var(--color-danger)]">
            {formError}
          </div>
        ) : null}
        <div className="space-y-1.5">
          <Label htmlFor="orgName">Workspace name</Label>
          <Input
            id="orgName"
            value={orgName}
            onChange={(event) => setOrgName(event.target.value)}
            placeholder="Acme Support"
            maxLength={100}
            autoFocus
            required
            aria-invalid={Boolean(fieldErrors.orgName)}
            className="h-10"
          />
          <div className="flex items-center justify-between">
            <FieldError>{fieldErrors.orgName}</FieldError>
            <span className="ml-auto text-[10.5px] text-subtle">Shown in the sidebar. Change it later.</span>
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="userName">Your name</Label>
            <Input
              id="userName"
              value={userName}
              onChange={(event) => setUserName(event.target.value)}
              placeholder="Ada Lovelace"
              autoComplete="name"
              maxLength={100}
              required
              aria-invalid={Boolean(fieldErrors.userName)}
              className="h-10"
            />
            <FieldError>{fieldErrors.userName}</FieldError>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="email">Email</Label>
            <Input
              id="email"
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              placeholder="you@company.com"
              autoComplete="username"
              required
              aria-invalid={Boolean(fieldErrors.email)}
              className="h-10"
            />
            <FieldError>{fieldErrors.email}</FieldError>
          </div>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="password">Password</Label>
          <div className="relative">
            <Input
              id="password"
              type={showPassword ? "text" : "password"}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="new-password"
              minLength={8}
              maxLength={200}
              required
              aria-invalid={Boolean(fieldErrors.password)}
              className="h-10 pr-10"
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
          <div className="flex items-center gap-2">
            <div className="flex flex-1 gap-1" aria-hidden="true">
              {[0, 1, 2, 3].map((index) => (
                <span
                  key={index}
                  className={`h-0.5 flex-1 rounded-full transition-colors ${
                    password.length >= [0, 8, 12, 16][index]! ? "bg-[var(--color-success)]" : "bg-rule"
                  }`}
                />
              ))}
            </div>
            <span className="font-mono text-[9px] uppercase tracking-[0.07em] text-subtle">
              {password.length === 0 ? "8+ characters" : password.length < 8 ? "Too short" : password.length < 12 ? "Good" : "Strong"}
            </span>
          </div>
          <FieldError>{fieldErrors.password}</FieldError>
        </div>
        <Button type="submit" className="h-10 w-full" loading={submitting}>
          Create the desk
          {!submitting ? <ArrowRight className="size-3.5" /> : null}
        </Button>
        <div className="grid gap-2 rounded-lg border border-rule bg-surface-2/55 p-3.5 sm:grid-cols-3">
          {[
            { icon: Check, label: "Private by default" },
            { icon: KeyRound, label: "Owner account" },
            { icon: Sparkles, label: "No sample data" },
          ].map((item) => (
            <div key={item.label} className="flex items-center gap-2 text-[11.5px] text-muted-foreground">
              <item.icon className="size-3.5 text-accent" />
              {item.label}
            </div>
          ))}
        </div>
        <p className="text-center text-[11.5px] text-muted-foreground">
          Already have a desk? <Link to="/login" className="font-semibold text-accent-ink hover:underline">Sign in</Link>
        </p>
      </form>
    </AuthLayout>
  );
}
