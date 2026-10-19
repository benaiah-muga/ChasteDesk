// SPDX-License-Identifier: AGPL-3.0-only
import pg from "pg";
import PgBoss from "pg-boss";
import { JOB_NAMES } from "@chastedesk/shared";
import { makeHandlers } from "./jobs/handlers.ts";

export interface RunningBoss {
  boss: PgBoss;
  pool: pg.Pool;
  stop: () => Promise<void>;
}

export async function startBoss(connectionString: string, opts: { uploadsDir: string; only?: string[] } = { uploadsDir: "data/uploads" }): Promise<RunningBoss> {
  const pool = new pg.Pool({ connectionString, max: 5 });
  const boss = new PgBoss({ connectionString, max: 2 });
  await boss.start();
  const handlers = makeHandlers({ pool, uploadsDir: opts.uploadsDir });
  const wanted: string[] = opts.only ?? [JOB_NAMES.ingestUrl, JOB_NAMES.ingestFile, JOB_NAMES.ingestText, JOB_NAMES.embedChunks, JOB_NAMES.aiRespond, JOB_NAMES.executeAction, JOB_NAMES.analyticsRollup];
  for (const name of wanted) {
    await boss.createQueue(name).catch(() => undefined);
  }
  const works = wanted.map((name) =>
    boss.work(name, { batchSize: 1 }, async (jobsOrJob: unknown) => {
      const handler = handlers[name as keyof typeof handlers];
      const items = Array.isArray(jobsOrJob) ? jobsOrJob : [jobsOrJob];
      for (const item of items as Array<{ data: unknown }>) {
        await handler(item.data as never);
      }
    }),
  );
  await Promise.all(works);
  return {
    boss,
    pool,
    stop: async () => {
      await boss.stop({ close: false, timeout: 2000 });
      await pool.end();
    },
  };
}
