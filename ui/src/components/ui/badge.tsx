// SPDX-License-Identifier: AGPL-3.0-only
import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex h-5 w-fit items-center gap-1 rounded-md border px-1.5 font-mono text-[9px] font-medium uppercase tracking-[0.06em] whitespace-nowrap",
  {
    variants: {
      variant: {
        default: "border-rule bg-surface-2/65 text-muted-foreground",
        outline: "border-rule bg-transparent text-muted-foreground",
        success: "border-[var(--color-success-rule)] bg-transparent text-[var(--color-success)]",
        warn: "border-[var(--color-warn-rule)] bg-transparent text-[var(--color-warn)]",
        danger: "border-[var(--color-danger-rule)] bg-transparent text-[var(--color-danger)]",
        info: "border-[var(--color-info-rule)] bg-transparent text-[var(--color-info)]",
        accent: "border-[var(--color-accent-soft)] bg-transparent text-[var(--color-accent-ink)]",
        ink: "border-ink bg-ink text-paper",
        internal: "border-[var(--color-info-rule)] bg-transparent text-[var(--color-info)]",
      },
    },
    defaultVariants: { variant: "default" },
  },
);

export interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement>, VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />;
}
