// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useState } from "react";
import { Clock3 } from "lucide-react";
import { cn } from "@/lib/utils";

function dateParts(date: Date) {
  const time = new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
    timeZoneName: "short",
  }).formatToParts(date);
  const value = (type: string) => time.find((part) => part.type === type)?.value ?? "";
  return {
    clock: `${value("hour")}:${value("minute")}`,
    zone: value("timeZoneName"),
    full: date.toLocaleString(undefined, { dateStyle: "full", timeStyle: "medium" }),
  };
}

export function TopbarClock({ className }: { className?: string }) {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 15_000);
    return () => window.clearInterval(timer);
  }, []);
  const parts = dateParts(now);
  return (
    <time
      dateTime={now.toISOString()}
      title={parts.full}
      aria-label={`Current time ${parts.clock} ${parts.zone}`}
      className={cn("flex h-8 shrink-0 items-center gap-1.5 rounded-md border border-[var(--color-sidebar-rule)] bg-[var(--color-sidebar)] px-2 font-mono text-[10px] tabular text-[var(--color-sidebar-ink)] shadow-[var(--shadow-whisper)] sm:px-2.5", className)}
    >
      <Clock3 className="size-3 text-[var(--color-sidebar-muted)]" aria-hidden="true" />
      <span className="text-[11px] font-semibold tracking-[0.01em]">{parts.clock}</span>
      <span className="hidden text-[8.5px] uppercase tracking-[0.06em] text-[var(--color-sidebar-muted)] sm:inline">{parts.zone}</span>
    </time>
  );
}
