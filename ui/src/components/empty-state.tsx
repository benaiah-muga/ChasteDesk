// SPDX-License-Identifier: AGPL-3.0-only
import type { LucideIcon } from "lucide-react";
import { ArrowRight } from "lucide-react";
import { Link } from "react-router-dom";
import { cn } from "@/lib/utils";

export function EmptyState({
  icon: Icon,
  title,
  body,
  actionLabel,
  actionTo,
  onAction,
  secondary,
  className,
  compact = false,
}: {
  icon: LucideIcon;
  title: string;
  body: string;
  actionLabel?: string;
  actionTo?: string;
  onAction?: () => void;
  secondary?: React.ReactNode;
  className?: string;
  compact?: boolean;
}) {
  const action = actionLabel ? (
    actionTo ? (
      <Link
        to={actionTo}
        className="inline-flex h-9 items-center gap-2 rounded-md bg-ink px-3 text-[13px] font-semibold text-paper transition-colors hover:bg-[var(--color-ink-2)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
      >
        {actionLabel}
        <ArrowRight className="size-3.5" />
      </Link>
    ) : (
      <button
        type="button"
        onClick={onAction}
        className="inline-flex h-9 items-center gap-2 rounded-md bg-ink px-3 text-[13px] font-semibold text-paper transition-colors hover:bg-[var(--color-ink-2)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
      >
        {actionLabel}
        <ArrowRight className="size-3.5" />
      </button>
    )
  ) : null;
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center px-6 text-center",
        compact ? "py-8" : "min-h-72 py-14",
        className,
      )}
    >
      <span className="flex size-11 items-center justify-center rounded-full border border-rule bg-surface-2 text-muted-foreground">
        <Icon className="size-5" strokeWidth={1.6} />
      </span>
      <h3 className={cn("mt-4 font-display text-[24px] leading-tight text-ink", compact && "text-[21px]")}>{title}</h3>
      <p className="mt-1.5 max-w-sm text-[13px] leading-5 text-muted-foreground">{body}</p>
      {action || secondary ? <div className="mt-4 flex flex-wrap items-center justify-center gap-2">{action}{secondary}</div> : null}
    </div>
  );
}
