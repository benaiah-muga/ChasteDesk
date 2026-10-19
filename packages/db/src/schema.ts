// SPDX-License-Identifier: AGPL-3.0-only
import { sql } from "drizzle-orm";
import {
  bigint,
  bigserial,
  boolean,
  customType,
  index,
  integer,
  jsonb,
  pgEnum,
  pgTable,
  text,
  timestamp,
  uniqueIndex,
  uuid,
} from "drizzle-orm/pg-core";

export const vector = customType<{ data: number[]; driverData: string }>({
  dataType() {
    return "vector";
  },
  toDriver(value: number[]) {
    return `[${value.join(",")}]`;
  },
  fromDriver(value: string): number[] {
    return value
      .trim()
      .replace(/^\[/, "")
      .replace(/\]$/, "")
      .split(",")
      .map((n) => Number(n));
  },
});

export const tsvector = customType<{ data: string; driverData: string }>({
  dataType() {
    return "tsvector";
  },
});

export const channelEnum = pgEnum("channel", ["widget", "api"]);
export const conversationStatusEnum = pgEnum("conversation_status", [
  "ai_active",
  "awaiting_human",
  "human_active",
  "resolved",
  "closed",
]);
export const messageRoleEnum = pgEnum("message_role", [
  "customer",
  "agent_ai",
  "agent_human",
  "internal_note",
  "system",
]);
export const knowledgeSourceTypeEnum = pgEnum("knowledge_source_type", ["url", "file", "text"]);
export const knowledgeScopeEnum = pgEnum("knowledge_scope", ["public", "internal"]);
export const knowledgeStatusEnum = pgEnum("knowledge_status", [
  "pending",
  "fetching",
  "indexing",
  "ready",
  "failed",
]);
export const actionStatusEnum = pgEnum("action_status", [
  "pending_approval",
  "approved",
  "rejected",
  "executing",
  "executed",
  "failed",
  "rejected_before_exec",
]);
export const runStatusEnum = pgEnum("run_status", [
  "running",
  "completed",
  "escalated",
  "failed",
  "awaiting_approval",
]);
export const auditActorEnum = pgEnum("audit_actor", ["user", "agent", "system"]);

const ts = () => timestamp("created_at", { withTimezone: true }).notNull().defaultNow();
const uts = () => timestamp("updated_at", { withTimezone: true }).notNull().defaultNow();

export const user = pgTable("user", {
  id: text("id").primaryKey(),
  name: text("name").notNull(),
  email: text("email").notNull().unique(),
  emailVerified: boolean("email_verified").notNull().default(false),
  image: text("image"),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow(),
});

export const session = pgTable("session", {
  id: text("id").primaryKey(),
  expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
  token: text("token").notNull().unique(),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow(),
  ipAddress: text("ip_address"),
  userAgent: text("user_agent"),
  userId: text("user_id")
    .notNull()
    .references(() => user.id, { onDelete: "cascade" }),
});

export const account = pgTable("account", {
  id: text("id").primaryKey(),
  accountId: text("account_id").notNull(),
  providerId: text("provider_id").notNull(),
  issuer: text("issuer").notNull(),
  userId: text("user_id")
    .notNull()
    .references(() => user.id, { onDelete: "cascade" }),
  accessToken: text("access_token"),
  refreshToken: text("refresh_token"),
  idToken: text("id_token"),
  accessTokenExpiresAt: timestamp("access_token_expires_at", { withTimezone: true }),
  refreshTokenExpiresAt: timestamp("refresh_token_expires_at", { withTimezone: true }),
  scope: text("scope"),
  password: text("password"),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow(),
});

export const verification = pgTable("verification", {
  id: text("id").primaryKey(),
  identifier: text("identifier").notNull(),
  value: text("value").notNull(),
  expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow(),
});

export const organization = pgTable("organization", {
  id: text("id").primaryKey(),
  name: text("name").notNull(),
  slug: text("slug").notNull().unique(),
  logo: text("logo"),
  metadata: text("metadata"),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
});

export const member = pgTable("member", {
  id: text("id").primaryKey(),
  organizationId: text("organization_id")
    .notNull()
    .references(() => organization.id, { onDelete: "cascade" }),
  userId: text("user_id")
    .notNull()
    .references(() => user.id, { onDelete: "cascade" }),
  role: text("role").notNull().default("agent"),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
});

export const invitation = pgTable("invitation", {
  id: text("id").primaryKey(),
  organizationId: text("organization_id")
    .notNull()
    .references(() => organization.id, { onDelete: "cascade" }),
  email: text("email").notNull(),
  role: text("role"),
  status: text("status").notNull().default("pending"),
  expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
  inviterId: text("inviter_id")
    .notNull()
    .references(() => user.id, { onDelete: "cascade" }),
});

export const rateLimit = pgTable("rate_limit", {
  id: text("id").primaryKey(),
  key: text("key").notNull().unique(),
  count: integer("count").notNull(),
  lastRequest: bigint("last_request", { mode: "number" }).notNull(),
});

export const rateLimitWindows = pgTable("rate_limit_windows", {
  key: text("key").primaryKey(),
  windowStart: timestamp("window_start", { withTimezone: true }).notNull().defaultNow(),
  count: integer("count").notNull().default(0),
});

export const customers = pgTable(
  "customers",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    orgId: text("org_id")
      .notNull()
      .references(() => organization.id, { onDelete: "cascade" }),
    email: text("email"),
    name: text("name"),
    attributes: jsonb("attributes").notNull().default({}),
    externalId: text("external_id"),
    createdAt: ts(),
    updatedAt: uts(),
  },
  (t) => [
    uniqueIndex("customers_org_email_uq")
      .on(t.orgId, sql`lower(${t.email})`)
      .where(sql`${t.email} is not null`),
    index("customers_org_idx").on(t.orgId),
  ],
);

export const conversations = pgTable(
  "conversations",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    orgId: text("org_id")
      .notNull()
      .references(() => organization.id, { onDelete: "cascade" }),
    customerId: uuid("customer_id")
      .notNull()
      .references(() => customers.id, { onDelete: "cascade" }),
    channel: channelEnum("channel").notNull(),
    status: conversationStatusEnum("status").notNull().default("ai_active"),
    assignedUserId: text("assigned_user_id").references(() => user.id, { onDelete: "set null" }),
    triage: jsonb("triage").notNull().default({}),
    handoff: jsonb("handoff"),
    firstResponseAt: timestamp("first_response_at", { withTimezone: true }),
    resolvedAt: timestamp("resolved_at", { withTimezone: true }),
    closedAt: timestamp("closed_at", { withTimezone: true }),
    createdAt: ts(),
    updatedAt: uts(),
  },
  (t) => [index("conversations_org_status_idx").on(t.orgId, t.status), index("conversations_customer_idx").on(t.customerId)],
);

export const messages = pgTable(
  "messages",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    orgId: text("org_id")
      .notNull()
      .references(() => organization.id, { onDelete: "cascade" }),
    conversationId: uuid("conversation_id")
      .notNull()
      .references(() => conversations.id, { onDelete: "cascade" }),
    role: messageRoleEnum("role").notNull(),
    content: text("content").notNull(),
    citations: jsonb("citations"),
    meta: jsonb("meta").notNull().default({}),
    createdAt: ts(),
  },
  (t) => [index("messages_conversation_idx").on(t.conversationId, t.createdAt)],
);

export const knowledgeSources = pgTable(
  "knowledge_sources",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    orgId: text("org_id")
      .notNull()
      .references(() => organization.id, { onDelete: "cascade" }),
    type: knowledgeSourceTypeEnum("type").notNull(),
    name: text("name").notNull(),
    scope: knowledgeScopeEnum("scope").notNull().default("public"),
    status: knowledgeStatusEnum("status").notNull().default("pending"),
    version: integer("version").notNull().default(1),
    error: text("error"),
    sourceConfig: jsonb("source_config").notNull().default({}),
    embeddingModel: text("embedding_model"),
    createdAt: ts(),
    updatedAt: uts(),
  },
  (t) => [index("knowledge_sources_org_idx").on(t.orgId, t.status)],
);

export const knowledgeChunks = pgTable(
  "knowledge_chunks",
  {
    id: text("id").primaryKey(),
    orgId: text("org_id")
      .notNull()
      .references(() => organization.id, { onDelete: "cascade" }),
    sourceId: uuid("source_id")
      .notNull()
      .references(() => knowledgeSources.id, { onDelete: "cascade" }),
    version: integer("version").notNull(),
    chunkIndex: integer("chunk_index").notNull(),
    content: text("content").notNull(),
    locator: jsonb("locator").notNull().default({}),
    tokenCount: integer("token_count").notNull(),
    embeddingModel: text("embedding_model"),
    embedding: vector("embedding"),
    tsv: tsvector("tsv").generatedAlwaysAs(sql`to_tsvector('english', content)`),
    createdAt: ts(),
  },
  (t) => [index("knowledge_chunks_source_idx").on(t.orgId, t.sourceId, t.version)],
);

export const agents = pgTable(
  "agents",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    orgId: text("org_id")
      .notNull()
      .references(() => organization.id, { onDelete: "cascade" }),
    name: text("name").notNull(),
    policy: jsonb("policy").notNull().default({}),
    isActive: boolean("is_active").notNull().default(true),
    createdAt: ts(),
    updatedAt: uts(),
  },
  (t) => [index("agents_org_active_idx").on(t.orgId, t.isActive)],
);

export const providerConfigs = pgTable(
  "provider_configs",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    orgId: text("org_id")
      .notNull()
      .references(() => organization.id, { onDelete: "cascade" }),
    providerId: text("provider_id").notNull(),
    displayName: text("display_name").notNull(),
    keyCipher: text("key_cipher"),
    keyIv: text("key_iv"),
    keyTag: text("key_tag"),
    keyVersion: integer("key_version"),
    keyLast4: text("key_last4"),
    baseUrl: text("base_url"),
    modelsCache: jsonb("models_cache").notNull().default([]),
    enabled: boolean("enabled").notNull().default(false),
    createdAt: ts(),
    updatedAt: uts(),
  },
  (t) => [uniqueIndex("provider_configs_org_provider_uq").on(t.orgId, t.providerId)],
);

export const apiKeys = pgTable(
  "api_keys",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    orgId: text("org_id")
      .notNull()
      .references(() => organization.id, { onDelete: "cascade" }),
    name: text("name").notNull(),
    keyPrefix: text("key_prefix").notNull(),
    keyHash: text("key_hash").notNull(),
    lastUsedAt: timestamp("last_used_at", { withTimezone: true }),
    createdAt: ts(),
  },
  (t) => [uniqueIndex("api_keys_hash_uq").on(t.keyHash)],
);

export const widgetConfigs = pgTable(
  "widget_configs",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    orgId: text("org_id")
      .notNull()
      .references(() => organization.id, { onDelete: "cascade" }),
    origins: text("origins").array().notNull().default([]),
    enabled: boolean("enabled").notNull().default(true),
    theme: jsonb("theme").notNull().default({}),
    createdAt: ts(),
    updatedAt: uts(),
  },
  (t) => [uniqueIndex("widget_configs_org_uq").on(t.orgId)],
);

export const aiRuns = pgTable(
  "ai_runs",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    orgId: text("org_id")
      .notNull()
      .references(() => organization.id, { onDelete: "cascade" }),
    conversationId: uuid("conversation_id")
      .notNull()
      .references(() => conversations.id, { onDelete: "cascade" }),
    agentId: uuid("agent_id")
      .notNull()
      .references(() => agents.id, { onDelete: "cascade" }),
    status: runStatusEnum("status").notNull().default("running"),
    outcome: text("outcome"),
    steps: jsonb("steps").notNull().default([]),
    evidence: jsonb("evidence").notNull().default([]),
    usage: jsonb("usage").notNull().default({}),
    error: text("error"),
    createdAt: ts(),
    updatedAt: uts(),
  },
  (t) => [index("ai_runs_conversation_idx").on(t.orgId, t.conversationId)],
);

export const actions = pgTable(
  "actions",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    orgId: text("org_id")
      .notNull()
      .references(() => organization.id, { onDelete: "cascade" }),
    runId: uuid("run_id")
      .notNull()
      .references(() => aiRuns.id, { onDelete: "cascade" }),
    conversationId: uuid("conversation_id")
      .notNull()
      .references(() => conversations.id, { onDelete: "cascade" }),
    agentId: uuid("agent_id")
      .notNull()
      .references(() => agents.id, { onDelete: "cascade" }),
    toolId: text("tool_id").notNull(),
    args: jsonb("args").notNull().default({}),
    status: actionStatusEnum("status").notNull().default("pending_approval"),
    result: jsonb("result"),
    verification: jsonb("verification"),
    decidedBy: text("decided_by").references(() => user.id, { onDelete: "set null" }),
    decidedAt: timestamp("decided_at", { withTimezone: true }),
    createdAt: ts(),
    updatedAt: uts(),
  },
  (t) => [index("actions_org_status_idx").on(t.orgId, t.status)],
);

export const auditEvents = pgTable(
  "audit_events",
  {
    id: bigserial("id", { mode: "number" }).primaryKey(),
    orgId: text("org_id"),
    actorType: auditActorEnum("actor_type").notNull(),
    actorId: text("actor_id"),
    type: text("type").notNull(),
    targetType: text("target_type").notNull(),
    targetId: text("target_id"),
    payload: jsonb("payload").notNull().default({}),
    createdAt: ts(),
  },
  (t) => [index("audit_events_org_type_idx").on(t.orgId, t.type), index("audit_events_created_idx").on(t.createdAt)],
);

export const analyticsDaily = pgTable(
  "analytics_daily",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    orgId: text("org_id")
      .notNull()
      .references(() => organization.id, { onDelete: "cascade" }),
    date: text("date").notNull(),
    metrics: jsonb("metrics").notNull().default({}),
    computedAt: timestamp("computed_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (t) => [uniqueIndex("analytics_daily_org_date_uq").on(t.orgId, t.date)],
);
