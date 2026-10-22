// SPDX-License-Identifier: AGPL-3.0-only
import { Suspense, useEffect, useState } from "react";
import { Outlet, useLocation, useNavigate } from "react-router-dom";
import { cn } from "@/lib/utils";
import { useAuthedSession } from "@/hooks/use-session";
import { useLiveUpdates } from "@/hooks/use-live-updates";
import { BootScreen, PageSkeleton } from "@/components/boot-screen";
import { ErrorState } from "@/components/error-state";
import { Sidebar, SidebarContent } from "@/components/sidebar";
import { Topbar } from "@/components/topbar";
import { CommandPalette } from "@/components/command-palette";
import { ShortcutsDialog } from "@/components/shortcuts-dialog";
import { Sheet, SheetContent } from "@/components/ui/sheet";
import { TooltipProvider } from "@/components/ui/tooltip";

export function AppShell() {
  const { data: session, isLoading, error, refetch } = useAuthedSession();
  const location = useLocation();
  const navigate = useNavigate();
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem("chastedesk-sidebar") === "collapsed";
    } catch {
      return false;
    }
  });
  const [mobileNav, setMobileNav] = useState(false);
  const [commandOpen, setCommandOpen] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);

  useLiveUpdates(Boolean(session?.authenticated));

  useEffect(() => {
    setMobileNav(false);
    window.scrollTo({ top: 0, behavior: "auto" });
  }, [location.pathname]);

  useEffect(() => {
    try {
      localStorage.setItem("chastedesk-sidebar", collapsed ? "collapsed" : "expanded");
    } catch {
      // Sidebar preference is optional.
    }
  }, [collapsed]);

  useEffect(() => {
    let goPrefix = false;
    let goTimer = 0;
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const typing = target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.tagName === "SELECT" || target.isContentEditable);
      const modalOpen = Boolean(target?.closest('[role="dialog"], [role="alertdialog"]'));
      const mod = event.metaKey || event.ctrlKey;
      if (modalOpen) return;
      if (mod && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setCommandOpen(true);
        return;
      }
      if (mod && event.key.toLowerCase() === "b") {
        event.preventDefault();
        setCollapsed((value) => !value);
        return;
      }
      if (typing || mod || event.altKey) return;
      if (event.key === "?") {
        event.preventDefault();
        setShortcutsOpen(true);
        return;
      }
      if (event.key.toLowerCase() === "g") {
        goPrefix = true;
        window.clearTimeout(goTimer);
        goTimer = window.setTimeout(() => { goPrefix = false; }, 1200);
        return;
      }
      if (goPrefix) {
        const routes: Record<string, string> = {
          o: "/dashboard",
          d: "/dashboard",
          i: "/dashboard/inbox",
          a: "/dashboard/approvals",
          c: "/dashboard/customers",
          k: "/dashboard/knowledge",
          s: "/dashboard/settings/providers",
        };
        const route = routes[event.key.toLowerCase()];
        if (route) {
          event.preventDefault();
          goPrefix = false;
          navigate(route);
        }
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
      window.clearTimeout(goTimer);
    };
  }, [navigate]);

  if (isLoading) return <BootScreen />;
  if (error || !session?.authenticated || !session.user || !session.org) {
    return (
      <div className="flex min-h-dvh items-center justify-center bg-paper p-6">
        <ErrorState error={error} title="The desk could not open" onRetry={() => void refetch()} />
      </div>
    );
  }

  const inboxRoute = location.pathname.startsWith("/dashboard/inbox");

  return (
    <TooltipProvider delayDuration={250}>
      <a href="#main-content" className="sr-only z-[100] rounded-md bg-ink px-3 py-2 text-paper focus:not-sr-only focus:fixed focus:left-4 focus:top-4">
        Skip to content
      </a>
      <Sidebar session={session} collapsed={collapsed} onCollapsedChange={setCollapsed} onShowShortcuts={() => setShortcutsOpen(true)} />
      <div className={cn("min-h-dvh transition-[padding] duration-200 ease-out", collapsed ? "lg:pl-[68px]" : "lg:pl-[248px]")}>
        <Topbar
          session={session}
          onOpenNav={() => setMobileNav(true)}
          onOpenCommand={() => setCommandOpen(true)}
          onShowShortcuts={() => setShortcutsOpen(true)}
        />
        <main
          id="main-content"
          className={cn(
            "min-h-[calc(100dvh-3.5rem)]",
            inboxRoute ? "p-0" : "mx-auto w-full max-w-[1520px] px-4 pb-16 pt-6 sm:px-6 lg:px-8",
          )}
        >
          <Suspense fallback={<PageSkeleton />}>
            <Outlet context={{ session }} />
          </Suspense>
        </main>
      </div>
      <Sheet open={mobileNav} onOpenChange={setMobileNav}>
        <SheetContent side="left" hideClose className="w-[min(88vw,19rem)] border-r-0 p-0">
          <SidebarContent session={session} mobile onClose={() => setMobileNav(false)} onNavigate={() => setMobileNav(false)} onShowShortcuts={() => { setMobileNav(false); setShortcutsOpen(true); }} />
        </SheetContent>
      </Sheet>
      <CommandPalette open={commandOpen} onOpenChange={setCommandOpen} session={session} onShowShortcuts={() => setShortcutsOpen(true)} />
      <ShortcutsDialog open={shortcutsOpen} onOpenChange={setShortcutsOpen} />
    </TooltipProvider>
  );
}
