// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";
import { z } from "zod";
import { getSessionCtx, db, pool } from "@/lib/auth";
import { errorResponse, okResponse, AppError } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { createBuiltinEngine } from "@chastedesk/retrieval";
import { messages, conversations } from "@chastedesk/db";
import { and, eq } from "drizzle-orm";

export async function POST(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "inbox.use");
    const { conversationId } = z.object({ conversationId: z.string().uuid() }).parse(await req.json());
    const convRows = await db
      .select({ id: conversations.id, handoff: conversations.handoff })
      .from(conversations)
      .where(and(eq(conversations.id, conversationId), eq(conversations.orgId, authed.orgId)))
      .limit(1);
    const conversation = convRows[0];
    if (!conversation) throw new AppError("not_found", "conversation not found", 404);

    const engine = createBuiltinEngine(pool);
    const lastCust = await db
      .select({ content: messages.content })
      .from(messages)
      .where(and(eq(messages.conversationId, conversationId), eq(messages.role, "customer")))
      .limit(1);
    const query = lastCust[0]?.content ?? "customer question";
    const evidence = await engine.search(authed.orgId, { text: query, scopes: ["public", "internal"], limit: 3 });

    const lines = evidence.map((e, i) => `[E${i + 1}] ${e.content.slice(0, 280)} (source: ${e.sourceName} v${e.version})`);
    const draft = [
      `Hi${""},`,
      "",
      lines.length
        ? lines.join("\n\n")
        : "I checked our knowledge base but could not find a confident answer - a teammate will follow up shortly.",
      "",
      lines.length ? "These points come from our help docs - let me know if that solves it." : "",
    ]
      .filter(Boolean)
      .join("\n");
    return okResponse({ draft, citations: evidence.map((e) => ({ chunkId: e.chunkId, sourceName: e.sourceName, version: e.version })), grounded: lines.length > 0 });
  } catch (err) {
    return errorResponse(err);
  }
}
