// SPDX-License-Identifier: AGPL-3.0-only
import type { AgentPolicy } from "@chastedesk/shared";
import type { Evidence } from "@chastedesk/retrieval";

export type EscalationReason =
  | "policy_keyword"
  | "explicit_request"
  | "low_confidence"
  | "denied_tool"
  | "budget_exhausted"
  | "missing_citations"
  | "tool_failure";

export function escalationCheck(
  customerMessage: string,
  policy: AgentPolicy,
  bestConfidence: number | null,
): EscalationReason | null {
  const text = customerMessage.toLowerCase();
  for (const keyword of policy.escalation.keywords) {
    if (keyword.length > 0 && text.includes(keyword.toLowerCase())) return "policy_keyword";
  }
  if (/(talk to|speak to|speak with|want) a (human|real person|agent|person)/.test(text) || text.includes("human agent")) {
    return "explicit_request";
  }
  if (bestConfidence !== null && bestConfidence < policy.escalation.lowConfidenceThreshold) {
    return "low_confidence";
  }
  return null;
}

export function bestEvidenceConfidence(evidence: Evidence[]): number | null {
  if (evidence.length === 0) return null;
  return Math.max(...evidence.map((e) => e.confidence));
}
