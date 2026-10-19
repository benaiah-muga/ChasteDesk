// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";

import { getSessionCtx, db } from "@/lib/auth";
import { errorResponse, okResponse, AppError } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { getBossSender } from "@/lib/jobs";
import { writeAudit, knowledgeSources } from "@chastedesk/db";
import { JOB_NAMES } from "@chastedesk/shared";
import { desc, eq } from "drizzle-orm";
import { z } from "zod";

export async function GET(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "knowledge.read");
    const rows = await db
      .select({
        id: knowledgeSources.id,
        type: knowledgeSources.type,
        name: knowledgeSources.name,
        scope: knowledgeSources.scope,
        status: knowledgeSources.status,
        version: knowledgeSources.version,
        error: knowledgeSources.error,
        embeddingModel: knowledgeSources.embeddingModel,
        updatedAt: knowledgeSources.updatedAt,
      })
      .from(knowledgeSources)
      .where(eq(knowledgeSources.orgId, authed.orgId))
      .orderBy(desc(knowledgeSources.updatedAt));
    return okResponse(rows);
  } catch (err) {
    return errorResponse(err);
  }
}

const createSchema = z.discriminatedUnion("kind", [
  z.object({ kind: z.literal("url"), url: z.string().url().max(2000), name: z.string().max(200).optional(), scope: z.enum(["public", "internal"]).default("public") }),
  z.object({ kind: z.literal("text"), name: z.string().min(1).max(200), text: z.string().min(1).max(500_000), scope: z.enum(["public", "internal"]).default("public") }),
]);

export async function POST(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "knowledge.write");
    const contentType = req.headers.get("content-type") ?? "";
    const boss = await getBossSender();

    if (contentType.includes("multipart/form-data")) {
      const form = await req.formData();
      const file = form.get("file");
      const scope = (form.get("scope") as string) === "internal" ? "internal" : "public";
      if (!(file instanceof File)) throw new AppError("bad_request", "file is required", 400);
      const { saveUpload } = await import("@/lib/uploads");
      const stored = await saveUpload(file);
      const inserted = await db
        .insert(knowledgeSources)
        .values({ orgId: authed.orgId, type: "file", name: stored.originalName, scope, status: "pending", sourceConfig: { path: stored.storedName, filename: stored.originalName } })
        .returning({ id: knowledgeSources.id });
      const row0 = inserted[0];
      if (!row0) throw new AppError("internal", "insert failed", 500);
      const sourceId = row0.id;
      await boss.send(JOB_NAMES.ingestFile, { orgId: authed.orgId, sourceId, filePath: stored.storedName, filename: stored.originalName }, { singletonKey: `src:${sourceId}` });
      await writeAudit(authed, { actorType: "user", actorId: authed.userId, type: "knowledge.source.created", targetType: "knowledge_source", targetId: sourceId, payload: { kind: "file", name: stored.originalName, scope } }, db);
      return okResponse({ id: sourceId }, 201);
    }

    const input = createSchema.parse(await req.json());
    if (input.kind === "url") {
      const inserted = await db
        .insert(knowledgeSources)
        .values({ orgId: authed.orgId, type: "url", name: input.name ?? input.url, scope: input.scope, status: "pending", sourceConfig: { url: input.url } })
        .returning({ id: knowledgeSources.id });
      const firstRow = inserted[0];
      if (!firstRow) throw new AppError("internal", "insert failed", 500);
      const sourceId = firstRow.id;
      await boss.send(JOB_NAMES.ingestUrl, { orgId: authed.orgId, sourceId, url: input.url }, { singletonKey: `src:${sourceId}` });
      await writeAudit(authed, { actorType: "user", actorId: authed.userId, type: "knowledge.source.created", targetType: "knowledge_source", targetId: sourceId, payload: { kind: "url", url: input.url, scope: input.scope } }, db);
      return okResponse({ id: sourceId }, 201);
    }

    const inserted = await db
      .insert(knowledgeSources)
      .values({ orgId: authed.orgId, type: "text", name: input.name, scope: input.scope, status: "pending", sourceConfig: { bytes: Buffer.byteLength(input.text) } })
      .returning({ id: knowledgeSources.id });
    const sourceId = inserted[0]!.id;
    await boss.send(JOB_NAMES.ingestText, { orgId: authed.orgId, sourceId, text: input.text, name: input.name }, { singletonKey: `src:${sourceId}` });
    await writeAudit(authed, { actorType: "user", actorId: authed.userId, type: "knowledge.source.created", targetType: "knowledge_source", targetId: sourceId, payload: { kind: "text", name: input.name, scope: input.scope } }, db);
    return okResponse({ id: sourceId }, 201);
  } catch (err) {
    return errorResponse(err);
  }
}
