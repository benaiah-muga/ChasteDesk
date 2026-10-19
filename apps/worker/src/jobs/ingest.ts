// SPDX-License-Identifier: AGPL-3.0-only
import type pg from "pg";
import { writeAuditPool } from "@chastedesk/db";
import { chunkDocument } from "@chastedesk/retrieval";
import type { ParsedDocument } from "@chastedesk/shared";

export interface IngestJobData {
  orgId: string;
  sourceId: string;
  kind: "url" | "file";
  url?: string;
  filePath?: string;
  filename?: string;
}

export async function indexDocument(
  pool: pg.Pool,
  orgId: string,
  sourceId: string,
  doc: ParsedDocument,
): Promise<number> {
  const current = await pool.query(
    "select version from knowledge_sources where id = $1 and org_id = $2 for update",
    [sourceId, orgId],
  );
  const row = current.rows[0] as { version: number } | undefined;
  if (!row) throw new Error(`source ${sourceId} not found`);
  const newVersion = Number(row.version) + 1;
  const chunks = chunkDocument(doc, sourceId, newVersion);
  await pool.query("begin");
  try {
    await pool.query("delete from knowledge_chunks where source_id = $1", [sourceId]);
    for (const chunk of chunks) {
      await pool.query(
        `insert into knowledge_chunks (id, org_id, source_id, version, chunk_index, content, locator, token_count)
         values ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)`,
        [chunk.id, orgId, sourceId, newVersion, chunk.chunkIndex, chunk.content, JSON.stringify(chunk.locator), chunk.tokenCount],
      );
    }
    await pool.query(
      "update knowledge_sources set version = $2, status = 'ready', error = null, updated_at = now() where id = $1 and org_id = $3",
      [sourceId, newVersion, orgId],
    );
    await pool.query("commit");
  } catch (err) {
    await pool.query("rollback");
    throw err;
  }
  await writeAuditPool(pool, orgId, {
    actorType: "system",
    type: "knowledge.ingest.completed",
    targetType: "knowledge_source",
    targetId: sourceId,
    payload: { version: newVersion, chunks: chunks.length },
  });
  return newVersion;
}

export async function markSourceFailed(pool: pg.Pool, orgId: string, sourceId: string, error: string) {
  await pool.query(
    "update knowledge_sources set status = 'failed', error = $3, updated_at = now() where id = $1 and org_id = $2",
    [sourceId, orgId, error.slice(0, 500)],
  );
  await writeAuditPool(pool, orgId, {
    actorType: "system",
    type: "knowledge.ingest.failed",
    targetType: "knowledge_source",
    targetId: sourceId,
    payload: { error: error.slice(0, 500) },
  });
}
