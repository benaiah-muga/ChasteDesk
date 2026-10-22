// SPDX-License-Identifier: AGPL-3.0-only
import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { copyText } from "@/lib/format";
import { cn } from "@/lib/utils";

export function CopyButton({
  value,
  label = "Copy",
  copiedLabel = "Copied",
  className,
  iconOnly = false,
  onCopied,
}: {
  value: string;
  label?: string;
  copiedLabel?: string;
  className?: string;
  iconOnly?: boolean;
  onCopied?: () => void;
}) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      className={cn(
        "inline-flex items-center gap-1.5 rounded-md text-[11px] font-semibold text-muted-foreground transition-colors hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        iconOnly ? "size-8 justify-center" : "h-8 border border-rule-strong bg-surface px-2.5",
        className,
      )}
      aria-label={copied ? copiedLabel : label}
      onClick={async () => {
        try {
          await copyText(value);
          setCopied(true);
          onCopied?.();
          window.setTimeout(() => setCopied(false), 1800);
        } catch {
          toast.error("Copy failed. Select the text and copy it manually.");
        }
      }}
    >
      {copied ? <Check className="size-3.5 text-[var(--color-success)]" /> : <Copy className="size-3.5" />}
      {iconOnly ? null : <span>{copied ? copiedLabel : label}</span>}
    </button>
  );
}
