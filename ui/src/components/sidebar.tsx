// SPDX-License-Identifier: AGPL-3.0-only
import {
  BadgeCheck,
  BarChart3,
  BookOpen,
  Bot,
  ChevronDown,
  ChevronsLeft,
  ChevronsRight,
  CircleHelp,
  ExternalLink,
  Gauge,
  Inbox,
  LogOut,
  MessageSquareText,
  ScrollText,
  Settings,
  UserRound,
  Users,
  X,
} from "lucide-react";
import { NavLink, useLocation, useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type { AppSession } from "@/lib/types";
import { cn, initials } from "@/lib/utils";
import { api } from "@/lib/api";
import { sessionKey } from "@/hooks/use-session";
import { Brand } from "@/components/logo";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { ThemeSwitch } from "@/components/theme-switch";

export interface NavItem {
  label: string;
  to: string;
  icon: typeof Inbox;
  count?: number;
  attention?: boolean;
}

export interface NavGroup {
  label: string;
  items: NavItem[];
}

export function navGroups(session: AppSession): NavGroup[] {
  const counts = session.counts;
  const permissions = session.permissions;
  const groups: NavGroup[] = [
    {
      label: "Desk",
      items: [
        { label: "Overview", to: "/dashboard", icon: Gauge },
        { label: "Inbox", to: "/dashboard/inbox", icon: Inbox, count: counts?.openConversations, attention: (counts?.awaitingHuman ?? 0) > 0 },
        { label: "Approvals", to: "/dashboard/approvals", icon: BadgeCheck, count: counts?.pendingApprovals, attention: (counts?.pendingApprovals ?? 0) > 0 },
        { label: "Customers", to: "/dashboard/customers", icon: Users },
        { label: "Quick replies", to: "/dashboard/replies", icon: MessageSquareText },
      ],
    },
    {
      label: "Knowledge",
      items: [
        { label: "Sources", to: "/dashboard/knowledge", icon: BookOpen },
        { label: "Analytics", to: "/dashboard/analytics", icon: BarChart3 },
        ...(permissions?.canAudit ? [{ label: "Audit log", to: "/dashboard/audit", icon: ScrollText }] : []),
      ],
    },
  ];
  if (permissions?.canSettings) {
    groups.push({
      label: "Configure",
      items: [
        { label: "Support agent", to: "/dashboard/settings/agent", icon: Bot },
        { label: "Settings", to: "/dashboard/settings/providers", icon: Settings },
      ],
    });
  }
  groups.push({
    label: "You",
    items: [{ label: "Account", to: "/dashboard/account", icon: UserRound }],
  });
  return groups;
}

function isActivePath(pathname: string, to: string): boolean {
  if (to === "/dashboard") return pathname === "/dashboard";
  if (to === "/dashboard/settings" || to === "/dashboard/settings/providers") {
    return pathname.startsWith("/dashboard/settings") && pathname !== "/dashboard/account";
  }
  return pathname === to || pathname.startsWith(to + "/");
}

function NavRow({
  item,
  collapsed,
  onNavigate,
}: {
  item: NavItem;
  collapsed: boolean;
  onNavigate?: () => void;
}) {
  const location = useLocation();
  const active = isActivePath(location.pathname + location.search, item.to) || location.pathname.startsWith(item.to + "/");
  const Icon = item.icon;
  const content = (
    <NavLink
      to={item.to}
      onClick={onNavigate}
      aria-current={active ? "page" : undefined}
      className={cn(
        "group relative flex h-9 items-center gap-2.5 rounded-md px-2.5 text-[13px] font-medium transition-colors",
        active
          ? "bg-[var(--color-sidebar-3)] text-[var(--color-sidebar-active)]"
          : "text-[var(--color-sidebar-muted)] hover:bg-[var(--color-sidebar-2)] hover:text-[var(--color-sidebar-ink)]",
        collapsed && "justify-center px-0",
      )}
    >
      <span
        className={cn(
          "absolute left-0 top-2 h-5 w-[2px] rounded-full bg-accent opacity-0 transition-opacity",
          active && "opacity-100",
        )}
        aria-hidden="true"
      />
      <Icon className="size-[17px] shrink-0" strokeWidth={1.75} />
      {collapsed ? null : (
        <>
          <span className="truncate">{item.label}</span>
          {item.count ? (
            <span
              className={cn(
                "ml-auto inline-flex min-w-5 items-center justify-center rounded-full px-1.5 py-0.5 font-mono text-[9px] font-semibold leading-none",
                item.attention
                  ? "bg-[var(--color-accent-strong)] text-[var(--color-on-accent)]"
                  : "bg-[var(--color-sidebar-3)] text-[var(--color-sidebar-muted)]",
              )}
            >
              {item.count > 99 ? "99+" : item.count}
            </span>
          ) : null}
        </>
      )}
      {collapsed && item.count ? (
        <span className={cn("absolute right-1.5 top-1.5 size-1.5 rounded-full", item.attention ? "bg-accent" : "bg-[var(--color-sidebar-muted)]")} />
      ) : null}
    </NavLink>
  );
  if (!collapsed) return content;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{content}</TooltipTrigger>
      <TooltipContent side="right">{item.label}</TooltipContent>
    </Tooltip>
  );
}

export function SidebarContent({
  session,
  collapsed = false,
  onCollapsedChange,
  onNavigate,
  mobile = false,
  onClose,
  onShowShortcuts,
}: {
  session: AppSession;
  collapsed?: boolean;
  onCollapsedChange?: (value: boolean) => void;
  onNavigate?: () => void;
  mobile?: boolean;
  onClose?: () => void;
  onShowShortcuts: () => void;
}) {
  const groups = navGroups(session);
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const user = session.user!;
  const org = session.org!;
  const onboarding = session.onboarding;

  async function signOut() {
    try {
      await api.logout();
      queryClient.setQueryData(sessionKey, { authenticated: false, setupRequired: false });
      navigate("/login", { replace: true });
    } catch {
      toast.error("Sign out failed. Try again.");
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--color-sidebar)] text-[var(--color-sidebar-ink)]">
      <div className={cn("flex h-16 shrink-0 items-center border-b border-[var(--color-sidebar-rule)] px-4", collapsed && "justify-center px-2")}>
        <button
          type="button"
          onClick={() => navigate("/dashboard")}
          className="rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-accent)]"
          aria-label="Go to overview"
        >
          <Brand compact={collapsed} onDark />
        </button>
        {mobile ? (
          <button type="button" className="ml-auto rounded-md p-2 text-[var(--color-sidebar-muted)] hover:bg-[var(--color-sidebar-2)] hover:text-[var(--color-sidebar-ink)]" onClick={onClose} aria-label="Close navigation">
            <X className="size-4" />
          </button>
        ) : null}
      </div>

      <div className={cn("shrink-0 border-b border-[var(--color-sidebar-rule)] px-3 py-3", collapsed && "px-2")}>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className={cn(
                "flex w-full items-center gap-2.5 rounded-md border border-[var(--color-sidebar-rule)] bg-[var(--color-sidebar-2)] px-2.5 py-2 text-left transition-colors hover:border-[var(--color-sidebar-3)] hover:bg-[var(--color-sidebar-3)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-accent)]",
                collapsed && "justify-center px-0",
              )}
              title={collapsed ? org.name : undefined}
            >
              <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-[var(--color-sidebar-3)] font-mono text-[10px] font-semibold text-[var(--color-sidebar-ink)]">
                {org.initial || initials(org.name)}
              </span>
              {collapsed ? null : (
                <>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[12.5px] font-semibold text-[var(--color-sidebar-ink)]">{org.name}</span>
                    <span className="block truncate font-mono text-[9px] uppercase tracking-[0.09em] text-[var(--color-sidebar-muted)]">
                      {user.role} · self-hosted
                    </span>
                  </span>
                  <ChevronDown className="size-3.5 shrink-0 text-[var(--color-sidebar-muted)]" />
                </>
              )}
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" side="bottom" className="w-60">
            <DropdownMenuLabel>{org.name}</DropdownMenuLabel>
            <DropdownMenuItem onClick={() => navigate("/dashboard/settings/workspace")}>
              <Settings /> Workspace settings
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => navigate("/dashboard/account")}>
              <UserRound /> Your account
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => window.open("https://github.com/benaiah-muga/ChasteDesk#readme", "_blank", "noopener,noreferrer")}>
              <ExternalLink /> Documentation
            </DropdownMenuItem>
            <DropdownMenuItem onClick={signOut}>
              <LogOut /> Sign out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <nav className="min-h-0 flex-1 overflow-y-auto px-2.5 py-4" aria-label="Primary">
        {onboarding && !onboarding.complete ? (
          <button
            type="button"
            onClick={() => navigate("/onboarding")}
            className={cn(
              "mb-4 flex w-full items-start gap-2.5 rounded-md border border-[var(--color-sidebar-rule)] bg-[color-mix(in_oklab,var(--color-accent)_14%,transparent)] p-2.5 text-left transition-colors hover:border-[var(--color-accent)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-accent)]",
              collapsed && "justify-center p-2",
            )}
          >
            <span className="mt-0.5 flex size-5 items-center justify-center rounded-full bg-accent text-[10px] font-semibold text-[var(--color-on-accent)]">
              {Number(onboarding.providerConfigured) + Number(onboarding.knowledgeAdded)}/2
            </span>
            {collapsed ? null : (
              <span>
                <span className="block text-[11.5px] font-semibold text-[var(--color-sidebar-ink)]">Finish setup</span>
                <span className="mt-0.5 block text-[10.5px] leading-4 text-[var(--color-sidebar-muted)]">
                  {onboarding.providerConfigured ? "Add knowledge to start answering." : "Connect a model provider."}
                </span>
              </span>
            )}
          </button>
        ) : null}
        <div className="space-y-5">
          {groups.map((group) => (
            <div key={group.label}>
              {collapsed ? (
                <div className="mx-auto mb-2 h-px w-6 bg-[var(--color-sidebar-rule)]" aria-hidden="true" />
              ) : (
                <div className="mb-1.5 px-2.5 font-mono text-[10px] font-semibold uppercase leading-[1.2] tracking-[0.11em] text-[var(--color-sidebar-muted)]">{group.label}</div>
              )}
              <div className="space-y-0.5">
                {group.items.map((item) => (
                  <NavRow key={item.to + item.label} item={item} collapsed={collapsed} onNavigate={onNavigate} />
                ))}
              </div>
            </div>
          ))}
        </div>
      </nav>

      <div className="shrink-0 border-t border-[var(--color-sidebar-rule)] px-3 py-3">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className={cn(
                "flex w-full items-center gap-2.5 rounded-md px-1 py-1.5 text-left transition-colors hover:bg-[var(--color-sidebar-2)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-accent)]",
                collapsed && "justify-center",
              )}
              aria-label="Open account menu"
            >
              <Avatar className="size-7 border-[var(--color-sidebar-rule)]">
                <AvatarFallback className="bg-[var(--color-sidebar-3)] text-[var(--color-sidebar-ink)]">{user.initial || initials(user.name)}</AvatarFallback>
              </Avatar>
              {collapsed ? null : (
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-[12px] font-semibold text-[var(--color-sidebar-ink)]">{user.name}</span>
                  <span className="block truncate text-[10.5px] text-[var(--color-sidebar-muted)]">{user.email}</span>
                </span>
              )}
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent side="top" align="start" className="w-60">
            <DropdownMenuLabel>Signed in as {user.email}</DropdownMenuLabel>
            <DropdownMenuItem onClick={() => navigate("/dashboard/account")}>
              <UserRound /> Account
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onShowShortcuts}>
              <CircleHelp /> Keyboard shortcuts
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={signOut}>
              <LogOut /> Sign out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <div className={cn("mt-2 border-t border-[var(--color-sidebar-rule)] pt-2.5", collapsed && "flex justify-center")}>
          {collapsed ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <ThemeSwitch inverse iconOnly />
              </TooltipTrigger>
              <TooltipContent side="right">Dark mode</TooltipContent>
            </Tooltip>
          ) : (
            <ThemeSwitch inverse className="w-full justify-between px-1" />
          )}
        </div>
        {collapsed ? null : (
          <div className="mt-2 flex items-center gap-1">
            <button
              type="button"
              onClick={onShowShortcuts}
              className="flex items-center gap-1 rounded px-1.5 py-1 font-mono text-[9px] uppercase tracking-[0.08em] text-[var(--color-sidebar-muted)] transition-colors hover:text-[var(--color-sidebar-ink)]"
            >
              <kbd className="border-[var(--color-sidebar-rule)] bg-[var(--color-sidebar-2)] text-[var(--color-sidebar-muted)]">?</kbd>
              shortcuts
            </button>
            {onCollapsedChange ? (
              <button
                type="button"
                onClick={() => onCollapsedChange(!collapsed)}
                className="ml-auto rounded p-1.5 text-[var(--color-sidebar-muted)] transition-colors hover:bg-[var(--color-sidebar-2)] hover:text-[var(--color-sidebar-ink)]"
                aria-label="Collapse sidebar"
              >
                <ChevronsLeft className="size-3.5" />
              </button>
            ) : null}
          </div>
        )}
        {collapsed && onCollapsedChange ? (
          <button
            type="button"
            onClick={() => onCollapsedChange(false)}
            className="mx-auto mt-2 flex rounded p-1.5 text-[var(--color-sidebar-muted)] transition-colors hover:bg-[var(--color-sidebar-2)] hover:text-[var(--color-sidebar-ink)]"
            aria-label="Expand sidebar"
          >
            <ChevronsRight className="size-3.5" />
          </button>
        ) : null}
      </div>
    </div>
  );
}

export function Sidebar({
  session,
  collapsed,
  onCollapsedChange,
  onShowShortcuts,
}: {
  session: AppSession;
  collapsed: boolean;
  onCollapsedChange: (value: boolean) => void;
  onShowShortcuts: () => void;
}) {
  return (
    <aside
      className={cn(
        "fixed inset-y-0 left-0 z-40 hidden border-r border-[var(--color-sidebar-rule)] transition-[width] duration-200 ease-out lg:block",
        collapsed ? "w-[68px]" : "w-[248px]",
      )}
      aria-label="Workspace navigation"
    >
      <SidebarContent
        session={session}
        collapsed={collapsed}
        onCollapsedChange={onCollapsedChange}
        onShowShortcuts={onShowShortcuts}
      />
    </aside>
  );
}
