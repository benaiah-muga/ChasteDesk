// SPDX-License-Identifier: AGPL-3.0-only
import { Moon, Sun } from "lucide-react";
import { useTheme } from "@/hooks/use-theme";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";

export function ThemeSwitch({
  inverse = false,
  className,
  iconOnly = false,
  label = "Dark mode",
}: {
  inverse?: boolean;
  className?: string;
  iconOnly?: boolean;
  label?: string;
}) {
  const { effective, setChoice } = useTheme();
  const dark = effective === "dark";
  return (
    <label
      className={cn(
        "inline-flex cursor-pointer select-none items-center gap-2",
        iconOnly && "justify-center",
        inverse ? "text-[var(--color-sidebar-muted)]" : "text-muted-foreground",
        className,
      )}
    >
      {dark ? <Moon className="size-3.5 shrink-0" aria-hidden="true" /> : <Sun className="size-3.5 shrink-0" aria-hidden="true" />}
      {iconOnly ? null : <span className={cn("text-[11px] font-medium", inverse ? "text-[var(--color-sidebar-muted)]" : "text-muted-foreground")}>{label}</span>}
      <Switch
        checked={dark}
        onCheckedChange={(checked) => setChoice(checked ? "dark" : "light")}
        aria-label={label}
        className={cn(inverse && "data-[state=checked]:bg-[var(--color-accent)] data-[state=unchecked]:bg-[var(--color-sidebar-3)]")}
      />
    </label>
  );
}
