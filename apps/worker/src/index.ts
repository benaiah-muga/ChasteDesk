// SPDX-License-Identifier: AGPL-3.0-only
import { startBoss } from "./boss";
import type { ExecuteActionData } from "./jobs/execute-action";

export async function main() {
  const connectionString = process.env.DATABASE_URL;
  if (!connectionString) {
    console.error("DATABASE_URL is required");
    process.exit(1);
  }
  const running = await startBoss(connectionString, { uploadsDir: process.env.UPLOADS_DIR ?? "data/uploads" });
  running.boss.work("execute-action", { batchSize: 1 }, async (jobsOrJob: unknown) => {
    const items = Array.isArray(jobsOrJob) ? jobsOrJob : [jobsOrJob];
    for (const item of items as Array<{ data: ExecuteActionData }>) {
      const { executeApprovedAction } = await import("./jobs/execute-action.ts");
      await executeApprovedAction(running.pool, item.data);
    }
  });
  console.log("worker started: ingest-url, ingest-file, ingest-text, embed-chunks, execute-action");
  const shutdown = async () => {
    await running.stop();
    process.exit(0);
  };
  process.on("SIGTERM", shutdown);
  process.on("SIGINT", shutdown);
}

if (process.env.NODE_ENV !== "test") {
  main().catch((err) => {
    console.error("worker failed to start", err);
    process.exit(1);
  });
}
