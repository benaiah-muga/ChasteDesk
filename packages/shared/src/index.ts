// SPDX-License-Identifier: AGPL-3.0-only
export const JOB_NAMES = {
  ingestUrl: "ingest-url",
  ingestFile: "ingest-file",
  ingestText: "ingest-text",
  embedChunks: "embed-chunks",
  aiRespond: "ai-respond",
  executeAction: "execute-action",
  analyticsRollup: "analytics-rollup",
  retentionSweep: "retention-sweep",
} as const;

export type JobName = (typeof JOB_NAMES)[keyof typeof JOB_NAMES];

export const SSE_CHANNEL = "chastedesk";

export type SseEventType = "conversation.updated" | "message.created" | "action.pending";

export interface SseEvent {
  orgId: string;
  conversationId: string;
  type: SseEventType;
}

export const ROLES = ["owner", "admin", "agent"] as const;
export type Role = (typeof ROLES)[number];

export const APP_ERROR_CODES = [
  "bad_request",
  "unauthorized",
  "forbidden",
  "not_found",
  "conflict",
  "rate_limited",
  "internal",
] as const;
export type AppErrorCode = (typeof APP_ERROR_CODES)[number];

export * from "./ingestion/ssrf.ts";
export * from "./ingestion/pipeline.ts";
export * from "./ingestion/policy.ts";
export * from "./schemas/settings.ts";
export * from "./schemas/agent.ts";
export * from "./tools.ts";
