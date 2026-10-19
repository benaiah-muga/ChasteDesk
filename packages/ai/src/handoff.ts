// SPDX-License-Identifier: AGPL-3.0-only
import type { Evidence } from "@chastedesk/retrieval";

export interface HandoffPackage {
  problem: string;
  customerContext: {
    name: string | null;
    email: string | null;
    attributes: Record<string, unknown>;
    recentConversations: string[];
  };
  investigated: string[];
  evidence: Array<{ source: string; version: number; quote: string }>;
  actionsTaken: string[];
  recommendedNextStep: string;
}

export function buildHandoffPackage(input: {
  customerMessage: string;
  customer: { name: string | null; email: string | null; attributes: Record<string, unknown> };
  recentConversations: string[];
  investigated: string[];
  evidence: Evidence[];
  actionsTaken: string[];
  recommendedNextStep: string;
}): HandoffPackage {
  return {
    problem: input.customerMessage.slice(0, 500),
    customerContext: {
      name: input.customer.name,
      email: input.customer.email,
      attributes: input.customer.attributes,
      recentConversations: input.recentConversations.slice(0, 5),
    },
    investigated: input.investigated.slice(0, 10),
    evidence: input.evidence.slice(0, 5).map((e) => ({ source: e.sourceName, version: e.version, quote: e.content.slice(0, 200) })),
    actionsTaken: input.actionsTaken.slice(0, 10),
    recommendedNextStep: input.recommendedNextStep,
  };
}
