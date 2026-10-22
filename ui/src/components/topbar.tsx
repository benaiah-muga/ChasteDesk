// SPDX-License-Identifier: AGPL-3.0-only
import { useLocation, useNavigate } from "react-router-dom";
import {
  Bell,
  ChevronDown,
  CircleHelp,
  ExternalLink,
  Keyboard,
  LogOut,
  Menu,
  Monitor,
  Moon,
  Plus,
  Search,
  Settings,
  Sun,
  UserPlus,
  UserRound,
  KeyRound,
  BookOpen,
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/lib/api";
import type { AppSession } from "@/lib/types";
import { sessionKey } from "@/hooks/use-session";
import { useTheme, type ThemeChoice } from "@/hooks/use-theme";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { navGroups } from "@/components/sidebar";
import { titleCase } from "@/lib/format";
import { avatarTone } from "@/lib/utils";
import { TopbarClock } from "@/components/clock";

function crumbFor(pathname: string): string {
  if (pathname === "/dashboard") return "Overview";
  if (pathname.startsWith("/dashboard/inbox")) return "Inbox";
  if (pathname.startsWith("/dashboard/approvals")) return "Approvals";
  if (pathname.startsWith("/dashboard/customers")) return "Customers";
  if (pathname.startsWith("/dashboard/knowledge")) return "Knowledge";
  if (pathname.startsWith("/dashboard/replies")) return "Quick replies";
  if (pathname.startsWith("/dashboard/analytics")) return "Analytics";
  if (pathname.startsWith("/dashboard/audit")) return "Audit log";
  if (pathname.startsWith("/dashboard/settings/agent")) return "Support agent";
  if (pathname.startsWith("/dashboard/settings")) return "Settings";
  if (pathname.startsWith("/dashboard/account")) return "Account";
  return "Desk";
}

interface TopbarProps {
  session: AppSession;
  onOpenNav: () => void;
  onOpenCommand: () => void;
  onShowShortcuts: () => void;
}

export function Topbar({ session, onOpenNav, onOpenCommand, onShowShortcuts }: TopbarProps) {
  const location = useLocation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { choice, setChoice } = useTheme();
  const permissions = session.permissions;
  const counts = session.counts;
  const attentionCount = (counts?.awaitingHuman ?? 0) + (counts?.pendingApprovals ?? 0) + (counts?.failedSources ?? 0);

  async function signOut() {
    try {
      await api.logout();
      queryClient.setQueryData(sessionKey, { authenticated: false, setupRequired: false });
      navigate("/login", { replace: true });
    } catch {
      toast.error("Sign out failed. Try again.");
    }
  }

  const attention = [
    counts?.pendingApprovals
      ? { key: "approvals", label: `${counts.pendingApprovals} ${counts.pendingApprovals === 1 ? "approval is" : "approvals are"} waiting`, detail: "Review the action before it runs.", to: "/dashboard/approvals", tone: "warn" }
      : null,
    counts?.awaitingHuman
      ? { key: "human", label: `${counts.awaitingHuman} ${counts.awaitingHuman === 1 ? "conversation needs" : "conversations need"} a human`, detail: "The AI left a handoff and stepped back.", to: "/dashboard/inbox?status=awaiting_human", tone: "danger" }
      : null,
    counts?.failedSources
      ? { key: "sources", label: `${counts.failedSources} ${counts.failedSources === 1 ? "source failed" : "sources failed"} to index`, detail: "Inspect the error and retry.", to: "/dashboard/knowledge?status=failed", tone: "danger" }
      : null,
  ].filter(Boolean) as { key: string; label: string; detail: string; to: string; tone: string }[];

  const nav = navGroups(session);
  const section = nav.flatMap((group) => group.items).find((item) => {
    if (item.to === "/dashboard") return location.pathname === "/dashboard";
    return location.pathname.startsWith(item.to);
  });

  return (
    <header className="sticky top-0 z-30 flex h-14 shrink-0 items-center gap-2 border-b border-rule bg-[color-mix(in_oklab,var(--color-paper)_94%,transparent)] px-3 sm:px-5">
      <button
        type="button"
        onClick={onOpenNav}
        className="flex size-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-surface-3 hover:text-ink lg:hidden"
        aria-label="Open navigation"
      >
        <Menu className="size-4.5" />
      </button>
      <nav className="hidden min-w-0 items-center gap-2 text-[12px] md:flex" aria-label="Breadcrumb">
        <span className="text-muted-foreground">Desk</span>
        <span className="text-subtle">/</span>
        <span className="truncate font-medium text-ink">{crumbFor(location.pathname)}</span>
        {section && section.to !== "/dashboard" && section.label !== crumbFor(location.pathname) ? (
          <>
            <span className="text-subtle">/</span>
            <span className="truncate text-muted-foreground">{section.label}</span>
          </>
        ) : null}
      </nav>
      <TopbarClock className="shrink-0" />
      <div className="flex flex-1 items-center justify-end gap-1.5">
        <button
          type="button"
          onClick={onOpenCommand}
          className="flex h-8 items-center gap-2 rounded-md border border-rule bg-surface px-2.5 text-left text-[12px] text-muted-foreground transition-colors hover:border-rule-strong hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:min-w-[220px]"
          aria-label="Search and jump, Command K"
        >
          <Search className="size-3.5 shrink-0" />
          <span className="hidden flex-1 sm:inline">Search or jump to...</span>
          <kbd className="hidden sm:inline-flex">⌘K</kbd>
        </button>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="secondary" size="sm" className="hidden sm:inline-flex" aria-label="Create or invite">
              <Plus />
              New
              <ChevronDown className="size-3 text-muted-foreground" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-60">
            <DropdownMenuLabel>Create</DropdownMenuLabel>
            {permissions?.canKnowledgeWrite ? (
              <DropdownMenuItem onClick={() => navigate("/dashboard/knowledge?add=1")}>
                <BookOpen /> Add knowledge
              </DropdownMenuItem>
            ) : null}
            {permissions?.canSettings ? (
              <>
                <DropdownMenuItem onClick={() => navigate("/dashboard/settings/workspace?invite=1")}>
                  <UserPlus /> Invite a teammate
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => navigate("/dashboard/settings/api-keys?create=1")}>
                  <KeyRound /> Create an API key
                </DropdownMenuItem>
              </>
            ) : null}
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={onShowShortcuts}>
              <Keyboard /> Keyboard shortcuts
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <Popover>
          <PopoverTrigger asChild>
            <button
              type="button"
              className="relative flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-surface-3 hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-label={attentionCount > 0 ? `${attentionCount} items need attention` : "Nothing needs attention"}
            >
              <Bell className="size-4" />
              {attentionCount > 0 ? <span className="absolute right-1.5 top-1.5 size-1.5 rounded-full bg-accent ring-2 ring-paper" /> : null}
            </button>
          </PopoverTrigger>
          <PopoverContent align="end" className="w-80 p-0">
            <div className="flex items-center justify-between border-b border-rule px-3.5 py-2.5">
              <span className="eyebrow">Needs attention</span>
              <span className="font-mono text-[10px] text-muted-foreground">{attentionCount} open</span>
            </div>
            {attention.length ? (
              <div className="divide-y divide-rule">
                {attention.map((item) => (
                  <button
                    key={item.key}
                    type="button"
                    onClick={() => navigate(item.to)}
                    className="flex w-full items-start gap-3 px-3.5 py-3 text-left transition-colors hover:bg-surface-2"
                  >
                    <span className={`mt-1.5 size-1.5 shrink-0 rounded-full ${item.tone === "danger" ? "bg-[var(--color-danger)]" : "bg-[var(--color-warn)]"}`} />
                    <span>
                      <span className="block text-[12.5px] font-semibold text-ink">{item.label}</span>
                      <span className="mt-0.5 block text-[11.5px] leading-4 text-muted-foreground">{item.detail}</span>
                    </span>
                  </button>
                ))}
              </div>
            ) : (
              <div className="px-3.5 py-7 text-center">
                <p className="font-display text-[22px]">The desk is clear.</p>
                <p className="mt-1 text-[12px] text-muted-foreground">No approvals or handoffs are waiting.</p>
              </div>
            )}
            <div className="border-t border-rule px-3.5 py-2">
              <button type="button" onClick={() => navigate("/dashboard/inbox?status=awaiting_human")} className="text-[11.5px] font-semibold text-accent-ink hover:underline">
                Open the attention queue
              </button>
            </div>
          </PopoverContent>
        </Popover>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className="flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-surface-3 hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-label="Change theme"
            >
              {choice === "dark" ? <Moon className="size-4" /> : choice === "light" ? <Sun className="size-4" /> : <Monitor className="size-4" />}
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuLabel>Appearance</DropdownMenuLabel>
            <DropdownMenuRadioGroup value={choice} onValueChange={(value) => setChoice(value as ThemeChoice)}>
              <DropdownMenuRadioItem value="light"><Sun className="mr-2 size-4" /> Light</DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="dark"><Moon className="mr-2 size-4" /> Dark</DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="system"><Monitor className="mr-2 size-4" /> System</DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className="hidden size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-surface-3 hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:flex"
              aria-label="Help and account"
            >
              <CircleHelp className="size-4" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuLabel>{session.user?.email}</DropdownMenuLabel>
            <DropdownMenuItem onClick={onShowShortcuts}><Keyboard /> Keyboard shortcuts</DropdownMenuItem>
            <DropdownMenuItem onClick={() => window.open("https://github.com/benaiah-muga/ChasteDesk#readme", "_blank", "noopener,noreferrer")}><ExternalLink /> Documentation</DropdownMenuItem>
            <DropdownMenuItem onClick={() => navigate("/dashboard/account")}><UserRound /> Account</DropdownMenuItem>
            {permissions?.canSettings ? (
              <DropdownMenuItem onClick={() => navigate("/dashboard/settings/providers")}><Settings /> Settings</DropdownMenuItem>
            ) : null}
            <DropdownMenuSeparator />
            <DropdownMenuItem destructive onClick={signOut}><LogOut /> Sign out</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              onClick={() => navigate("/dashboard/account")}
              className="ml-0.5 flex size-8 items-center justify-center rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-label="Open account"
            >
              <Avatar className="size-7">
                <AvatarFallback className={avatarTone(session.user?.name)}>{session.user?.initial}</AvatarFallback>
              </Avatar>
            </button>
          </TooltipTrigger>
          <TooltipContent side="bottom">{session.user?.name} · {titleCase(session.user?.role ?? "")}</TooltipContent>
        </Tooltip>
      </div>
    </header>
  );
}
