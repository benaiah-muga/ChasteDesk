// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from "@tanstack/react-query";
import { NavLink, useOutletContext, useParams, Navigate } from "react-router-dom";
import { BookOpen, Bot, Cable, Clock3, KeyRound, Palette, ServerCog, UserRound, Users } from "lucide-react";
import type { AppSession } from "@/lib/types";
import { cn } from "@/lib/utils";
import { api } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { ErrorState } from "@/components/error-state";
import { PageSkeleton } from "@/components/boot-screen";
import { ProvidersSection } from "./providers-section";
import { AgentSection } from "./agent-section";
import { WidgetSection } from "./widget-section";
import { APIKeysSection } from "./api-keys-section";
import { WorkspaceSection } from "./workspace-section";
import { SLASection } from "./sla-section";
import { SCIMSection } from "./scim-section";

const sections = [
  { key: "providers", label: "AI providers", description: "Models and keys", icon: ServerCog },
  { key: "agent", label: "Support agent", description: "Purpose and guardrails", icon: Bot },
  { key: "widget", label: "Chat widget", description: "Appearance and origins", icon: Palette },
  { key: "api-keys", label: "API keys", description: "REST channel access", icon: KeyRound },
  { key: "workspace", label: "Workspace", description: "People and invitations", icon: Users },
  { key: "sla", label: "SLA policies", description: "Targets and breach alerts", icon: Clock3 },
  { key: "provisioning", label: "Provisioning", description: "SCIM directory sync", icon: Cable },
];
export const settingsKeys = sections.map((section) => section.key);

export function SettingsPage() {
  const { session } = useOutletContext<{ session: AppSession }>();
  const params = useParams();
  const section = params.section ?? "providers";
  const query = useQuery({ queryKey: ["settings"], queryFn: api.settings, staleTime: 10_000 });

  if (!session.permissions?.canSettings) return <Navigate to="/dashboard/account" replace />;
  if (!settingsKeys.includes(section)) return <Navigate to="/dashboard/settings/providers" replace />;
  if (query.isLoading) return <PageSkeleton />;
  if (query.isError || !query.data) return <ErrorState error={query.error} title="Settings could not open" onRetry={() => void query.refetch()} />;
  const settings = query.data;

  return (
    <div className="page-enter space-y-6">
      <PageHeader
        eyebrow={<>Desk / Configuration</>}
        title="Settings"
        description="The provider, policy, and people behind the desk. Provider keys are encrypted at rest and returned masked, forever."
        meta={
          <>
            <span className="flex items-center gap-1.5"><span className={cn("size-1.5 rounded-full", settings.providers.length > 0 ? "bg-[var(--color-success)]" : "bg-[var(--color-warn)]")} />{settings.providers.length} provider{settings.providers.length === 1 ? "" : "s"} connected</span>
            <span className="text-subtle">/</span>
            <span>{settings.org.name}</span>
            <span className="text-subtle">/</span>
            <span className="capitalize">{settings.user.role}</span>
          </>
        }
      />

      <div className="grid gap-6 lg:grid-cols-[13rem_minmax(0,1fr)]">
        <nav aria-label="Settings sections" className="min-w-0 lg:sticky lg:top-20 lg:z-10 lg:self-start">
          <div className="flex gap-1 overflow-x-auto pb-1 lg:flex-col lg:overflow-visible lg:pb-0">
            {sections.map((item) => {
              const Icon = item.icon;
              return (
                <NavLink
                  key={item.key}
                  to={`/dashboard/settings/${item.key}`}
                  className={({ isActive }) =>
                    cn(
                      "group flex shrink-0 items-center gap-2.5 rounded-md border px-2.5 py-2 transition-colors lg:border-transparent",
                      isActive
                        ? "border-rule bg-surface text-ink shadow-[var(--shadow-whisper)] lg:border-rule"
                        : "border-transparent text-muted-foreground hover:bg-surface-2 hover:text-ink",
                    )
                  }
                >
                  <Icon className="size-3.5 shrink-0" />
                  <span className="min-w-0">
                    <span className="block whitespace-nowrap text-[12px] font-semibold">{item.label}</span>
                    <span className="hidden text-[10px] text-subtle lg:block">{item.description}</span>
                  </span>
                </NavLink>
              );
            })}
            <div className="my-1 hidden h-px bg-rule lg:block" />
            <NavLink to="/dashboard/account" className={({ isActive }) => cn("group flex shrink-0 items-center gap-2.5 rounded-md border px-2.5 py-2 transition-colors lg:border-transparent", isActive ? "border-rule bg-surface text-ink" : "border-transparent text-muted-foreground hover:bg-surface-2 hover:text-ink")}>
              <UserRound className="size-3.5 shrink-0" />
              <span>
                <span className="block whitespace-nowrap text-[12px] font-semibold">Account</span>
                <span className="hidden text-[10px] text-subtle lg:block">Your profile and password</span>
              </span>
            </NavLink>
            <a href="https://github.com/benaiah-muga/ChasteDesk#readme" target="_blank" rel="noreferrer noopener" className="hidden items-center gap-2.5 rounded-md px-2.5 py-2 text-muted-foreground transition-colors hover:bg-surface-2 hover:text-ink lg:flex">
              <BookOpen className="size-3.5 shrink-0" />
              <span><span className="block text-[12px] font-semibold">Documentation</span><span className="text-[10px] text-subtle">Self-hosting guide</span></span>
            </a>
          </div>
          <div className="mt-4 hidden rounded-lg border border-rule bg-surface-2/45 p-3 lg:block">
            <p className="eyebrow">Secrets</p>
            <p className="mt-1.5 text-[10.5px] leading-4 text-muted-foreground">Keys are sealed with AES-256-GCM derived from APP_SECRET. They never reach page HTML.</p>
          </div>
        </nav>
        <div className="min-w-0">
          {section === "providers" ? <ProvidersSection settings={settings} /> : null}
          {section === "agent" ? <AgentSection settings={settings} /> : null}
          {section === "widget" ? <WidgetSection settings={settings} /> : null}
          {section === "api-keys" ? <APIKeysSection settings={settings} /> : null}
          {section === "workspace" ? <WorkspaceSection settings={settings} /> : null}
          {section === "sla" ? <SLASection settings={settings} /> : null}
          {section === "provisioning" ? <SCIMSection settings={settings} /> : null}
        </div>
      </div>
    </div>
  );
}
