// SPDX-License-Identifier: AGPL-3.0-only
import { createHash } from "node:crypto";
import { eq } from "drizzle-orm";
import { db, pool } from "@/lib/auth";
import { apiKeys } from "@chastedesk/db";

export function generateApiKey(): { plaintext: string; hash: string; prefix: string } {
  const raw = randomToken(24);
  const plaintext = `cd_${raw}`;
  return { plaintext, hash: sha256(plaintext), prefix: plaintext.slice(0, 8) };
}

export function sha256(text: string): string {
  return createHash("sha256").update(text).digest("hex");
}

function randomToken(bytes: number): string {
  const chars = "abcdefghijklmnopqrstuvwxyz0123456789";
  let out = "";
  const arr = new Uint8Array(bytes);
  crypto.getRandomValues(arr);
  for (const b of arr) out += chars[b % chars.length];
  return out;
}

export interface ApiKeyCtx {
  orgId: string;
  keyId: string;
}

export async function resolveApiKey(req: Request): Promise<ApiKeyCtx | null> {
  const header = req.headers.get("authorization") ?? "";
  if (!header.startsWith("Bearer cd_")) return null;
  const key = header.slice("Bearer ".length);
  const hash = sha256(key);
  const rows = await db.select().from(apiKeys).where(eq(apiKeys.keyHash, hash)).limit(1);
  const row = rows[0];
  if (!row) return null;
  await pool.query("update api_keys set last_used_at = now() where id = $1", [row.id]);
  return { orgId: row.orgId, keyId: row.id };
}

export async function subscribePg(onEvent: (payload: { orgId: string; type: string; conversationId: string }) => void): Promise<{ client: object; stop: () => Promise<void> }> {
  const { Client } = await import("pg");
  const client = new Client({ connectionString: process.env.DATABASE_URL });
  await client.connect();
  await client.query("LISTEN chastedesk");
  client.on("notification", (msg) => {
    if (!msg.payload) return;
    try {
      onEvent(JSON.parse(msg.payload));
    } catch {
      /* ignore malformed payloads */
    }
  });
  return {
    client,
    stop: async () => {
      try {
        await client.end();
      } catch {
        /* already closed */
      }
    },
  };
}

export async function publishEvent(event: { orgId: string; type: string; conversationId: string }): Promise<void> {
  await pool.query("select pg_notify($1, $2)", ["chastedesk", JSON.stringify(event)]);
}
