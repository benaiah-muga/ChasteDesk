// SPDX-License-Identifier: AGPL-3.0-only
import { cn } from "@/lib/utils";

export function LogoMark({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex size-9 shrink-0 overflow-hidden rounded-[10px] bg-[#080706] shadow-[inset_0_0_0_1px_oklch(100%_0_0_/_0.14)]",
        className,
      )}
      aria-hidden="true"
    >
      <img src="/static/favicon-192.png" alt="" width="32" height="32" className="size-full object-cover" />
    </span>
  );
}

export function Brand({ compact = false, onDark = false, className }: { compact?: boolean; onDark?: boolean; className?: string }) {
  return (
    <span className={cn("flex items-center gap-2.5", className)}>
      <LogoMark />
      {compact ? null : (
        <span className="flex flex-col">
          <span className={cn("font-display text-[22px] leading-none tracking-[-0.02em]", onDark ? "text-[var(--color-sidebar-ink)]" : "text-ink")}>
            ChasteDesk
          </span>
          <span className={cn("mt-0.5 font-mono text-[8.5px] font-semibold uppercase tracking-[0.18em]", onDark ? "text-[var(--color-sidebar-muted)]" : "text-subtle")}>
            Support desk
          </span>
        </span>
      )}
    </span>
  );
}
