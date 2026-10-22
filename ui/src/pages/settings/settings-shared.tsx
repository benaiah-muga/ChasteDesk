// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from "@/components/ui/button";

export function SectionHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description: string;
  actions?: React.ReactNode;
}) {
  return (
    <header className="flex flex-col gap-3 border-b border-rule pb-4 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <h2 className="font-display text-[30px] leading-none tracking-[-0.02em]">{title}</h2>
        <p className="mt-1.5 max-w-2xl text-[12.5px] leading-5 text-muted-foreground">{description}</p>
      </div>
      {actions ? <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div> : null}
    </header>
  );
}

export function SaveBar({
  dirty,
  saving,
  onSave,
  onDiscard,
  label = "Save changes",
}: {
  dirty: boolean;
  saving: boolean;
  onSave: () => void;
  onDiscard: () => void;
  label?: string;
}) {
  if (!dirty) return null;
  return (
    <div className="sticky bottom-3 z-20 mt-4 flex items-center justify-between gap-3 rounded-lg border border-rule-strong bg-surface px-3.5 py-2.5 shadow-[var(--shadow-float)]">
      <p className="text-[11.5px] text-muted-foreground">You have unsaved changes.</p>
      <div className="flex items-center gap-2">
        <Button type="button" variant="ghost" size="sm" onClick={onDiscard} disabled={saving}>Discard</Button>
        <Button type="button" size="sm" onClick={onSave} loading={saving}>{label}</Button>
      </div>
    </div>
  );
}
