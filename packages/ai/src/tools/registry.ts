// SPDX-License-Identifier: AGPL-3.0-only
import type { z } from "zod";
import { toolArgsSchemas } from "@chastedesk/shared";
import type { Evidence, PgClientLike, RetrievalEngine } from "@chastedesk/retrieval";
import type { RunContext } from "../context";
import type { ToolDeclaration } from "../model-port";

export interface ToolCtx extends RunContext {
  dryRun: boolean;
}

export interface ToolResult {
  ok: boolean;
  data?: unknown;
  error?: string;
  sideEffects: string[];
}

export type BuiltinToolId = "knowledge.search" | "customers.lookup" | "conversations.add_note" | "escalate";

export interface AgentTool {
  id: BuiltinToolId;
  description: string;
  parameters: Record<string, unknown>;
  risk: "autonomous" | "approval_required";
  execute: (ctx: ToolCtx, args: unknown) => Promise<ToolResult>;
  verify?: (ctx: ToolCtx, args: unknown) => Promise<{ passed: boolean; detail: string }>;
}

export interface BuiltinToolDeps {
  pool: PgClientLike;
  retrieval?: RetrievalEngine;
}

function parseArgs<T extends z.ZodType>(schema: T, args: unknown): z.output<T> {
  return schema.parse(args);
}

export function buildBuiltinTools(deps: BuiltinToolDeps): AgentTool[] {
  return [
    {
      id: "knowledge.search",
      description: "Search the business knowledge base for information relevant to the customer question.",
      parameters: { type: "object", properties: { query: { type: "string", description: "The search query" } }, required: ["query"] },
      risk: "autonomous",
      execute: async (ctx, args) => {
        const { query } = parseArgs(toolArgsSchemas["knowledge.search"], args);
        if (!deps.retrieval) return { ok: true, data: { evidence: [], note: "no knowledge index available" }, sideEffects: [] };
        const evidence: Evidence[] = await deps.retrieval.search(ctx.orgId, { text: query, scopes: ["public", "internal"], limit: 5 });
        return {
          ok: true,
          data: evidence.map((e) => ({ source: e.sourceName, version: e.version, quote: e.content.slice(0, 300), confidence: e.confidence })),
          sideEffects: [],
        };
      },
    },
    {
      id: "customers.lookup",
      description: "Look up the customer record for the current conversation.",
      parameters: { type: "object", properties: { query: { type: "string" } }, required: [] },
      risk: "autonomous",
      execute: async (ctx, args) => {
        parseArgs(toolArgsSchemas["customers.lookup"], args);
        const res = await deps.pool.query(
          "select c.id, c.email, c.name, c.attributes from customers c where c.id = $1 and c.org_id = $2",
          [ctx.customerId, ctx.orgId],
        );
        const row = res.rows[0];
        if (!row) return { ok: false, error: "customer not found in this organization", sideEffects: [] };
        return { ok: true, data: { id: row.id, email: row.email, name: row.name, attributes: row.attributes }, sideEffects: [] };
      },
      verify: async (ctx) => {
        const res = await deps.pool.query("select 1 from customers where id = $1 and org_id = $2", [ctx.customerId, ctx.orgId]);
        return { passed: res.rows.length > 0, detail: "customer record present" };
      },
    },
    {
      id: "conversations.add_note",
      description: "Add an internal note to the current conversation. Notes are never shown to the customer.",
      parameters: { type: "object", properties: { note: { type: "string", description: "The note text" } }, required: ["note"] },
      risk: "autonomous",
      execute: async (ctx, args) => {
        const { note } = parseArgs(toolArgsSchemas["conversations.add_note"], args);
        await deps.pool.query(
          "insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'internal_note', $3)",
          [ctx.orgId, ctx.conversationId, `[AI note] ${note}`],
        );
        return { ok: true, data: { added: true }, sideEffects: ["internal_note_added"] };
      },
      verify: async (ctx, args) => {
        const { note } = parseArgs(toolArgsSchemas["conversations.add_note"], args);
        const res = await deps.pool.query(
          "select 1 from messages where conversation_id = $1 and org_id = $2 and role = 'internal_note' and content = $3",
          [ctx.conversationId, ctx.orgId, `[AI note] ${note}`],
        );
        return { passed: res.rows.length > 0, detail: res.rows.length > 0 ? "note persisted" : "note not found" };
      },
    },
    {
      id: "escalate",
      description: "Escalate the conversation to a human teammate with a summary and a recommended next step.",
      parameters: {
        type: "object",
        properties: { reason: { type: "string" }, summary: { type: "string" }, recommendedNextStep: { type: "string" } },
        required: ["reason", "summary", "recommendedNextStep"],
      },
      risk: "autonomous",
      execute: async (ctx, args) => {
        const { reason, summary, recommendedNextStep } = parseArgs(toolArgsSchemas.escalate, args);
        void ctx;
        return { ok: true, data: { reason, summary, recommendedNextStep }, sideEffects: ["escalation_requested"] };
      },
    },
  ];
}

export type ToolDeclarationLike = ToolDeclaration;
