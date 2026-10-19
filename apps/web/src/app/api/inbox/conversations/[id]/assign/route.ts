// SPDX-License-Identifier: AGPL-3.0-only
import { z } from "zod";
import { connection } from "next/server";
import { getSessionCtx, db } from "@/lib/auth";
import { errorResponse, okResponse, AppError } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { conversations } from "@chastedesk/db";
import { and, eq } from "drizzle-orm";

export async function POST(req: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "inbox.use");
    const { id } = await params;
    const input = z.object({ assignedUserId: z.string().nullable() }).parse(await req.json());
    const updated = await db
      .update(conversations)
      .set({ assignedUserId: input.assignedUserId ?? authed.userId, updatedAt: new Date() })
      .where(and(eq(conversations.id, id), eq(conversations.orgId, authed.orgId)))
      .returning({ id: conversations.id });
    if (!updated[0]) throw new AppError("not_found", "conversation not found", 404);
    return okResponse({ assigned: true });
  } catch (err) {
    return errorResponse(err);
  }
}
