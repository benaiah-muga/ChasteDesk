// SPDX-License-Identifier: AGPL-3.0-only
import { readFile } from "node:fs/promises";
import path from "node:path";
import type pg from "pg";
import { crawlUrl } from "../crawler/crawler.ts";
import { parseFile } from "../parsers.ts";
import { indexDocument, markSourceFailed } from "./ingest.ts";
import { executeApprovedAction } from "./execute-action.ts";

export interface HandlerDeps {
  pool: pg.Pool;
  uploadsDir: string;
}

export function makeHandlers(deps: HandlerDeps) {
  return {
    "ingest-url": async (data: { orgId: string; sourceId: string; url: string }) => {
      try {
        const crawled = await crawlUrl(data.url);
        await indexDocument(deps.pool, data.orgId, data.sourceId, { title: crawled.title || data.url, markdown: crawled.markdown });
      } catch (err) {
        await markSourceFailed(deps.pool, data.orgId, data.sourceId, err instanceof Error ? err.message : String(err));
        throw err;
      }
    },
    "ingest-file": async (data: { orgId: string; sourceId: string; filePath: string; filename: string }) => {
      try {
        const resolved = path.resolve(deps.uploadsDir, path.basename(data.filePath));
        const doc = await parseFile(resolved, data.filename);
        await indexDocument(deps.pool, data.orgId, data.sourceId, { title: data.filename, markdown: doc.markdown });
      } catch (err) {
        await markSourceFailed(deps.pool, data.orgId, data.sourceId, err instanceof Error ? err.message : String(err));
        throw err;
      }
    },
    "ingest-text": async (data: { orgId: string; sourceId: string; text: string; name?: string }) => {
      try {
        await indexDocument(deps.pool, data.orgId, data.sourceId, { title: data.name ?? "Pasted text", markdown: data.text });
      } catch (err) {
        await markSourceFailed(deps.pool, data.orgId, data.sourceId, err instanceof Error ? err.message : String(err));
        throw err;
      }
    },
    "ai-respond": async (data: { orgId: string; conversationId: string }) => {
      const { handleAiRespond } = await import("./ai-respond.ts");
      await handleAiRespond(deps.pool, process.env.APP_SECRET ?? "", data);
    },
    "analytics-rollup": async (data: { orgId: string }) => {
      const { rollupAnalytics } = await import("./analytics.ts");
      const today = new Date().toISOString().slice(0, 10);
      const yesterday = new Date(Date.now() - 86400000).toISOString().slice(0, 10);
      await rollupAnalytics(deps.pool, data.orgId, today);
      await rollupAnalytics(deps.pool, data.orgId, yesterday);
    },
    "execute-action": async (data: { orgId: string; actionId: string }) => {
      await executeApprovedAction(deps.pool, data);
    },
    "embed-chunks": async (data: { orgId: string; sourceId: string }) => {
      const provider = await deps.pool.query(
        "select provider_id from provider_configs where org_id = $1 and enabled = true limit 1",
        [data.orgId],
      );
      if (provider.rows.length === 0) {
        return;
      }
    },
  };
}

export type JobName = keyof ReturnType<typeof makeHandlers>;

export async function readUpload(uploadsDir: string, storedName: string) {
  return readFile(path.resolve(uploadsDir, path.basename(storedName)));
}
