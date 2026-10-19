// SPDX-License-Identifier: AGPL-3.0-only
import { z } from "zod";
import { connection } from "next/server";
import { getSessionCtx, db } from "@/lib/auth";
import { errorResponse, okResponse, AppError } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { writeAudit, conversations, messages } from "@chastedesk/db";
import { and, eq } from "drizzle-orm";
import { publishEventWeb } from "@/lib/publish";

export async function POST(req: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "inbox.use");
    const { id } = await params;
    const input = z
      .object({ content: z.string().min(1).max(8000), internal: z.boolean().default(false) })
      .parse(await req.json());
    const rows = await db
      .select({ id: conversations.id, status: conversations.status, firstResponseAt: conversations.firstResponseAt })
      .from(conversations)
      .where(and(eq(conversations.id, id), eq(conversations.orgId, authed.orgId)))
      .limit(1);
    const conversation = rows[0];
    if (!conversation) throw new AppError("not_found", "conversation not found", 404);
    const role = input.internal ? "internal_note" : "agent_human";
    await db.insert(messages).values({ orgId: authed.orgId, conversationId: id, role, content: input.content });
    if (!input.internal) {
      await db
        .update(conversations)
        .set({
          status: conversation.status === "awaiting_human" || conversation.status === "ai_active" ? "human_active" : conversation.status,
          firstResponseAt: conversation.firstResponseAt ?? new Date(),
          updatedAt: new Date(),
        })
        .where(eq(conversations.id, id));
      await publishEventWeb({ orgId: authed.orgId, conversationId: id, type: "message.created" });
    }
    await writeAudit(authed, { actorType: "user", actorId: authed.userId, type: "message.created", targetType: "conversation", targetId: id, payload: { role, length: input.content.length } }, db);
    return okResponse({ sent: true, role }, 201);
  } catch (err) {
    return errorResponse(err);
  }
}
