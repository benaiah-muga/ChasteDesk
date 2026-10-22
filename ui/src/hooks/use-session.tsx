// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { AppSession } from "@/lib/types";

export const sessionKey = ["app-session"] as const;

export function useSession() {
  return useQuery({
    queryKey: sessionKey,
    queryFn: api.session,
    staleTime: 20_000,
    refetchOnMount: "always",
  });
}

export function useAuthedSession() {
  const query = useSession();
  const session = query.data;
  const ready = session?.authenticated === true && session.user && session.org && session.permissions;
  return { ...query, session, ready };
}

export function hasPermission(session: AppSession | undefined, permission: keyof NonNullable<AppSession["permissions"]>): boolean {
  return session?.permissions?.[permission] === true;
}
