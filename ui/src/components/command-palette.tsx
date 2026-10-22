// SPDX-License-Identifier: AGPL-3.0-only
import { useDeferredValue, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowRight,
  BadgeCheck,
  BarChart3,
  BookOpen,
  Bot,
  Building2,
  CircleHelp,
  Inbox,
  KeyRound,
  LayoutDashboard,
  LogOut,
  MessageSquare,
  MessageSquareText,
  Moon,
  Plus,
  ScrollText,
  Settings,
  Sun,
  UserPlus,
  UserRound,
  Users,
} from "lucide-react";
import { toast } from "sonner";
import type { AppSession } from "@/lib/types";
import { api } from "@/lib/api";
import { sessionKey } from "@/hooks/use-session";
import { useTheme } from "@/hooks/use-theme";
import { CommandDialog, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandSeparator, CommandShortcut } from "@/components/ui/command";
import { StatusDot } from "@/components/status-badge";
import { titleCase } from "@/lib/format";

interface CommandPaletteProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  session: AppSession;
  onShowShortcuts: () => void;
}

export function CommandPalette({ open, onOpenChange, session, onShowShortcuts }: CommandPaletteProps) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { effective, setChoice } = useTheme();
  const [query, setQuery] = useState("");
  const deferredQuery = useDeferredValue(query.trim());
  const searchEnabled = open && deferredQuery.length >= 2;
  const search = useQuery({
    queryKey: ["command-search", deferredQuery],
    queryFn: () => api.search(deferredQuery),
    enabled: searchEnabled,
    staleTime: 10_000,
  });

  useEffect(() => {
    if (!open) setQuery("");
  }, [open]);

  function run(action: () => void) {
    onOpenChange(false);
    window.setTimeout(action, 0);
  }

  function go(to: string) {
    run(() => navigate(to));
  }

  async function signOut() {
    try {
      await api.logout();
      queryClient.setQueryData(sessionKey, { authenticated: false, setupRequired: false });
      run(() => navigate("/login", { replace: true }));
    } catch {
      toast.error("Sign out failed");
    }
  }

  const permissions = session.permissions;
  const pages = [
    { label: "Overview", to: "/dashboard", icon: LayoutDashboard, hint: "G O" },
    { label: "Inbox", to: "/dashboard/inbox", icon: Inbox, hint: "G I" },
    { label: "Approvals", to: "/dashboard/approvals", icon: BadgeCheck },
    { label: "Customers", to: "/dashboard/customers", icon: Users, hint: "G C" },
    { label: "Knowledge sources", to: "/dashboard/knowledge", icon: BookOpen, hint: "G K" },
    { label: "Quick replies", to: "/dashboard/replies", icon: MessageSquareText },
    { label: "Analytics", to: "/dashboard/analytics", icon: BarChart3 },
    ...(permissions?.canAudit ? [{ label: "Audit log", to: "/dashboard/audit", icon: ScrollText }] : []),
    ...(permissions?.canSettings
      ? [
          { label: "Support agent", to: "/dashboard/settings/agent", icon: Bot },
          { label: "Settings", to: "/dashboard/settings/providers", icon: Settings },
        ]
      : []),
    { label: "Your account", to: "/dashboard/account", icon: UserRound },
  ];

  return (
    <CommandDialog open={open} onOpenChange={onOpenChange}>
      <CommandInput
        value={query}
        onValueChange={setQuery}
        placeholder="Search conversations, customers, sources, or jump to a page..."
        aria-label="Search and commands"
      />
      <CommandList>
        {searchEnabled && search.isFetching ? (
          <div className="border-b border-rule px-3 py-2 text-[11px] text-muted-foreground">Searching the desk...</div>
        ) : null}
        {searchEnabled && search.data ? (
          <>
            {search.data.conversations.length > 0 ? (
              <CommandGroup heading="Conversations">
                {search.data.conversations.map((item) => (
                  <CommandItem
                    key={item.id}
                    value={`conversation-${item.id}-${item.customer.name}`}
                    onSelect={() => go(`/dashboard/inbox/${item.id}`)}
                  >
                    <MessageSquare />
                    <span className="min-w-0 flex-1 truncate">
                      <span className="font-medium">{item.customer.name}</span>
                      <span className="ml-2 text-muted-foreground">{item.preview}</span>
                    </span>
                    <span className="flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
                      <StatusDot status={item.status} />
                      {titleCase(item.status)}
                    </span>
                  </CommandItem>
                ))}
              </CommandGroup>
            ) : null}
            {search.data.customers.length > 0 ? (
              <CommandGroup heading="Customers">
                {search.data.customers.map((item) => (
                  <CommandItem key={item.id} value={`customer-${item.id}-${item.name}`} onSelect={() => go(`/dashboard/customers/${item.id}`)}>
                    <Users />
                    <span className="min-w-0 flex-1 truncate">
                      <span className="font-medium">{item.name}</span>
                      <span className="ml-2 text-muted-foreground">{item.email}</span>
                    </span>
                    {item.open > 0 ? <span className="font-mono text-[10px] text-muted-foreground">{item.open} open</span> : null}
                  </CommandItem>
                ))}
              </CommandGroup>
            ) : null}
            {search.data.sources.length > 0 ? (
              <CommandGroup heading="Knowledge">
                {search.data.sources.map((item) => (
                  <CommandItem key={item.id} value={`source-${item.id}-${item.name}`} onSelect={() => go("/dashboard/knowledge")}>
                    <BookOpen />
                    <span className="min-w-0 flex-1 truncate">{item.name}</span>
                    <span className="font-mono text-[10px] text-muted-foreground">{titleCase(item.status)}</span>
                  </CommandItem>
                ))}
              </CommandGroup>
            ) : null}
            {search.data.conversations.length === 0 && search.data.customers.length === 0 && search.data.sources.length === 0 ? (
              <CommandEmpty>No conversations, customers, or sources match “{query.trim()}”.</CommandEmpty>
            ) : null}
            <CommandSeparator />
          </>
        ) : null}

        {!searchEnabled ? (
          <>
            <CommandGroup heading="Jump to">
              {pages.map((page) => (
                <CommandItem key={page.to} value={page.label} onSelect={() => go(page.to)}>
                  <page.icon />
                  {page.label}
                  {page.hint ? <CommandShortcut>{page.hint}</CommandShortcut> : null}
                </CommandItem>
              ))}
            </CommandGroup>
            <CommandSeparator />
            <CommandGroup heading="Quick actions">
              {permissions?.canKnowledgeWrite ? (
                <CommandItem value="add knowledge source website document" onSelect={() => go("/dashboard/knowledge?add=1")}>
                  <Plus />
                  Add a knowledge source
                  <CommandShortcut>N K</CommandShortcut>
                </CommandItem>
              ) : null}
              {permissions?.canSettings ? (
                <>
                  <CommandItem value="invite teammate member" onSelect={() => go("/dashboard/settings/workspace?invite=1")}>
                    <UserPlus />
                    Invite a teammate
                  </CommandItem>
                  <CommandItem value="create API key developer" onSelect={() => go("/dashboard/settings/api-keys?create=1")}>
                    <KeyRound />
                    Create an API key
                  </CommandItem>
                </>
              ) : null}
              <CommandItem
                value="toggle theme appearance dark light"
                onSelect={() => run(() => setChoice(effective === "dark" ? "light" : "dark"))}
              >
                {effective === "dark" ? <Sun /> : <Moon />}
                Switch to {effective === "dark" ? "light" : "dark"} theme
              </CommandItem>
            </CommandGroup>
          </>
        ) : null}

        <CommandSeparator />
        <CommandGroup heading="Help">
          <CommandItem value="keyboard shortcuts keys" onSelect={() => run(onShowShortcuts)}>
            <CircleHelp />
            Keyboard shortcuts
            <CommandShortcut>?</CommandShortcut>
          </CommandItem>
          <CommandItem value="docs documentation guide" onSelect={() => run(() => window.open("https://github.com/benaiah-muga/ChasteDesk#readme", "_blank", "noopener,noreferrer"))}>
            <Building2 />
            Open documentation
          </CommandItem>
          <CommandItem value="sign out log out" onSelect={signOut}>
            <LogOut />
            Sign out
          </CommandItem>
        </CommandGroup>
      </CommandList>
      <div className="flex items-center gap-4 border-t border-rule bg-surface-2 px-3 py-2 font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">
        <span className="flex items-center gap-1"><kbd>↑</kbd><kbd>↓</kbd> move</span>
        <span className="flex items-center gap-1"><kbd>↵</kbd> open</span>
        <span className="ml-auto flex items-center gap-1"><span className="normal-case tracking-normal">Search the desk</span><ArrowRight className="size-3" /></span>
      </div>
    </CommandDialog>
  );
}
