// SPDX-License-Identifier: AGPL-3.0-only
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { QueryClientProvider } from "@tanstack/react-query";
import { Toaster } from "sonner";
import { App } from "./app";
import { AppErrorBoundary } from "@/components/error-boundary";
import { queryClient } from "@/lib/query";
import { ThemeProvider, useTheme } from "@/hooks/use-theme";
import "./styles.css";

function ThemedApp() {
  const { effective } = useTheme();
  return (
    <AppErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <App />
          <Toaster
            theme={effective}
            position="bottom-right"
            closeButton
            toastOptions={{
              classNames: {
                toast: "!rounded-lg !border !border-rule !bg-surface !text-ink !shadow-[var(--shadow-float)] !font-sans",
                title: "!text-[12.5px] !font-semibold !text-ink",
                description: "!text-[11.5px] !leading-5 !text-muted-foreground",
                actionButton: "!bg-ink !text-paper !rounded !text-[11px]",
                cancelButton: "!bg-surface-2 !text-ink !rounded !text-[11px]",
                closeButton: "!border-rule !bg-surface !text-muted-foreground",
              },
            }}
          />
        </BrowserRouter>
      </QueryClientProvider>
    </AppErrorBoundary>
  );
}

function Root() {
  return (
    <ThemeProvider>
      <ThemedApp />
    </ThemeProvider>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <Root />
  </StrictMode>,
);
