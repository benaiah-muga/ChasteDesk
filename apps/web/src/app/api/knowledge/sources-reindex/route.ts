// SPDX-License-Identifier: AGPL-3.0-only
import { z } from "zod";
import { connection } from "next/server";
import { getSessionCtx, db } from "@/lib/auth";
import { errorResponse, okResponse, AppError } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { getBossSender } from "@/lib/jobs";
import { writeAudit, knowledgeSources } from "@chastedesk/db";
import { JOB_NAMES } from "@chastedesk/shared";
import { and, eq } from "drizzle-orm";

export async function POST(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "knowledge.write");
    const input = z.object({ sourceId: z.string().min(1) }).parse(await req.json());
    const id = input.sourceId;
    const rows = await db
      .select()
      .from(knowledgeSources)
      .where(and(eq(knowledgeSources.id, id), eq(knowledgeSources.orgId, authed.orgId)))
      .limit(1);
    const source = rows[0];
    if (!source) throw new AppError("not_found", "source not found", 404);
    const config = source.sourceConfig as { url?: string; path?: string; filename?: string };
    const boss = await getBossSender();
    if (source.type === "url" && config.url) {
      await boss.send(JOB_NAMES.ingestUrl, { orgId: authed.orgId, sourceId: id, url: config.url }, { singletonKey: `src:${id}` });
    } else if (source.type === "file" && config.path) {
      await boss.send(JOB_NAMES.ingestFile, { orgId: authed.orgId, sourceId: id, filePath: config.path, filename: config.filename ?? source.name }, { singletonKey: `src:${id}` });
    } else {
      throw new AppError("bad_request", "source cannot be reindexed", 400);
    }
    await db.update(knowledgeSources).set({ status: "pending", updatedAt: new Date() }).where(eq(knowledgeSources.id, id));
    await writeAudit(authed, { actorType: "user", actorId: authed.userId, type: "knowledge.source.updated", targetType: "knowledge_source", targetId: id, payload: { reindex: true } }, db);
    return okResponse({ reindexing: true });
  } catch (err) {
    return errorResponse(err);
  }
}
