// SPDX-License-Identifier: AGPL-3.0-only
import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0].slice(0, 1).toUpperCase();
  return (parts[0].slice(0, 1) + parts[parts.length - 1].slice(0, 1)).toUpperCase();
}

export function plural(count: number, singular: string, pluralForm = singular + "s"): string {
  return `${count} ${count === 1 ? singular : pluralForm}`;
}

export function safeWindowOpen(url: string) {
  window.open(url, "_blank", "noopener,noreferrer");
}


const avatarTones = [
  "bg-[var(--avatar-sage)] text-[var(--avatar-ink)]",
  "bg-[var(--avatar-slate)] text-[var(--avatar-ink)]",
  "bg-[var(--avatar-clay)] text-[var(--avatar-ink)]",
  "bg-[var(--avatar-plum)] text-[var(--avatar-ink)]",
  "bg-[var(--avatar-sea)] text-[var(--avatar-ink)]",
];

export function avatarTone(name: string | null | undefined): string {
  const value = (name || "?").trim();
  let hash = 0;
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 31 + value.charCodeAt(index)) >>> 0;
  }
  return avatarTones[hash % avatarTones.length];
}
