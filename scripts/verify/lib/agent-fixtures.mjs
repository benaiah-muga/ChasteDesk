// SPDX-License-Identifier: AGPL-3.0-only
// Shared fixtures and a deterministic mock ModelProvider for agent gates.
import { randomUUID } from "node:crypto";

export const AGENT_POLICY = {
  purpose: "Answer customer questions from the knowledge base and escalate sensitive requests.",
  knowledgeScopes: ["public"],
  tools: [
    { toolId: "knowledge.search", mode: "autonomous" },
    { toolId: "customers.lookup", mode: "autonomous" },
    { toolId: "conversations.add_note", mode: "autonomous" },
    { toolId: "escalate", mode: "autonomous" },
  ],
  escalation: {
    lowConfidenceThreshold: 0.35,
    keywords: ["refund", "cancel my account"],
    alwaysEscalateTopics: [],
  },
  model: { providerId: "mock", modelId: "mock-mini" },
  budgets: { maxSteps: 6, maxToolCalls: 4, maxInputTokens: 8000, maxOutputTokens: 2000 },
};

export function makeMockProvider() {
  const calls = [];
  let script = [{ text: "Here is the answer from the knowledge base. [E1]", inputTokens: 10, outputTokens: 5, finish: "stop" }];
  const provider = {
    id: "mock",
    async listModels() {
      return [{ id: "mock-mini", displayName: "Mock Mini", supportsTools: true, supportsEmbedding: false }];
    },
    async embed() {
      throw new Error("mock provider cannot embed");
    },
    async chat(cfg, req) {
      calls.push({ messages: req.messages, tools: req.tools, aborted: Boolean(req.signal?.aborted) });
      if (req.signal?.aborted) {
        const err = new Error("This operation was aborted");
        err.name = "AbortError";
        throw err;
      }
      if (cfg?.apiKey === "FORCE_500") throw new Error("provider unavailable");
      const step = script.length > 1 ? script.shift() : script[0];
      return {
        text: step.text ?? null,
        toolCalls: (step.toolCalls ?? []).map((tc, i) => ({ id: tc.id ?? `call_${i}`, toolId: tc.toolId, args: tc.args ?? {} })),
        usage: { inputTokens: step.inputTokens ?? 5, outputTokens: step.outputTokens ?? 3 },
        finishReason: step.finish ?? "stop",
      };
    },
  };
  return {
    provider,
    setScript: (s) => {
      script = s;
    },
    calls,
  };
}

export async function seedAgentScene(pool, { policy = AGENT_POLICY, keywords } = {}) {
  const orgId = `org-${randomUUID().slice(0, 8)}`;
  const agentId = randomUUID();
  const customerId = randomUUID();
  const conversationId = randomUUID();
  await pool.query("insert into organization (id, name, slug) values ($1, 'Agent Org', $2)", [orgId, `agent-${orgId.slice(4, 12)}`]);
  const effectivePolicy = keywords
    ? { ...policy, escalation: { ...policy.escalation, keywords } }
    : policy;
  await pool.query(
    `insert into agents (id, org_id, name, policy, is_active) values ($1, $2, 'Support agent', $3::jsonb, true)`,
    [agentId, orgId, JSON.stringify(effectivePolicy)],
  );
  await pool.query(
    `insert into customers (id, org_id, email, name, attributes) values ($1, $2, 'cust@agent.test', 'Cora Customer', $3::jsonb)`,
    [customerId, orgId, JSON.stringify({ tier: "growth" })],
  );
  await pool.query(
    `insert into conversations (id, org_id, customer_id, channel, status) values ($1, $2, $3, 'widget', 'ai_active')`,
    [conversationId, orgId, customerId],
  );
  await pool.query(
    `insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'customer', $3)`,
    [orgId, conversationId, "I need help with my account."],
  );
  return { orgId, agentId, customerId, conversationId };
}

export async function seedKnowledgeChunk(pool, orgId, content, { sourceName = "Help Docs" } = {}) {
  const sourceId = randomUUID();
  await pool.query(
    `insert into knowledge_sources (id, org_id, type, name, scope, status, version) values ($1, $2, 'text', $3, 'public', 'ready', 1)`,
    [sourceId, orgId, sourceName],
  );
  const chunkId = randomUUID();
  await pool.query(
    `insert into knowledge_chunks (id, org_id, source_id, version, chunk_index, content, locator, token_count)
     values ($1, $2, $3, 1, 0, $4, $5::jsonb, 50)`,
    [chunkId, orgId, sourceId, content, JSON.stringify({ headings: ["Help"], charRange: [0, content.length] })],
  );
  return { sourceId, chunkId };
}
