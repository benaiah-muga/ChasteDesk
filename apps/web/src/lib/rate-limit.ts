// SPDX-License-Identifier: AGPL-3.0-only
import { AppError } from "./app-error";
import { db } from "./auth";
import { sql } from "drizzle-orm";

export async function enforceRateLimit(key: string, limit: number, windowSeconds: number) {
  const result = await db.execute(sql`
    insert into rate_limit_windows (key, window_start, count)
    values (${key}, now(), 1)
    on conflict (key) do update set
      count = case
        when rate_limit_windows.window_start < now() - (${String(windowSeconds)} || ' seconds')::interval then 1
        else rate_limit_windows.count + 1
      end,
      window_start = case
        when rate_limit_windows.window_start < now() - (${String(windowSeconds)} || ' seconds')::interval then now()
        else rate_limit_windows.window_start
      end
    returning count
  `);
  const rows = result.rows as Array<{ count: number }>;
  const count = rows[0]?.count ?? 0;
  if (count > limit) {
    throw new AppError("rate_limited", "too many requests, slow down", 429);
  }
  return { count, remaining: Math.max(0, limit - count) };
}
