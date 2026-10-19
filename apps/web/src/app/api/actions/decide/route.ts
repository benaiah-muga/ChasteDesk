// SPDX-License-Identifier: AGPL-3.0-only
import { z } from "zod";
import { connection } from "next/server";
import { getSessionCtx, db } from "@/lib/auth";
import { errorResponse, okResponse, AppError } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { actions, messages } from "@chastedesk/db";
import { and, eq } from "drizzle-orm";
import { getBossSender } from "@/lib/jobs";
import { JOB_NAMES } from "@chastedesk/shared";
import { writeAudit } from "@chastedesk/db";

const decideSchema = z.object({
  actionId: z.string().uuid(),
  decision: z.enum(["approve", "reject"]),
});

export async function POST(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "inbox.use");
    const input = decideSchema.parse(await req.json());
    const rows = await db
      .select()
      .from(actions)
      .where(and(eq(actions.id, input.actionId), eq(actions.orgId, authed.orgId)))
      .limit(1);
    const action = rows[0];
    if (!action) throw new AppError("not_found", "action not found", 404);
    if (action.status !== "pending_approval") throw new AppError("conflict", `action is ${action.status}, not pending`, 409);

    if (input.decision === "reject") {
      await db
        .update(actions)
        .set({ status: "rejected_before_exec", decidedBy: authed.userId, decidedAt: new Date(), updatedAt: new Date() })
        .where(eq(actions.id, action.id));
      await writeAudit(authed, { actorType: "user", actorId: authed.userId, type: "action.rejected", targetType: "action", targetId: action.id, payload: {} }, db);
      return okResponse({ status: "rejected_before_exec" });
    }

    await db
      .update(actions)
      .set({ status: "approved", decidedBy: authed.userId, decidedAt: new Date(), updatedAt: new Date() })
      .where(eq(actions.id, action.id));
    await writeAudit(authed, { actorType: "user", actorId: authed.userId, type: "action.approved", targetType: "action", targetId: action.id, payload: {} }, db);
    const boss = await getBossSender();
    await boss.send(JOB_NAMES.executeAction, { orgId: authed.orgId, actionId: action.id }, { singletonKey: `action:${action.id}` });
    await db.insert(messages).values({ orgId: authed.orgId, conversationId: action.conversationId, role: "system", content: "Action approved - executing." });
    return okResponse({ status: "approved" });
  } catch (err) {
    return errorResponse(err);
  }
}
