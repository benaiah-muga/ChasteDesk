// SPDX-License-Identifier: AGPL-3.0-only
import { pool } from "@/lib/auth";

export async function publishEventWeb(event: { orgId: string; type: string; conversationId: string }): Promise<void> {
  await pool.query("select pg_notify($1, $2)", ["chastedesk", JSON.stringify(event)]);
}
