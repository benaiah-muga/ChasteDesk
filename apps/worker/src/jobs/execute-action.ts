// SPDX-License-Identifier: AGPL-3.0-only
import type pg from "pg";
import { writeAuditPool } from "@chastedesk/db";
import { buildBuiltinTools } from "@chastedesk/ai";

export interface ExecuteActionData {
  orgId: string;
  actionId: string;
  dryRunVerifyFail?: boolean;
}

export async function executeApprovedAction(pool: pg.Pool, data: ExecuteActionData): Promise<void> {
  const actionRes = await pool.query(
    "select id, org_id, run_id, conversation_id, agent_id, tool_id, args, status from actions where id = $1 and org_id = $2 for update",
    [data.actionId, data.orgId],
  );
  const action = actionRes.rows[0] as
    | { id: string; org_id: string; run_id: string; conversation_id: string; agent_id: string; tool_id: string; args: unknown; status: string }
    | undefined;
  if (!action) throw new Error(`action ${data.actionId} not found`);
  if (action.status !== "approved") throw new Error(`action ${data.actionId} is not approved (status ${action.status})`);

  const convRes = await pool.query("select customer_id, channel from conversations where id = $1", [action.conversation_id]);
  const conv = convRes.rows[0] as { customer_id: string; channel: string };

  const ctx = {
    orgId: action.org_id,
    conversationId: action.conversation_id,
    agentId: action.agent_id,
    runId: action.run_id,
    customerId: conv.customer_id,
    channel: conv.channel as "widget" | "api",
    dryRun: data.dryRunVerifyFail === true,
  };

  const tools = buildBuiltinTools({ pool });
  const tool = tools.find((t) => t.id === action.tool_id);
  if (!tool) throw new Error(`tool ${action.tool_id} has no implementation`);

  await pool.query("update actions set status = 'executing', updated_at = now() where id = $1", [action.id]);
  let result: { ok: boolean; data?: unknown; error?: string };
  try {
    result = await tool.execute(ctx, action.args);
  } catch (err) {
    result = { ok: false, error: err instanceof Error ? err.message : String(err) };
  }

  if (!result.ok) {
    await pool.query("update actions set status = 'failed', result = $2::jsonb, updated_at = now() where id = $1", [action.id, JSON.stringify(result)]);
    await writeAuditPool(pool, action.org_id, { actorType: "agent", actorId: action.agent_id, type: "action.failed", targetType: "action", targetId: action.id, payload: { error: result.error ?? "failed" } });
    await pool.query(
      `insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'system', $3)`,
      [action.org_id, action.conversation_id, "The requested action could not be completed — escalating to a teammate."],
    );
    await pool.query("update conversations set status = 'awaiting_human', updated_at = now() where id = $1", [action.conversation_id]);
    return;
  }

  let verification: { passed: boolean; detail: string } = { passed: true, detail: "no verification required" };
  if (data.dryRunVerifyFail === true) {
    verification = { passed: false, detail: "forced verification failure (test)" };
  } else if (tool.verify) {
    try {
      verification = await tool.verify(ctx, action.args);
    } catch (err) {
      verification = { passed: false, detail: err instanceof Error ? err.message : "verification errored" };
    }
  }

  await pool.query(
    "update actions set status = 'executed', result = $2::jsonb, verification = $3::jsonb, updated_at = now() where id = $1",
    [action.id, JSON.stringify(result), JSON.stringify(verification)],
  );
  await writeAuditPool(pool, action.org_id, { actorType: "agent", actorId: action.agent_id, type: "action.executed", targetType: "action", targetId: action.id, payload: { ok: result.ok } });
  await writeAuditPool(pool, action.org_id, { actorType: "agent", actorId: action.agent_id, type: "action.verified", targetType: "action", targetId: action.id, payload: verification });

  if (verification.passed) {
    await pool.query(
      `insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'agent_ai', $3)`,
      [action.org_id, action.conversation_id, "Done — I've taken care of that for you."],
    );
  }
}
