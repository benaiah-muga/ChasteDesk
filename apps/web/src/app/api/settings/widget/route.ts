// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";
import { z } from "zod";
import { getSessionCtx, db } from "@/lib/auth";
import { errorResponse, okResponse } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { getWidgetConfig } from "@/lib/repo";
import { writeAudit, widgetConfigs } from "@chastedesk/db";
import { eq } from "drizzle-orm";

export async function GET(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "settings.read");
    const config = await getWidgetConfig(authed);
    return okResponse(config ?? { origins: [], enabled: false, theme: {} });
  } catch (err) {
    return errorResponse(err);
  }
}

export async function PUT(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "settings.write");
    const input = z
      .object({ origins: z.array(z.string().min(1).max(200)).max(20), theme: z.record(z.string(), z.unknown()).optional() })
      .parse(await req.json());
    const existing = await getWidgetConfig(authed);
    if (existing) {
      await db.update(widgetConfigs).set({ origins: input.origins, theme: input.theme ?? existing.theme, updatedAt: new Date() }).where(eq(widgetConfigs.orgId, authed.orgId));
    } else {
      await db.insert(widgetConfigs).values({ orgId: authed.orgId, origins: input.origins, theme: input.theme ?? {} });
    }
    await writeAudit(authed, { actorType: "user", actorId: authed.userId, type: "settings.widget.updated", targetType: "widget_config", payload: { origins: input.origins } }, db);
    return okResponse(await getWidgetConfig(authed));
  } catch (err) {
    return errorResponse(err);
  }
}
