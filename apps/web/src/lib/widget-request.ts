// SPDX-License-Identifier: AGPL-3.0-only
import { db } from "@/lib/auth";
import { widgetConfigs } from "@chastedesk/db";
import { eq } from "drizzle-orm";
import { verifyWidgetTokenAnyOrg, originAllowed } from "./widget-token";

export interface WidgetAuthOk {
  ok: true;
  orgId: string;
  origins: string[];
  theme: Record<string, string>;
}

export interface WidgetAuthError {
  ok: false;
  status: number;
  code: string;
  message: string;
}

export async function authenticateWidget(req: Request, url: URL): Promise<WidgetAuthOk | WidgetAuthError> {
  const token = url.searchParams.get("token") ?? req.headers.get("x-widget-token") ?? "";
  const payload = verifyWidgetTokenAnyOrg(token);
  if (!payload) {
    return { ok: false, status: 401, code: "unauthorized", message: "invalid or expired widget token" };
  }
  const origin = req.headers.get("origin");
  if (!originAllowed(origin, payload.origins)) {
    return { ok: false, status: 403, code: "forbidden", message: "origin not allowed for this widget" };
  }
  const rows = await db.select().from(widgetConfigs).where(eq(widgetConfigs.orgId, payload.orgId)).limit(1);
  const config = rows[0];
  if (!config || !config.enabled) {
    return { ok: false, status: 404, code: "not_found", message: "widget disabled" };
  }
  return { ok: true, orgId: payload.orgId, origins: config.origins, theme: config.theme as Record<string, string> };
}
