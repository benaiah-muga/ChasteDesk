// SPDX-License-Identifier: AGPL-3.0-only
import { z } from "zod";
import { connection } from "next/server";
import { getSessionCtx, db } from "@/lib/auth";
import { errorResponse, okResponse, AppError } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { writeAudit, conversations } from "@chastedesk/db";
import { and, eq } from "drizzle-orm";

export async function POST(req: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "inbox.use");
    const { id } = await params;
    const input = z.object({ action: z.enum(["resolve", "reopen"]) }).parse(await req.json());
    const updates =
      input.action === "resolve"
        ? { status: "resolved" as const, resolvedAt: new Date(), updatedAt: new Date() }
        : { status: "human_active" as const, resolvedAt: null, updatedAt: new Date() };
    const updated = await db
      .update(conversations)
      .set(updates)
      .where(and(eq(conversations.id, id), eq(conversations.orgId, authed.orgId)))
      .returning({ id: conversations.id });
    if (!updated[0]) throw new AppError("not_found", "conversation not found", 404);
    await writeAudit(authed, { actorType: "user", actorId: authed.userId, type: "conversation.updated", targetType: "conversation", targetId: id, payload: input }, db);
    return okResponse({ done: true });
  } catch (err) {
    return errorResponse(err);
  }
}
