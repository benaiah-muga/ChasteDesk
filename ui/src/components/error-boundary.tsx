// SPDX-License-Identifier: AGPL-3.0-only
import { Component, type ReactNode } from "react";
import { Button } from "@/components/ui/button";

export class AppErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state = { error: null as Error | null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex min-h-dvh items-center justify-center bg-paper px-6">
          <div className="max-w-md text-center">
            <p className="eyebrow">Unexpected stop</p>
            <h1 className="mt-2 font-display text-[38px] leading-none">The console hit a snag.</h1>
            <p className="mt-3 text-[12.5px] leading-5 text-muted-foreground">A view failed to render. Your data is safe. Reload the console to continue.</p>
            <pre className="mt-4 max-h-32 overflow-auto rounded-md border border-rule bg-surface-2 p-3 text-left font-mono text-[10px] leading-5 text-muted-foreground">{this.state.error.message}</pre>
            <Button className="mt-5" onClick={() => window.location.reload()}>Reload console</Button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
