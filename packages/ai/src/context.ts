// SPDX-License-Identifier: AGPL-3.0-only

export interface RunContext {
  orgId: string;
  conversationId: string;
  agentId: string;
  runId: string;
  customerId: string;
  channel: "widget" | "api";
}
