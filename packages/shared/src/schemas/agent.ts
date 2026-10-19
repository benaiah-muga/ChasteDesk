// SPDX-License-Identifier: AGPL-3.0-only
import { z } from "zod";

export const agentPolicySchema = z.object({
  purpose: z.string().min(1),
  knowledgeScopes: z.array(z.enum(["public", "internal"])).default(["public"]),
  tools: z
    .array(
      z.object({
        toolId: z.string(),
        mode: z.enum(["autonomous", "approval_required", "disabled"]),
      }),
    )
    .default([]),
  escalation: z
    .object({
      lowConfidenceThreshold: z.number().min(0).max(1).default(0.35),
      keywords: z.array(z.string()).default([]),
      alwaysEscalateTopics: z.array(z.string()).default([]),
    })
    .default({ lowConfidenceThreshold: 0.35, keywords: [], alwaysEscalateTopics: [] }),
  model: z.object({ providerId: z.string(), modelId: z.string() }),
  budgets: z
    .object({
      maxSteps: z.number().int().min(1).max(20).default(6),
      maxToolCalls: z.number().int().min(1).max(20).default(4),
      maxInputTokens: z.number().int().min(500).default(8000),
      maxOutputTokens: z.number().int().min(200).default(2000),
    })
    .default({ maxSteps: 6, maxToolCalls: 4, maxInputTokens: 8000, maxOutputTokens: 2000 }),
});

export type AgentPolicy = z.infer<typeof agentPolicySchema>;

export const HANDOFF_SCHEMA = {
  problem: z.string(),
  customerContext: z.object({
    name: z.string().nullable(),
    email: z.string().nullable(),
    attributes: z.record(z.string(), z.unknown()).default({}),
    recentConversations: z.array(z.string()).default([]),
  }),
  investigated: z.array(z.string()).default([]),
  evidence: z
    .array(z.object({ source: z.string(), version: z.number(), quote: z.string() }))
    .default([]),
  actionsTaken: z.array(z.string()).default([]),
  recommendedNextStep: z.string(),
};
