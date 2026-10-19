// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";
import { getSessionCtx, db } from "@/lib/auth";
import { errorResponse, okResponse, AppError } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { actions, conversations, customers, messages } from "@chastedesk/db";
import { and, asc, desc, eq } from "drizzle-orm";

export async function GET(req: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "inbox.use");
    const { id } = await params;
    const convRows = await db
      .select()
      .from(conversations)
      .where(and(eq(conversations.id, id), eq(conversations.orgId, authed.orgId)))
      .limit(1);
    const conversation = convRows[0];
    if (!conversation) throw new AppError("not_found", "conversation not found", 404);
    const customerRows = await db.select().from(customers).where(eq(customers.id, conversation.customerId)).limit(1);
    const customer = customerRows[0] ?? null;
    const msgs = await db
      .select({ id: messages.id, role: messages.role, content: messages.content, citations: messages.citations, createdAt: messages.createdAt })
      .from(messages)
      .where(eq(messages.conversationId, id))
      .orderBy(asc(messages.createdAt));
    const recent = await db
      .select({ id: conversations.id, status: conversations.status, createdAt: conversations.createdAt })
      .from(conversations)
      .where(and(eq(conversations.orgId, authed.orgId), eq(conversations.customerId, conversation.customerId)))
      .orderBy(desc(conversations.createdAt))
      .limit(10);
    const pending = await db
      .select({ id: actions.id, toolId: actions.toolId, args: actions.args, status: actions.status, createdAt: actions.createdAt })
      .from(actions)
      .where(and(eq(actions.orgId, authed.orgId), eq(actions.conversationId, id)))
      .orderBy(desc(actions.createdAt))
      .limit(20);
    const otherConversations = recent.filter((c) => c.id !== id);
    return okResponse({
      conversation,
      customer: customer ? { id: customer.id, name: customer.name, email: customer.email, attributes: customer.attributes } : null,
      messages: msgs,
      recentConversations: otherConversations,
      openConversations: otherConversations.filter((c) => !["resolved", "closed"].includes(c.status)).length,
      actions: pending,
    });
  } catch (err) {
    return errorResponse(err);
  }
}
