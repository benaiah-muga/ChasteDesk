// SPDX-License-Identifier: AGPL-3.0-only
import { AlertTriangle, RotateCcw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ApiError } from "@/lib/api";

export function ErrorState({
  error,
  title = "That did not load",
  onRetry,
  compact = false,
}: {
  error: unknown;
  title?: string;
  onRetry?: () => void;
  compact?: boolean;
}) {
  const message =
    error instanceof ApiError
      ? error.status === 401
        ? "Your session has expired. Sign in again to continue."
        : error.message
      : error instanceof Error
        ? error.message
        : "Something went wrong while loading this view.";
  return (
    <div className={compact ? "flex flex-col items-start gap-3 py-6" : "flex min-h-72 flex-col items-center justify-center px-6 py-12 text-center"}>
      <span className="flex size-10 items-center justify-center rounded-full border border-[var(--color-danger-rule)] bg-[var(--color-danger-soft)] text-[var(--color-danger)]">
        <AlertTriangle className="size-4.5" />
      </span>
      <div className={compact ? "" : "mt-3"}>
        <h3 className="text-[15px] font-semibold text-ink">{title}</h3>
        <p className="mt-1 max-w-md text-[13px] leading-5 text-muted-foreground">{message}</p>
      </div>
      {onRetry ? (
        <Button variant="secondary" size="sm" className="mt-4" onClick={onRetry}>
          <RotateCcw />
          Try again
        </Button>
      ) : null}
    </div>
  );
}
