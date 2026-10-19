// SPDX-License-Identifier: AGPL-3.0-only
import { sql, type SQLWrapper } from "drizzle-orm";
import type { RepoCtx } from "./client.ts";
import { auditEvents } from "./schema.ts";

export type AuditActorType = "user" | "agent" | "system";

export interface AuditInput {
  actorType: AuditActorType;
  actorId?: string;
  type: string;
  targetType: string;
  targetId?: string;
  payload?: Record<string, unknown>;
}

function redact(payload: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(payload)) {
    if (k === "content" || k === "args" || k === "result") {
      out[k] = String(JSON.stringify(v)).slice(0, 500);
    } else {
      out[k] = v;
    }
  }
  return out;
}

export function auditInsertSql(
  orgId: string | null,
  input: AuditInput,
): { text: string; values: unknown[] } {
  return {
    text: "insert into audit_events (org_id, actor_type, actor_id, type, target_type, target_id, payload) values ($1, $2, $3, $4, $5, $6, $7::jsonb)",
    values: [
      orgId,
      input.actorType,
      input.actorId ?? null,
      input.type,
      input.targetType,
      input.targetId ?? null,
      JSON.stringify(redact(input.payload ?? {})),
    ],
  };
}

export async function writeAudit(ctx: RepoCtx, input: AuditInput, db: { execute: (q: SQLWrapper) => Promise<unknown> }) {
  await db.execute(sql`
    insert into audit_events (org_id, actor_type, actor_id, type, target_type, target_id, payload)
    values (${ctx.orgId}, ${input.actorType}, ${input.actorId ?? null}, ${input.type}, ${input.targetType}, ${input.targetId ?? null}, ${JSON.stringify(redact(input.payload ?? {}))}::jsonb)
  `);
}

export async function writeAuditPool(pool: { query: (text: string, values?: unknown[]) => Promise<unknown> }, orgId: string | null, input: AuditInput) {
  const { text, values } = auditInsertSql(orgId, input);
  await pool.query(text, values);
}

export const auditTable = auditEvents;
