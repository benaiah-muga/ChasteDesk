// SPDX-License-Identifier: AGPL-3.0-only
import { titleCase } from "@/lib/format";
import { cn } from "@/lib/utils";

export function StatusBadge({
  status,
  label,
  className,
  dot = true,
}: {
  status: string;
  label?: string;
  className?: string;
  dot?: boolean;
}) {
  return (
    <span
      className={cn(
        "inline-flex h-5 w-fit items-center gap-1.5 rounded-full border border-rule bg-surface/80 px-2 text-[10px] font-medium leading-none text-ink-2 whitespace-nowrap",
        className,
      )}
    >
      {dot ? <span className={cn("size-1.5 rounded-full", statusDotClass(status))} aria-hidden="true" /> : null}
      {label ?? titleCase(status)}
    </span>
  );
}

export function statusDotClass(status: string): string {
  switch (status) {
    case "resolved":
    case "ready":
    case "executed":
    case "approved":
    case "success":
      return "bg-[var(--color-success)]";
    case "closed":
      return "bg-[var(--color-muted)]";
    case "awaiting_human":
    case "pending_approval":
    case "pending":
    case "indexing":
    case "fetching":
    case "warn":
      return "bg-[var(--color-warn)]";
    case "failed":
    case "rejected":
    case "rejected_before_exec":
    case "danger":
      return "bg-[var(--color-danger)]";
    case "human_active":
      return "bg-[var(--color-accent)]";
    case "ai_active":
      return "bg-[var(--color-info)]";
    default:
      return "bg-[var(--color-info)]";
  }
}

export function StatusDot({ status, className }: { status: string; className?: string }) {
  return <span className={cn("size-1.5 rounded-full", statusDotClass(status), className)} aria-hidden="true" />;
}
