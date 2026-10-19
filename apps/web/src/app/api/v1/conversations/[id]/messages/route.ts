// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";
import { z } from "zod";
import { db } from "@/lib/auth";
import { errorResponse, okResponse, AppError } from "@/lib/app-error";
import { resolveApiKey } from "@/lib/sse";
import { getBossSender } from "@/lib/jobs";
import { JOB_NAMES } from "@chastedesk/shared";
import { publishEventWeb } from "@/lib/publish";
import { messages } from "@chastedesk/db";
import { and, eq } from "drizzle-orm";

export async function POST(req: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    await connection();
    const api = await resolveApiKey(req);
    if (!api) throw new AppError("unauthorized", "valid API key required", 401);
    const { id: conversationId } = await params;
    const owned = await db
      .select({ id: messages.id })
      .from(messages)
      .where(and(eq(messages.conversationId, conversationId), eq(messages.orgId, api.orgId)))
      .limit(1);
    if (!owned[0]) throw new AppError("not_found", "conversation not found", 404);
    const input = z.object({ content: z.string().min(1).max(4000) }).parse(await req.json());
    await db.insert(messages).values({ orgId: api.orgId, conversationId, role: "customer", content: input.content });
    const boss = await getBossSender();
    await boss.send(JOB_NAMES.aiRespond, { orgId: api.orgId, conversationId }, { singletonKey: `conv:${conversationId}` });
    await publishEventWeb({ orgId: api.orgId, type: "message.created", conversationId });
    return okResponse({ received: true }, 202);
  } catch (err) {
    return errorResponse(err);
  }
}
