// SPDX-License-Identifier: AGPL-3.0-only
import { z } from "zod";
import { connection } from "next/server";
import { getSessionCtx, db } from "@/lib/auth";
import { errorResponse, okResponse} from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { messages } from "@chastedesk/db";

export async function POST(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "inbox.use");
    const input = z.object({ conversationId: z.string().uuid(), content: z.string().min(1).max(8000) }).parse(await req.json());
    await db.insert(messages).values({ orgId: authed.orgId, conversationId: input.conversationId, role: "agent_human", content: input.content });
    return okResponse({ sent: true });
  } catch (err) {
    return errorResponse(err);
  }
}
