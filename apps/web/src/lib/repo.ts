// SPDX-License-Identifier: AGPL-3.0-only
import { writeAudit } from "@chastedesk/db";
import { and, desc, eq } from "drizzle-orm";
import { agents, auditEvents, knowledgeSources, organization, providerConfigs, widgetConfigs } from "@chastedesk/db";
import type { SessionCtx } from "./auth";
import { db } from "./auth";

export async function orgExists(): Promise<boolean> {
  const rows = await db.select({ id: organization.id }).from(organization).limit(1);
  return rows.length > 0;
}

const DEFAULT_AGENT_POLICY = {
  purpose: "Answer customer questions from the connected knowledge base and escalate anything sensitive to a human.",
  knowledgeScopes: ["public"],
  tools: [
    { toolId: "knowledge.search", mode: "autonomous" },
    { toolId: "customers.lookup", mode: "autonomous" },
    { toolId: "conversations.add_note", mode: "autonomous" },
    { toolId: "escalate", mode: "autonomous" },
  ],
  escalation: {
    lowConfidenceThreshold: 0.35,
    keywords: ["refund", "cancel my account", "legal", "complaint"],
    alwaysEscalateTopics: [],
  },
  model: { providerId: "", modelId: "" },
  budgets: { maxSteps: 6, maxToolCalls: 4, maxInputTokens: 8000, maxOutputTokens: 2000 },
};

export async function seedOrgDefaults(orgId: string, actorUserId: string) {
  await db.insert(agents).values({
    orgId,
    name: "Default support agent",
    policy: DEFAULT_AGENT_POLICY,
    isActive: true,
  });
  await db.insert(widgetConfigs).values({ orgId, origins: [], theme: {} });
  await writeAudit(
    { orgId },
    { actorType: "user", actorId: actorUserId, type: "org.created", targetType: "organization", targetId: orgId, payload: {} },
    db,
  );
}

export async function getOnboardingState(ctx: SessionCtx) {
  const rows = await db.select().from(organization).where(eq(organization.id, ctx.orgId)).limit(1);
  const meta = parseMetadata(rows[0]?.metadata);
  const providers = await db
    .select({ id: providerConfigs.id })
    .from(providerConfigs)
    .where(eq(providerConfigs.orgId, ctx.orgId));
  const sources = await db
    .select({ id: knowledgeSources.id })
    .from(knowledgeSources)
    .where(eq(knowledgeSources.orgId, ctx.orgId));
  return {
    orgCreated: true,
    providerConfigured: providers.length > 0,
    knowledgeAdded: sources.length > 0,
    testSent: meta.onboarding?.testSent === true,
    complete: meta.onboarding?.complete === true,
  };
}

function parseMetadata(metadata: string | null | undefined): { onboarding?: { testSent?: boolean; complete?: boolean } } {
  if (!metadata) return {};
  try {
    return JSON.parse(metadata) as { onboarding?: { testSent?: boolean; complete?: boolean } };
  } catch {
    return {};
  }
}

export async function setOnboardingComplete(ctx: SessionCtx, complete: boolean) {
  const rows = await db.select().from(organization).where(eq(organization.id, ctx.orgId)).limit(1);
  const meta = parseMetadata(rows[0]?.metadata);
  meta.onboarding = { ...(meta.onboarding ?? {}), complete };
  await db
    .update(organization)
    .set({ metadata: JSON.stringify(meta) })
    .where(eq(organization.id, ctx.orgId));
  await writeAudit(
    ctx,
    { actorType: "user", actorId: ctx.userId, type: "org.admin", targetType: "onboarding", payload: { complete } },
    db,
  );
}

export async function listProviders(ctx: SessionCtx) {
  return db
    .select({
      id: providerConfigs.id,
      providerId: providerConfigs.providerId,
      displayName: providerConfigs.displayName,
      enabled: providerConfigs.enabled,
      keyLast4: providerConfigs.keyLast4,
      baseUrl: providerConfigs.baseUrl,
    })
    .from(providerConfigs)
    .where(eq(providerConfigs.orgId, ctx.orgId));
}

export async function listAuditEvents(ctx: SessionCtx, opts: { type?: string; limit?: number } = {}) {
  const limit = Math.min(opts.limit ?? 50, 200);
  const where = opts.type
    ? and(eq(auditEvents.orgId, ctx.orgId), eq(auditEvents.type, opts.type))
    : eq(auditEvents.orgId, ctx.orgId);
  return db
    .select({
      id: auditEvents.id,
      type: auditEvents.type,
      actorType: auditEvents.actorType,
      actorId: auditEvents.actorId,
      targetType: auditEvents.targetType,
      targetId: auditEvents.targetId,
      payload: auditEvents.payload,
      createdAt: auditEvents.createdAt,
    })
    .from(auditEvents)
    .where(where)
    .orderBy(desc(auditEvents.id))
    .limit(limit);
}

export async function getDefaultAgent(ctx: SessionCtx) {
  const rows = await db
    .select()
    .from(agents)
    .where(and(eq(agents.orgId, ctx.orgId), eq(agents.isActive, true)))
    .limit(1);
  return rows[0] ?? null;
}

export async function getWidgetConfig(ctx: SessionCtx) {
  const rows = await db.select().from(widgetConfigs).where(eq(widgetConfigs.orgId, ctx.orgId)).limit(1);
  return rows[0] ?? null;
}
