// SPDX-License-Identifier: AGPL-3.0-only
import type pg from "pg";
import { randomUUID } from "node:crypto";
import { createBuiltinEngine } from "@chastedesk/retrieval";
import { buildBuiltinTools, resolveProviderConfig, providerFor, runAgentTurn } from "@chastedesk/ai";
import type { ProviderId } from "@chastedesk/ai";
import { agentPolicySchema, type AgentPolicy } from "@chastedesk/shared";
import { writeAuditPool } from "@chastedesk/db";

export interface AiRespondData {
  orgId: string;
  conversationId: string;
}

export async function handleAiRespond(pool: pg.Pool, appSecret: string, data: AiRespondData): Promise<void> {
  const convRes = await pool.query(
    `select id, customer_id, channel, status, first_response_at from conversations where id = $1 and org_id = $2`,
    [data.conversationId, data.orgId],
  );
  const conv = convRes.rows[0] as
    | { id: string; customer_id: string; channel: string; status: string; first_response_at: string | null }
    | undefined;
  if (!conv) throw new Error(`conversation ${data.conversationId} not found`);
  if (conv.status === "resolved" || conv.status === "closed") return;

  const agentRes = await pool.query(
    "select id, policy from agents where org_id = $1 and is_active = true order by created_at limit 1",
    [data.orgId],
  );
  const agent = agentRes.rows[0] as { id: string; policy: unknown } | undefined;
  if (!agent) throw new Error("no active agent configured");
  const policy: AgentPolicy = agentPolicySchema.parse(agent.policy);

  const customerRes = await pool.query("select id, name, email, attributes from customers where id = $1", [conv.customer_id]);
  const customer = customerRes.rows[0] as { name: string | null; email: string | null; attributes: Record<string, unknown> };

  const historyRes = await pool.query(
    "select role, content from messages where conversation_id = $1 and role <> 'internal_note' order by created_at asc limit 40",
    [data.conversationId],
  );
  const lastCustomer = [...historyRes.rows].reverse().find((r) => r.role === "customer");
  const customerMessage = String(lastCustomer?.content ?? "").slice(0, 4000);
  if (!customerMessage) return;

  const history = historyRes.rows.slice(-10).map((r) => ({
    role: r.role === "customer" ? ("customer" as const) : r.role === "agent_ai" ? ("agent_ai" as const) : ("agent_human" as const),
    content: String(r.content).slice(0, 2000),
  }));

  const recentRes = await pool.query(
    `select to_char(created_at, 'YYYY-MM-DD') || ' - ' || coalesce(triage->>'summary', 'previous conversation') as line
     from conversations where org_id = $1 and customer_id = $2 and id <> $3 order by created_at desc limit 5`,
    [data.orgId, conv.customer_id, data.conversationId],
  );

  const engine = createBuiltinEngine(pool);
  const allTools = buildBuiltinTools({ pool, retrieval: engine });
  const tools = allTools
    .map((t) => {
      const configured = policy.tools.find((p) => p.toolId === t.id);
      const mode = configured?.mode ?? (t.risk === "approval_required" ? "approval_required" : "autonomous");
      return { t, mode };
    })
    .filter(({ mode }) => mode !== "disabled")
    .map(({ t, mode }) => ({
      declaration: { id: t.id, description: t.description, parameters: t.parameters },
      mode: mode as "autonomous" | "approval_required",
      execute: async (args: unknown) =>
        t.execute(
          { orgId: data.orgId, conversationId: data.conversationId, agentId: agent.id, runId: randomUUID(), customerId: conv.customer_id, channel: conv.channel as "widget" | "api", dryRun: false },
          args,
        ),
    }));

  const configuredProvider = policy.model.providerId || null;
  const providerId = (configuredProvider ?? (await firstEnabledProvider(pool, data.orgId))) as ProviderId | null;
  if (!providerId) {
    await escalateWithoutProvider(pool, data.orgId, data.conversationId, agent.id, "No AI provider is configured yet - routing to a teammate.");
    return;
  }
  const resolved = await resolveProviderConfig(pool, appSecret, data.orgId, providerId);
  const provider = providerFor(providerId);
  const cacheRes = await pool.query("select models_cache from provider_configs where org_id = $1 and provider_id = $2", [data.orgId, providerId]);
  const modelsCache = (cacheRes.rows[0] as { models_cache?: Array<{ id: string }> } | undefined)?.models_cache ?? [];

  const result = await runAgentTurn(
    { orgId: data.orgId, conversationId: data.conversationId, agentId: agent.id, runId: randomUUID(), customerId: conv.customer_id, channel: conv.channel as "widget" | "api" },
    policy,
    {
      customerMessage,
      history,
      customer: { name: customer.name, email: customer.email, attributes: customer.attributes ?? {} },
      recentConversations: recentRes.rows.map((r) => r.line),
    },
    {
      chat: (req) => {
        const modelId = policy.model.modelId || modelsCache[0]?.id || "gpt-4o-mini";
        return provider.chat(resolved.cfg, { ...req, modelId });
      },
      pool,
      retrieval: engine,
      tools,
      notify: async (ev) => {
        await pool.query("select pg_notify($1, $2)", ["chastedesk", JSON.stringify(ev)]);
      },
    },
  );

  if (result.outcome === "answered") {
    await pool.query(
      "update conversations set first_response_at = coalesce(first_response_at, now()), updated_at = now() where id = $1",
      [data.conversationId],
    );
  }
}

async function firstEnabledProvider(pool: pg.Pool, orgId: string): Promise<string | null> {
  const res = await pool.query("select provider_id from provider_configs where org_id = $1 and enabled = true order by created_at limit 1", [orgId]);
  return (res.rows[0] as { provider_id: string } | undefined)?.provider_id ?? null;
}

async function escalateWithoutProvider(pool: pg.Pool, orgId: string, conversationId: string, agentId: string, note: string): Promise<void> {
  await pool.query(
    `insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'agent_ai', $3)`,
    [orgId, conversationId, "Thanks for reaching out! Connecting you with a teammate who can help."],
  );
  await pool.query(
    `update conversations set status = 'awaiting_human', handoff = $2::jsonb, updated_at = now() where id = $1`,
    [conversationId, JSON.stringify({ problem: note, recommendedNextStep: "Answer the customer - no AI provider is configured.", customerContext: {}, investigated: [], evidence: [], actionsTaken: [] })],
  );
  await writeAuditPool(pool, orgId, { actorType: "agent", actorId: agentId, type: "run.escalated", targetType: "conversation", targetId: conversationId, payload: { reason: "no_provider" } });
  await pool.query("select pg_notify($1, $2)", ["chastedesk", JSON.stringify({ orgId, conversationId, type: "conversation.updated" })]);
}
