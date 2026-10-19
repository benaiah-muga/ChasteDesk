// SPDX-License-Identifier: AGPL-3.0-only
import { z } from "zod";
import { connection } from "next/server";

import { getSessionCtx, db, pool } from "@/lib/auth";
import { errorResponse, okResponse, AppError } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { writeAudit, providerConfigs } from "@chastedesk/db";
import { eq, and } from "drizzle-orm";
import { resolveProviderConfig, providerFor } from "@chastedesk/ai";
import type { ProviderId } from "@chastedesk/ai";

export async function POST(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "settings.write");
    const body = z.object({ configId: z.string().min(1) }).parse(await req.json());
    const id = body.configId;
    const rows = await db
      .select()
      .from(providerConfigs)
      .where(and(eq(providerConfigs.id, id), eq(providerConfigs.orgId, authed.orgId)))
      .limit(1);
    const row = rows[0];
    if (!row) throw new AppError("not_found", "provider config not found", 404);
    const resolved = await resolveProviderConfig(pool, process.env.APP_SECRET ?? "", authed.orgId, row.providerId as ProviderId);
    const provider = providerFor(row.providerId as ProviderId);
    try {
    await connection();
      const models = await provider.listModels(resolved.cfg);
      await db.update(providerConfigs).set({ modelsCache: models, updatedAt: new Date() }).where(eq(providerConfigs.id, id));
      await writeAudit(authed, { actorType: "user", actorId: authed.userId, type: "settings.provider.tested", targetType: "provider_config", targetId: id, payload: { ok: true, models: models.length } }, db);
      return okResponse({ ok: true, models });
    } catch (err) {
      await writeAudit(authed, { actorType: "user", actorId: authed.userId, type: "settings.provider.tested", targetType: "provider_config", targetId: id, payload: { ok: false, error: err instanceof Error ? err.message : "failed" } }, db);
      return okResponse({ ok: false, error: err instanceof Error ? err.message : "connectivity failed" });
    }
  } catch (err) {
    return errorResponse(err);
  }
}
