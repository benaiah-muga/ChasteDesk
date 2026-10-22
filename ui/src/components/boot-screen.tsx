// SPDX-License-Identifier: AGPL-3.0-only
import { LogoMark } from "@/components/logo";
import { Skeleton } from "@/components/ui/skeleton";

export function BootScreen() {
  return (
    <div className="flex min-h-dvh items-center justify-center bg-paper px-6">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex items-center gap-3">
          <LogoMark className="size-9" />
          <div>
            <p className="font-display text-[26px] leading-none">ChasteDesk</p>
            <p className="eyebrow mt-1">Opening the desk</p>
          </div>
        </div>
        <div className="space-y-3">
          <Skeleton className="h-3 w-24" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-3 w-40" />
          <Skeleton className="h-10 w-full" />
        </div>
        <span className="sr-only" role="status">Loading ChasteDesk</span>
      </div>
    </div>
  );
}

export function PageSkeleton() {
  return (
    <div className="space-y-6" aria-busy="true" aria-label="Loading page">
      <div className="space-y-3 border-b border-rule pb-5">
        <Skeleton className="h-3 w-20" />
        <Skeleton className="h-10 w-64 max-w-full" />
        <Skeleton className="h-4 w-96 max-w-full" />
      </div>
      <div className="grid gap-4 md:grid-cols-3">
        <Skeleton className="h-32" />
        <Skeleton className="h-32" />
        <Skeleton className="h-32" />
      </div>
      <Skeleton className="h-72" />
    </div>
  );
}
