// SPDX-License-Identifier: AGPL-3.0-only
import type pg from "pg";
import { writeAuditPool } from "@chastedesk/db";
import type { RunContext } from "./context.ts";
import type { SseEvent } from "@chastedesk/shared";

export interface RunPersistenceDeps {
  pool: pg.Pool;
  notify?: (event: SseEvent) => Promise<void>;
}

export async function startRun(deps: RunPersistenceDeps, ctx: RunContext): Promise<void> {
  await deps.pool.query(
    `insert into ai_runs (id, org_id, conversation_id, agent_id, status, steps, evidence, usage)
     values ($1, $2, $3, $4, 'running', '[]'::jsonb, '[]'::jsonb, '{}'::jsonb)`,
    [ctx.runId, ctx.orgId, ctx.conversationId, ctx.agentId],
  );
  await appendAudit(deps, ctx, "run.started", "ai_run", ctx.runId, {});
  await writeAuditPool(deps.pool, ctx.orgId, { actorType: "agent", actorId: ctx.agentId, type: "conversation.created", targetType: "conversation", targetId: ctx.conversationId, payload: {} }).catch(() => {});
}

export async function appendAudit(
  deps: RunPersistenceDeps,
  ctx: RunContext,
  type: string,
  targetType: string,
  targetId: string | null | undefined,
  payload: Record<string, unknown>,
): Promise<void> {
  await writeAuditPool(
    deps.pool,
    ctx.orgId,
    { actorType: "agent", actorId: ctx.agentId, type, targetType, targetId: targetId ?? undefined, payload },
  );
}

export async function finishRun(
  deps: RunPersistenceDeps,
  ctx: RunContext,
  update: { status: string; outcome?: string | null; steps?: unknown[]; evidence?: unknown[]; usage?: Record<string, number>; error?: string | null | undefined },
): Promise<void> {
  await deps.pool.query(
    `update ai_runs set status = $2, outcome = $3, steps = $4::jsonb, evidence = $5::jsonb, usage = $6::jsonb, error = $7, updated_at = now()
     where id = $1`,
    [ctx.runId, update.status, update.outcome ?? null, JSON.stringify(update.steps ?? []), JSON.stringify(update.evidence ?? []), JSON.stringify(update.usage ?? {}), update.error != null ? update.error : null],
  );
}

export async function publishEvent(deps: RunPersistenceDeps, event: SseEvent): Promise<void> {
  if (!deps.notify) return;
  await deps.notify(event);
}
