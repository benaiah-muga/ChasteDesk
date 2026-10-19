// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";

import { getSessionCtx, db, pool } from "@/lib/auth";
import { errorResponse, okResponse, AppError } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { providerConfigs } from "@chastedesk/db";
import { eq, and } from "drizzle-orm";
import { resolveProviderConfig, providerFor } from "@chastedesk/ai";
import type { ProviderId } from "@chastedesk/ai";

export async function GET(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "settings.read");
    const configId = new URL(req.url).searchParams.get("configId");
    if (!configId) throw new AppError("bad_request", "configId is required", 400);
    const rows = await db
      .select()
      .from(providerConfigs)
      .where(and(eq(providerConfigs.id, configId), eq(providerConfigs.orgId, authed.orgId)))
      .limit(1);
    const row = rows[0];
    if (!row || !row.enabled) throw new AppError("not_found", "provider config not found", 404);
    const resolved = await resolveProviderConfig(pool, process.env.APP_SECRET ?? "", authed.orgId, row.providerId as ProviderId);
    const provider = providerFor(row.providerId as ProviderId);
    const models = await provider.listModels(resolved.cfg);
    await db.update(providerConfigs).set({ modelsCache: models, updatedAt: new Date() }).where(eq(providerConfigs.id, configId));
    return okResponse(models);
  } catch (err) {
    return errorResponse(err);
  }
}
