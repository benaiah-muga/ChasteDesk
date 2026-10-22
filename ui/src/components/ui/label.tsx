// SPDX-License-Identifier: AGPL-3.0-only
import * as React from "react";
import * as LabelPrimitive from "@radix-ui/react-label";
import { cn } from "@/lib/utils";

export const Label = React.forwardRef<
  React.ElementRef<typeof LabelPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof LabelPrimitive.Root>
>(({ className, ...props }, ref) => (
  <LabelPrimitive.Root
    ref={ref}
    className={cn("text-[12.5px] font-semibold text-ink-2 peer-disabled:opacity-60", className)}
    {...props}
  />
));
Label.displayName = "Label";
