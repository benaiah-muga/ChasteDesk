// SPDX-License-Identifier: AGPL-3.0-only
import { cn } from "@/lib/utils";
import { priorityMeta } from "@/lib/tickets";

export function PriorityBadge({
  priority,
  className,
  muted = false,
  compact = false,
}: {
  priority: string;
  className?: string;
  muted?: boolean;
  compact?: boolean;
}) {
  if (muted && priority === "normal") return null;
  const meta = priorityMeta(priority);
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border border-rule bg-surface font-mono font-medium uppercase tracking-[0.06em] text-ink-2",
        compact ? "h-4 px-1.5 text-[8px]" : "h-5 px-2 text-[8.5px]",
        className,
      )}
    >
      <span className={cn("size-1.5 rounded-full", meta.dot)} aria-hidden="true" />
      {meta.label}
    </span>
  );
}
