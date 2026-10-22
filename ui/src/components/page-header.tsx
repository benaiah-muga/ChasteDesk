// SPDX-License-Identifier: AGPL-3.0-only
import { cn } from "@/lib/utils";

export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  meta,
  className,
}: {
  eyebrow?: React.ReactNode;
  title: React.ReactNode;
  description?: React.ReactNode;
  actions?: React.ReactNode;
  meta?: React.ReactNode;
  className?: string;
}) {
  return (
    <header className={cn("flex flex-col gap-5 border-b border-rule pb-5 md:flex-row md:items-end md:justify-between", className)}>
      <div className="min-w-0">
        {eyebrow ? <div className="eyebrow mb-2 flex flex-wrap items-center gap-2">{eyebrow}</div> : null}
        <h1 className="font-display text-[clamp(2rem,3vw,2.65rem)] leading-[1.04] tracking-[-0.025em] text-ink">{title}</h1>
        {description ? <p className="mt-2 max-w-2xl text-[13px] leading-5 text-muted-foreground">{description}</p> : null}
        {meta ? <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2">{meta}</div> : null}
      </div>
      {actions ? <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div> : null}
    </header>
  );
}
