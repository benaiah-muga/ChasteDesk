// SPDX-License-Identifier: AGPL-3.0-only
import { drizzle } from "drizzle-orm/node-postgres";
import { Pool } from "pg";
import * as schema from "./schema.ts";

export type Database = ReturnType<typeof createDb>["db"];

export function createDb(connectionString: string) {
  const pool = new Pool({ connectionString, max: 10 });
  const db = drizzle(pool, { schema });
  return { db, pool };
}

export interface RepoCtx {
  orgId: string;
}

export function assertCtx(ctx: RepoCtx): string {
  if (!ctx || typeof ctx.orgId !== "string" || ctx.orgId.length === 0) {
    throw new Error("repository functions require a RepoCtx with orgId");
  }
  return ctx.orgId;
}
