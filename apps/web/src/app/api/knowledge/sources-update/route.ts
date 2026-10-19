// SPDX-License-Identifier: AGPL-3.0-only
import { z } from "zod";
import { connection } from "next/server";
import { getSessionCtx, db } from "@/lib/auth";
import { errorResponse, okResponse, AppError } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { writeAudit, knowledgeSources } from "@chastedesk/db";
import { and, eq } from "drizzle-orm";

export async function POST(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "knowledge.write");
    const input = z
      .object({ sourceId: z.string().min(1), scope: z.enum(["public", "internal"]).optional() })
      .parse(await req.json());
    const updates = {
      ...(input.scope ? { scope: input.scope } : {}),
      updatedAt: new Date(),
    };
    const updated = await db
      .update(knowledgeSources)
      .set(updates)
      .where(and(eq(knowledgeSources.id, input.sourceId), eq(knowledgeSources.orgId, authed.orgId)))
      .returning({ id: knowledgeSources.id });
    if (!updated[0]) throw new AppError("not_found", "source not found", 404);
    await writeAudit(authed, { actorType: "user", actorId: authed.userId, type: "knowledge.source.updated", targetType: "knowledge_source", targetId: input.sourceId, payload: { scope: input.scope ?? "unchanged" } }, db);
    return okResponse({ updated: true });
  } catch (err) {
    return errorResponse(err);
  }
}

export async function DELETE(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "knowledge.write");
    const input = z.object({ sourceId: z.string().min(1) }).parse(await req.json());
    const deleted = await db
      .delete(knowledgeSources)
      .where(and(eq(knowledgeSources.id, input.sourceId), eq(knowledgeSources.orgId, authed.orgId)))
      .returning({ id: knowledgeSources.id });
    if (!deleted[0]) throw new AppError("not_found", "source not found", 404);
    await writeAudit(authed, { actorType: "user", actorId: authed.userId, type: "knowledge.source.deleted", targetType: "knowledge_source", targetId: input.sourceId, payload: {} }, db);
    return okResponse({ deleted: true });
  } catch (err) {
    return errorResponse(err);
  }
}
