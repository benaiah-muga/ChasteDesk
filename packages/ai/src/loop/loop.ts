// SPDX-License-Identifier: AGPL-3.0-only
import { z } from "zod";
import type { ChatMessage, ChatRequest, ChatResult, ToolDeclaration } from "../model-port.ts";
import type { Evidence, RetrievalEngine } from "@chastedesk/retrieval";
import type { AgentPolicy } from "@chastedesk/shared";
import type { RunContext } from "../context.ts";
import { bestEvidenceConfidence, escalationCheck, type EscalationReason } from "../policy.ts";
import { buildHandoffPackage, type HandoffPackage } from "../handoff.ts";
import { buildSystemPrompt, extractCitations, hasHallucinatedCitation } from "./prompts.ts";
import { appendAudit, finishRun, publishEvent, startRun, type RunPersistenceDeps } from "../runs.ts";

export const loopInputSchema = z.object({
  customerMessage: z.string().min(1),
  history: z.array(z.object({ role: z.enum(["customer", "agent_ai", "agent_human"]), content: z.string() })).default([]),
  customer: z.object({ name: z.string().nullable(), email: z.string().nullable(), attributes: z.record(z.string(), z.unknown()).default({}) }),
  recentConversations: z.array(z.string()).default([]),
});

export interface LoopTool {
  declaration: ToolDeclaration;
  mode: "autonomous" | "approval_required" | "disabled";
  execute: (args: unknown) => Promise<{ ok: boolean; data?: unknown; error?: string }>;
}

export interface LoopDeps extends RunPersistenceDeps {
  chat: (req: ChatRequest) => Promise<ChatResult>;
  retrieval?: RetrievalEngine;
  tools: LoopTool[];
}

export interface AgentTurnResult {
  outcome: "answered" | "awaiting_approval" | "escalated" | "failed";
  escalationReason?: EscalationReason;
  handoff?: HandoffPackage;
  citations: Array<{ marker: string; chunkId: string }>;
  evidence: Evidence[];
  steps: Array<Record<string, unknown>>;
  usage: { inputTokens: number; outputTokens: number; steps: number };
  error?: string;
}

const HOLDING_MESSAGE = "I'm checking with a teammate on this one - you'll hear from us shortly.";

export async function runAgentTurn(
  ctx: RunContext,
  policy: AgentPolicy,
  input: z.infer<typeof loopInputSchema>,
  deps: LoopDeps,
): Promise<AgentTurnResult> {
  const steps: Array<Record<string, unknown>> = [];
  const usage = { inputTokens: 0, outputTokens: 0, steps: 0 };
  await startRun(deps, ctx);

  let lastEvidence: Evidence[] = [];

  const escalate = async (reason: EscalationReason, recommendedNextStep: string, extra?: Partial<AgentTurnResult>): Promise<AgentTurnResult> => {
    const handoff = buildHandoffPackage({
      customerMessage: input.customerMessage,
      customer: input.customer,
      recentConversations: input.recentConversations,
      investigated: steps.filter((s) => s.kind === "tool").map((s) => String(s.toolId)),
      evidence: lastEvidence,
      actionsTaken: steps.filter((s) => s.kind === "tool" && s.executed === true).map((s) => String(s.toolId)),
      recommendedNextStep,
    });
    await deps.pool.query(
      "update conversations set status = 'awaiting_human', handoff = $2::jsonb, updated_at = now() where id = $1 and org_id = $3",
      [ctx.conversationId, JSON.stringify(handoff), ctx.orgId],
    );
    await finishRun(deps, ctx, { status: "escalated", outcome: reason, steps, evidence: lastEvidence, usage });
    await appendAudit(deps, ctx, "run.escalated", "conversation", ctx.conversationId, { reason, recommendedNextStep });
    await publishEvent(deps, { orgId: ctx.orgId, conversationId: ctx.conversationId, type: "conversation.updated" });
    return { outcome: "escalated", escalationReason: reason, handoff, citations: [], evidence: lastEvidence, steps, usage, ...extra };
  };

  // Pre-loop escalation: policy keywords and explicit human requests.
  const preflight = escalationCheck(input.customerMessage, policy, null);
  if (preflight === "policy_keyword" || preflight === "explicit_request") {
    return escalate(preflight, "A human should review this request directly.");
  }

  // Retrieval within the agent's knowledge scopes.
  if (deps.retrieval) {
    lastEvidence = await deps.retrieval.search(ctx.orgId, { text: input.customerMessage, scopes: policy.knowledgeScopes, limit: 8 });
  }
  const bestConfidence = bestEvidenceConfidence(lastEvidence);
  steps.push({ kind: "retrieval", evidence: lastEvidence.length, bestConfidence });

  const lowConfidence = bestConfidence !== null && bestConfidence < policy.escalation.lowConfidenceThreshold;
  const noEvidence = deps.retrieval !== undefined && lastEvidence.length === 0;
  if (lowConfidence || noEvidence) {
    return escalate("low_confidence", "The knowledge base does not confidently cover this question; a human should answer it.");
  }

  const messages: ChatMessage[] = [
    { role: "system", content: buildSystemPrompt({ purpose: policy.purpose, knowledgeScopes: policy.knowledgeScopes, escalationKeywords: policy.escalation.keywords }, lastEvidence) },
    ...input.history.map((m) => ({ role: m.role === "customer" ? "user" : "assistant", content: m.content }) as ChatMessage),
    { role: "user", content: input.customerMessage },
  ];

  const declarations: ToolDeclaration[] = deps.tools.map((t) => t.declaration);
  let answerText: string | null = null;
  let toolCallCount = 0;

  for (let step = 1; step <= policy.budgets.maxSteps; step++) {
    if (usage.inputTokens >= policy.budgets.maxInputTokens || usage.outputTokens >= policy.budgets.maxOutputTokens) {
      return escalate("budget_exhausted", "The AI ran out of its processing budget; a human should take over.");
    }
    let result: ChatResult;
    try {
      const request: ChatRequest = {
        modelId: "agent",
        messages,
        tools: declarations,
        maxOutputTokens: policy.budgets.maxOutputTokens,
      };
      result = await deps.chat(request);
    } catch (err) {
      return escalate("tool_failure", "The AI provider failed; a human should handle this conversation.", { error: err instanceof Error ? err.message : String(err) });
    }
    usage.inputTokens += result.usage.inputTokens;
    usage.outputTokens += result.usage.outputTokens;
    usage.steps++;
    steps.push({ kind: "model", finishReason: result.finishReason, text: result.text?.slice(0, 300) ?? null, toolCalls: result.toolCalls.map((c) => c.toolId) });

    if (result.finishReason === "tool_calls" && result.toolCalls.length > 0) {
      messages.push({ role: "assistant", content: JSON.stringify(result.toolCalls.map((c) => ({ id: c.id, toolId: c.toolId, args: c.args }))) });
      for (const call of result.toolCalls) {
        toolCallCount++;
        if (toolCallCount > policy.budgets.maxToolCalls) {
          return escalate("budget_exhausted", "Too many tool attempts; a human should take over.");
        }
        const tool = deps.tools.find((t) => t.declaration.id === call.toolId);
        if (!tool) {
          steps.push({ kind: "tool_refused", toolId: call.toolId, reason: "not_configured" });
          await appendAudit(deps, ctx, "run.step", "tool", call.toolId, { refused: "not_configured" });
          messages.push({ role: "user", content: `Tool ${call.toolId} is not available. Continue without it or escalate.` });
          continue;
        }
        if (tool.mode === "disabled") {
          steps.push({ kind: "tool_denied", toolId: call.toolId });
          await appendAudit(deps, ctx, "run.step", "tool", call.toolId, { denied: true });
          return escalate("denied_tool", "The requested action is not permitted for the AI; a human should handle it.");
        }
        if (tool.mode === "approval_required") {
          await deps.pool.query(
            `insert into actions (org_id, run_id, conversation_id, agent_id, tool_id, args, status)
             values ($1, $2, $3, $4, $5, $6::jsonb, 'pending_approval')`,
            [ctx.orgId, ctx.runId, ctx.conversationId, ctx.agentId, call.toolId, JSON.stringify(call.args)],
          );
          steps.push({ kind: "tool_pending_approval", toolId: call.toolId });
          await appendAudit(deps, ctx, "action.requested", "action", call.toolId, { args: call.args });
          await deps.pool.query(
            `insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'agent_ai', $3)`,
            [ctx.orgId, ctx.conversationId, HOLDING_MESSAGE],
          );
          await finishRun(deps, ctx, { status: "awaiting_approval", outcome: "awaiting_approval", steps, evidence: lastEvidence, usage });
          await publishEvent(deps, { orgId: ctx.orgId, conversationId: ctx.conversationId, type: "action.pending" });
          return { outcome: "awaiting_approval", citations: [], evidence: lastEvidence, steps, usage };
        }
        const toolResult = await tool.execute(call.args);
        steps.push({ kind: "tool", toolId: call.toolId, executed: true, ok: toolResult.ok });
        await appendAudit(deps, ctx, "run.step", "tool", call.toolId, { ok: toolResult.ok });
        messages.push({
          role: "tool",
          content: JSON.stringify(toolResult.data ?? { error: toolResult.error ?? "failed" }),
          toolCallId: call.id,
          toolName: call.toolId,
        });
      }
      continue;
    }

    answerText = result.text;
    break;
  }

  if (answerText === null || answerText.trim() === "") {
    return escalate("budget_exhausted", "The AI could not produce an answer; a human should take over.");
  }

  // Citation enforcement.
  const cited = extractCitations(answerText, lastEvidence.length);
  const hallucinated = hasHallucinatedCitation(answerText, lastEvidence.length);
  if ((cited.length === 0 && lastEvidence.length > 0) || hallucinated) {
    messages.push({ role: "user", content: "Your answer was missing valid [E#] citations or used invalid ones. Answer again, citing the evidence you actually used. If you cannot answer from the evidence, say so." });
    const retry = await deps.chat({
      modelId: "agent",
      messages,
      maxOutputTokens: policy.budgets.maxOutputTokens,
    });
    usage.inputTokens += retry.usage.inputTokens;
    usage.outputTokens += retry.usage.outputTokens;
    answerText = retry.text;
    const retriedCitations = answerText ? extractCitations(answerText, lastEvidence.length) : [];
    const retriedHallucinated = answerText ? hasHallucinatedCitation(answerText, lastEvidence.length) : true;
    if (!answerText || (retriedCitations.length === 0 && lastEvidence.length > 0) || retriedHallucinated) {
      return escalate("missing_citations", "The AI could not ground its answer in the knowledge base; a human should answer.");
    }
  }

  const finalCitations = extractCitations(answerText, lastEvidence.length);
  const citations = finalCitations.map((n) => ({ marker: `E${n}`, chunkId: lastEvidence[n - 1]?.chunkId ?? "" }));
  const citationsJson = citations.map((c) => {
    const ev = lastEvidence[Number(c.marker.slice(1)) - 1];
    return { chunkId: ev?.chunkId, sourceId: ev?.sourceId, version: ev?.version, locator: ev?.locator, quote: ev?.content.slice(0, 200) };
  });

  await deps.pool.query(
    `insert into messages (org_id, conversation_id, role, content, citations) values ($1, $2, 'agent_ai', $3, $4::jsonb)`,
    [ctx.orgId, ctx.conversationId, answerText, JSON.stringify(citationsJson)],
  );
  await finishRun(deps, ctx, { status: "completed", outcome: "answered", steps, evidence: lastEvidence, usage });
  await appendAudit(deps, ctx, "run.completed", "ai_run", ctx.runId, { citations: citations.length });
  await publishEvent(deps, { orgId: ctx.orgId, conversationId: ctx.conversationId, type: "message.created" });

  return { outcome: "answered", citations, evidence: lastEvidence, steps, usage };
}

