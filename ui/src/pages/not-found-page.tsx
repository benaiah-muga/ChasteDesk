// SPDX-License-Identifier: AGPL-3.0-only
import { Link } from "react-router-dom";
import { ArrowLeft } from "lucide-react";
import { LogoMark } from "@/components/logo";

export function NotFoundPage() {
  return (
    <div className="flex min-h-dvh flex-col bg-paper">
      <header className="flex items-center justify-between border-b border-rule px-5 py-4 sm:px-8">
        <Link to="/dashboard" className="flex items-center gap-2.5"><LogoMark /><span className="font-display text-[22px] leading-none">ChasteDesk</span></Link>
        <span className="font-mono text-[9px] uppercase tracking-[0.1em] text-subtle">404 / Off the desk</span>
      </header>
      <main className="mx-auto flex w-full max-w-3xl flex-1 flex-col justify-center px-5 py-16 sm:px-8">
        <p className="eyebrow">A quiet detour</p>
        <h1 className="mt-3 font-display text-[clamp(3rem,7vw,5.5rem)] leading-[0.95] tracking-[-0.035em]">This page is not on the desk.</h1>
        <p className="mt-5 max-w-lg text-[13.5px] leading-6 text-muted-foreground">The link may be old, the page may have moved, or the URL has a typo. Your inbox, decisions, and knowledge are still where you left them.</p>
        <div className="mt-7 flex flex-wrap gap-2">
          <Link to="/dashboard" className="inline-flex h-10 items-center gap-2 rounded-md bg-ink px-4 text-[13px] font-semibold text-paper hover:bg-[var(--color-ink-2)]"><ArrowLeft className="size-3.5" /> Back to overview</Link>
          <Link to="/dashboard/inbox" className="inline-flex h-10 items-center gap-2 rounded-md border border-rule-strong bg-surface px-4 text-[13px] font-semibold hover:bg-surface-2">Open inbox</Link>
        </div>
      </main>
      <footer className="border-t border-rule px-5 py-4 font-mono text-[9px] uppercase tracking-[0.1em] text-subtle sm:px-8">ChasteDesk / self-hosted support</footer>
    </div>
  );
}
