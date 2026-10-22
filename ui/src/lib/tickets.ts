// SPDX-License-Identifier: AGPL-3.0-only
import type { TicketPriority } from "@/lib/types";

export const ticketPriorities: { value: TicketPriority; label: string; dot: string }[] = [
  { value: "urgent", label: "Urgent", dot: "bg-[var(--color-danger)]" },
  { value: "high", label: "High", dot: "bg-[var(--color-warn)]" },
  { value: "normal", label: "Normal", dot: "bg-[var(--color-info)]" },
  { value: "low", label: "Low", dot: "bg-[var(--color-faint)]" },
];

export function priorityMeta(priority: string | null | undefined) {
  return ticketPriorities.find((item) => item.value === priority) ?? ticketPriorities[2];
}

export function isSnoozed(snoozedUntil: string | null | undefined): boolean {
  return Boolean(snoozedUntil && new Date(snoozedUntil).getTime() > Date.now());
}

export function snoozeLabel(snoozedUntil: string | null | undefined): string {
  if (!isSnoozed(snoozedUntil)) return "Active now";
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(snoozedUntil!));
}

export function ticketTitle(ticketNumber: number): string {
  return `#${ticketNumber}`;
}

export function slaState(dueAt: string | null | undefined, completedAt?: string | null): { label: string; tone: "ok" | "warn" | "danger" | "muted" } {
  if (!dueAt) return { label: "Not set", tone: "muted" };
  const due = new Date(dueAt).getTime();
  if (completedAt) {
    return new Date(completedAt).getTime() <= due
      ? { label: "Met", tone: "ok" }
      : { label: "Missed", tone: "danger" };
  }
  const seconds = Math.round((due - Date.now()) / 1000);
  const span = (value: number) => {
    const minutes = Math.max(1, Math.round(Math.abs(value) / 60));
    if (minutes < 60) return `${minutes}m`;
    if (minutes < 1440) return `${Math.floor(minutes / 60)}h ${minutes % 60 === 0 ? "" : `${minutes % 60}m`}`.trim();
    return `${Math.floor(minutes / 1440)}d`;
  };
  if (seconds < 0) return { label: `Overdue ${span(seconds)}`, tone: "danger" };
  if (seconds < 3600) return { label: `Due in ${span(seconds)}`, tone: "warn" };
  return { label: `Due in ${span(seconds)}`, tone: "muted" };
}

export function snoozeChoices(): { label: string; until: Date }[] {
  const now = new Date();
  const inHours = (hours: number) => new Date(now.getTime() + hours * 3_600_000);
  const tomorrow = new Date(now);
  tomorrow.setDate(tomorrow.getDate() + 1);
  tomorrow.setHours(9, 0, 0, 0);
  const nextWeek = new Date(now);
  nextWeek.setDate(nextWeek.getDate() + 7);
  nextWeek.setHours(9, 0, 0, 0);
  return [
    { label: "In 1 hour", until: inHours(1) },
    { label: "In 4 hours", until: inHours(4) },
    { label: "Tomorrow at 9", until: tomorrow },
    { label: "Next week at 9", until: nextWeek },
  ];
}
