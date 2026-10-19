// SPDX-License-Identifier: AGPL-3.0-only
import { z } from "zod";

export const TOOL_IDS = ["knowledge.search", "customers.lookup", "conversations.add_note", "escalate"] as const;
export type ToolId = (typeof TOOL_IDS)[number];

export const toolArgsSchemas = {
  "knowledge.search": z.object({ query: z.string().min(1) }),
  "customers.lookup": z.object({ query: z.string().default("") }),
  "conversations.add_note": z.object({ note: z.string().min(1).max(4000) }),
  escalate: z.object({ reason: z.string().min(1), summary: z.string().min(1), recommendedNextStep: z.string().min(1) }),
} as const;

export type ToolMode = "autonomous" | "approval_required";
