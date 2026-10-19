// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";
import { getSessionCtx, db } from "@/lib/auth";
import { errorResponse, okResponse } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { actions } from "@chastedesk/db";
import { and, desc, eq } from "drizzle-orm";

export async function GET(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "inbox.use");
    const status = new URL(req.url).searchParams.get("status") ?? undefined;
    const rows = await db
      .select()
      .from(actions)
      .where(status ? and(eq(actions.orgId, authed.orgId), eq(actions.status, status as never)) : eq(actions.orgId, authed.orgId))
      .orderBy(desc(actions.createdAt))
      .limit(50);
    return okResponse(rows);
  } catch (err) {
    return errorResponse(err);
  }
}
