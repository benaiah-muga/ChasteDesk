// SPDX-License-Identifier: AGPL-3.0-only
import PgBoss from "pg-boss";
import { JOB_NAMES } from "@chastedesk/shared";

let bossPromise: Promise<PgBoss> | null = null;

export function getBossSender(): Promise<PgBoss> {
  if (!bossPromise) {
    const connectionString = process.env.DATABASE_URL ?? "";
    const boss = new PgBoss({ connectionString, max: 2 });
    bossPromise = boss.start().then(async () => {
      for (const name of Object.values(JOB_NAMES)) {
        await boss.createQueue(name).catch(() => undefined);
      }
      return boss;
    });
  }
  return bossPromise;
}
