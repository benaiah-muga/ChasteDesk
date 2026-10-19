// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";

import { getSessionCtx } from "@/lib/auth";
import { db } from "@/lib/auth";
import { errorResponse, okResponse } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { listProviders } from "@/lib/repo";
import { providerConfigInput } from "@chastedesk/shared";
import { sealSecret, writeAudit, providerConfigs } from "@chastedesk/db";
import { eq, and } from "drizzle-orm";

export async function GET(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "settings.read");
    return okResponse(await listProviders(authed));
  } catch (err) {
    return errorResponse(err);
  }
}

export async function POST(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "settings.write");
    const input = providerConfigInput.parse(await req.json());
    const appSecret = process.env.APP_SECRET ?? "";
    const sealed = input.apiKey ? sealSecret(input.apiKey, appSecret) : null;
    const existing = await db
      .select({ id: providerConfigs.id })
      .from(providerConfigs)
      .where(and(eq(providerConfigs.orgId, authed.orgId), eq(providerConfigs.providerId, input.providerId)))
      .limit(1);
    const values = {
      displayName: input.displayName,
      baseUrl: input.baseUrl ?? null,
      ...(sealed
        ? { keyCipher: sealed.ciphertext, keyIv: sealed.iv, keyTag: sealed.authTag, keyVersion: sealed.keyVersion, keyLast4: sealed.last4 }
        : {}),
      enabled: true,
      updatedAt: new Date(),
    };
    if (existing[0]) {
      await db.update(providerConfigs).set(values).where(eq(providerConfigs.id, existing[0].id));
    } else {
      await db.insert(providerConfigs).values({ orgId: authed.orgId, providerId: input.providerId, ...values });
    }
    await writeAudit(authed, { actorType: "user", actorId: authed.userId, type: "settings.provider.saved", targetType: "provider_config", targetId: input.providerId, payload: { providerId: input.providerId, hasKey: Boolean(sealed) } }, db);
    return okResponse({ saved: true }, 201);
  } catch (err) {
    return errorResponse(err);
  }
}
