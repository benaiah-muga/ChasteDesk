// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";
import { getSessionCtx, db } from "@/lib/auth";
import { errorResponse, okResponse} from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { analyticsDaily } from "@chastedesk/db";
import { and, asc, eq, gte } from "drizzle-orm";

export async function GET(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "inbox.use");
    const url = new URL(req.url);
    const days = Math.min(Math.max(Number(url.searchParams.get("days") ?? "14") || 14, 1), 90);
    const since = new Date(Date.now() - (days - 1) * 86400000).toISOString().slice(0, 10);
    const rows = await db
      .select()
      .from(analyticsDaily)
      .where(and(eq(analyticsDaily.orgId, authed.orgId), gte(analyticsDaily.date, since)))
      .orderBy(asc(analyticsDaily.date));
    const latest = rows[rows.length - 1]?.metrics ?? null;
    return okResponse({ metrics: latest, trend: rows.map((r) => ({ date: r.date, metrics: r.metrics })) });
  } catch (err) {
    return errorResponse(err);
  }
}
