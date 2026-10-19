// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";
import { getSessionCtx, db } from "@/lib/auth";
import { errorResponse, okResponse} from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { auditEvents } from "@chastedesk/db";
import { and, desc, eq, ilike, or, type SQL } from "drizzle-orm";

export async function GET(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "audit.read");
    const url = new URL(req.url);
    const type = url.searchParams.get("type") ?? undefined;
    const q = url.searchParams.get("q");
    const limit = Math.min(Number(url.searchParams.get("limit") ?? "50") || 50, 200);
    const conditions: SQL[] = [eq(auditEvents.orgId, authed.orgId)];
    if (type) conditions.push(eq(auditEvents.type, type));
    if (q) {
      const like = `%${q}%`;
      const search = or(ilike(auditEvents.type, like), ilike(auditEvents.targetType, like));
      if (search) conditions.push(search);
    }
    const rows = await db
      .select({
        id: auditEvents.id,
        type: auditEvents.type,
        actorType: auditEvents.actorType,
        actorId: auditEvents.actorId,
        targetType: auditEvents.targetType,
        targetId: auditEvents.targetId,
        payload: auditEvents.payload,
        createdAt: auditEvents.createdAt,
      })
      .from(auditEvents)
      .where(and(...conditions))
      .orderBy(desc(auditEvents.id))
      .limit(limit);
    return okResponse(rows);
  } catch (err) {
    return errorResponse(err);
  }
}
