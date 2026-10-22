// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { sessionKey } from "./use-session";

export function useLiveUpdates(enabled: boolean) {
  const queryClient = useQueryClient();
  const lastToast = useRef(0);

  useEffect(() => {
    if (!enabled || !window.EventSource) return;
    const source = new EventSource("/api/conversations/stream");
    let pending = false;
    const refresh = () => {
      if (pending) return;
      pending = true;
      window.setTimeout(() => {
        pending = false;
        void queryClient.invalidateQueries({ queryKey: ["inbox"] });
        void queryClient.invalidateQueries({ queryKey: ["conversation"] });
        void queryClient.invalidateQueries({ queryKey: ["overview"] });
        void queryClient.invalidateQueries({ queryKey: sessionKey });
        void queryClient.invalidateQueries({ queryKey: ["approvals"] });
        if (Date.now() - lastToast.current > 20_000) {
          lastToast.current = Date.now();
          toast("New activity in the inbox", {
            description: "The conversation list and open thread were refreshed.",
            action: { label: "Open inbox", onClick: () => { window.location.href = "/dashboard/inbox"; } },
          });
        }
      }, 900);
    };
    source.addEventListener("message", refresh);
    source.addEventListener("error", () => {
      // EventSource reconnects on its own.
    });
    return () => source.close();
  }, [enabled, queryClient]);
}
