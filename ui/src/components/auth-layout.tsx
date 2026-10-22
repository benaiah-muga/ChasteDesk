// SPDX-License-Identifier: AGPL-3.0-only
import { Link } from "react-router-dom";
import { ArrowUpRight, FileSearch, ShieldCheck, Sparkles } from "lucide-react";
import { Brand, LogoMark } from "@/components/logo";
import { ThemeSwitch } from "@/components/theme-switch";
import { cn } from "@/lib/utils";

export function AuthLayout({
  children,
  eyebrow,
  title,
  description,
  wide = false,
}: {
  children: React.ReactNode;
  eyebrow: string;
  title: string;
  description: string;
  wide?: boolean;
}) {
  return (
    <div className="min-h-dvh bg-paper lg:grid lg:grid-cols-[minmax(0,1.04fr)_minmax(30rem,0.96fr)]">
      <aside className="relative hidden min-h-dvh overflow-hidden bg-[var(--color-sidebar)] px-12 py-10 text-[var(--color-sidebar-ink)] lg:flex lg:flex-col">
        <div className="absolute inset-x-12 top-20 h-px bg-[var(--color-sidebar-rule)]" aria-hidden="true" />
        <Link to="/" aria-label="ChasteDesk home" className="relative z-10 w-fit">
          <Brand onDark />
        </Link>
        <div className="relative z-10 my-auto max-w-lg py-20">
          <p className="eyebrow text-[var(--color-sidebar-muted)]">A support desk that shows its work</p>
          <h1 className="mt-5 font-display text-[clamp(3rem,4.2vw,4.2rem)] leading-[0.98] tracking-[-0.03em] text-[var(--color-sidebar-ink)]">
            The answer is only
            <br />
            as good as its
            <br />
            <span className="text-accent">evidence.</span>
          </h1>
          <p className="mt-6 max-w-md text-[14px] leading-6 text-[var(--color-sidebar-muted)]">
            ChasteDesk answers from the material you approved, cites the source, and stops before doing anything risky. Every handoff is complete enough to act on.
          </p>
          <div className="mt-10 space-y-0 border-y border-[var(--color-sidebar-rule)]">
            {[
              { icon: FileSearch, title: "Read the receipts", body: "Every AI reply carries the passage, source, and version it used." },
              { icon: ShieldCheck, title: "Keep the risky parts human", body: "Tools that change customer records wait for your approval." },
              { icon: Sparkles, title: "Leave a clean handoff", body: "When confidence runs out, the AI explains what it checked and what comes next." },
            ].map((item, index) => (
              <div key={item.title} className={cn("flex gap-3.5 py-4", index > 0 && "border-t border-[var(--color-sidebar-rule)]")}>
                <span className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md border border-[var(--color-sidebar-rule)] bg-[var(--color-sidebar-2)] text-accent">
                  <item.icon className="size-3.5" strokeWidth={1.75} />
                </span>
                <span>
                  <span className="block text-[12.5px] font-semibold text-[var(--color-sidebar-ink)]">{item.title}</span>
                  <span className="mt-0.5 block max-w-sm text-[11.5px] leading-5 text-[var(--color-sidebar-muted)]">{item.body}</span>
                </span>
              </div>
            ))}
          </div>
        </div>
        <div className="relative z-10 flex items-center justify-between font-mono text-[9px] uppercase tracking-[0.11em] text-[var(--color-sidebar-muted)]">
          <span>Self-hosted · AGPL-3.0-only</span>
          <span>ChasteDesk / 01</span>
        </div>
        <div className="pointer-events-none absolute -right-16 top-[18%] size-[26rem] rounded-full border border-[var(--color-sidebar-rule)] opacity-50" aria-hidden="true" />
        <div className="pointer-events-none absolute -right-2 top-[32%] size-[15rem] rounded-full border border-[var(--color-sidebar-rule)] opacity-40" aria-hidden="true" />
      </aside>

      <div className="flex min-h-dvh flex-col bg-paper">
        <header className="flex items-center justify-between px-5 py-5 sm:px-8 lg:justify-end lg:py-6">
          <Link to="/" className="lg:hidden" aria-label="ChasteDesk home">
            <span className="flex items-center gap-2.5">
              <LogoMark />
              <span className="font-display text-[22px] leading-none">ChasteDesk</span>
            </span>
          </Link>
          <div className="flex items-center gap-2">
            <a
              href="https://github.com/benaiah-muga/ChasteDesk#readme"
              target="_blank"
              rel="noreferrer noopener"
              className="hidden items-center gap-1.5 rounded-md px-2 py-1.5 text-[11px] font-medium text-muted-foreground transition-colors hover:bg-surface-3 hover:text-ink sm:flex"
            >
              Read the docs <ArrowUpRight className="size-3" />
            </a>
            <ThemeSwitch />
          </div>
        </header>

        <main className={cn("flex flex-1 items-center px-5 pb-16 pt-3 sm:px-8 lg:px-12 xl:px-20", wide && "items-start pt-8")}>
          <div className="mx-auto w-full max-w-[26rem] animate-[page-enter_260ms_var(--ease-out)_both]">
            <div className="mb-8">
              <p className="eyebrow">{eyebrow}</p>
              <h2 className="mt-2 font-display text-[clamp(2.3rem,4vw,2.85rem)] leading-[1.02] tracking-[-0.025em]">{title}</h2>
              <p className="mt-3 text-[13px] leading-5 text-muted-foreground">{description}</p>
            </div>
            {children}
          </div>
        </main>
        <footer className="px-5 pb-6 text-center font-mono text-[9px] uppercase tracking-[0.11em] text-subtle sm:px-8 lg:text-right">
          A quiet desk for loud support days.
        </footer>
      </div>
    </div>
  );
}

export function FieldError({ children }: { children: React.ReactNode }) {
  if (!children) return null;
  return <p className="mt-1.5 text-[11.5px] font-medium text-[var(--color-danger)]">{children}</p>;
}
