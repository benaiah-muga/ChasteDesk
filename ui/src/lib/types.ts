// SPDX-License-Identifier: AGPL-3.0-only
export type Role = "owner" | "admin" | "agent";

export interface Permissions {
  canAudit: boolean;
  canSettings: boolean;
  canSettingsWrite: boolean;
  canKnowledgeWrite: boolean;
  canInbox: boolean;
}

export interface OnboardingState {
  complete: boolean;
  providerConfigured: boolean;
  knowledgeAdded: boolean;
  providerCount: number;
  sourceCount: number;
}

export interface DeskCounts {
  openConversations: number;
  awaitingHuman: number;
  slaBreached: number;
  pendingApprovals: number;
  customers: number;
  sources: number;
  readySources: number;
  failedSources: number;
  enabledProviders: number;
  widgetOrigins: number;
}

export interface SessionUser {
  id: string;
  name: string;
  email: string;
  role: Role;
  initial: string;
}

export interface AppSession {
  authenticated: boolean;
  setupRequired: boolean;
  user?: SessionUser;
  org?: { id: string; name: string; initial: string };
  permissions?: Permissions;
  onboarding?: OnboardingState;
  counts?: DeskCounts;
}

export interface CustomerRef {
  id: string;
  name: string;
  email: string;
  initial: string;
}

export type TicketPriority = "low" | "normal" | "high" | "urgent";

export interface ConversationListItem {
  id: string;
  ticketNumber: number;
  priority: TicketPriority;
  tags: string[];
  snoozedUntil?: string | null;
  firstResponseDueAt?: string | null;
  resolutionDueAt?: string | null;
  slaBreached?: boolean;
  status: string;
  statusLabel: string;
  channel: string;
  channelLabel: string;
  createdAt: string;
  updatedAt: string;
  firstResponseAt?: string | null;
  customer: CustomerRef;
  preview: string;
  lastRole: string;
  assigned: { id: string; name: string } | null;
  pendingApprovals: number;
}

export interface ConversationListResponse {
  conversations: ConversationListItem[];
  counts: Record<string, number>;
  availableTags: { name: string; count: number }[];
  members: Member[];
  total: number;
}

export interface Member {
  id: string;
  name: string;
  email: string;
  role: string;
  initial: string;
  self: boolean;
}

export interface Citation {
  sourceName?: string;
  sourceId?: string;
  version?: number;
  quote?: string;
}

export interface Message {
  id: string;
  role: string;
  content: string;
  createdAt: string;
  citations?: Citation[];
}

export interface Action {
  id: string;
  conversationId: string;
  toolId: string;
  args: unknown;
  status: string;
  createdAt: string;
  decidedAt?: string | null;
  customer?: CustomerRef;
}

export interface HandoffEvidence {
  source: string;
  version: number;
  quote: string;
}

export interface Handoff {
  problem?: string;
  recommendedNextStep?: string;
  investigated?: string[];
  evidence?: (HandoffEvidence | string)[];
  actionsTaken?: string[];
}

export interface ConversationDetail {
  conversation: {
    id: string;
    ticketNumber: number;
    priority: TicketPriority;
    tags: string[];
    snoozedUntil?: string | null;
    firstResponseDueAt?: string | null;
    resolutionDueAt?: string | null;
    slaBreached?: boolean;
    customerId: string;
    status: string;
    channel: string;
    createdAt: string;
    updatedAt: string;
    firstResponseAt?: string | null;
    resolvedAt?: string | null;
    closedAt?: string | null;
    assignedUserId?: string | null;
    assignedName?: string;
    handoff?: Handoff;
  };
  customer: {
    id: string;
    name: string;
    email: string;
    createdAt?: string;
    attributes: Record<string, unknown>;
  };
  messages: Message[];
  recentConversations: { id: string; status: string; createdAt: string }[];
  openConversations: number;
  actions: Action[];
}

export interface Overview {
  greeting: string;
  today: string;
  status: { ok: boolean; text: string };
  counts: DeskCounts;
  metrics: {
    aiResolutionRate: number | null;
    p50FirstResponseSeconds: number | null;
    p50ResolutionSeconds: number | null;
    totalConversations: number | null;
    latestDate: string | null;
  };
  recent: ConversationListItem[];
  attention: AttentionItem[];
  readiness: ReadinessItem[];
  statusMix: { status: string; label: string; count: number }[];
  trend: { date: string; total: number }[];
}

export interface AttentionItem {
  kind: "approval" | "human" | "source" | "provider" | "knowledge" | "sla";
  title: string;
  body: string;
  href: string;
  count: number;
  severity: "danger" | "warn" | "info";
}

export interface ReadinessItem {
  key: string;
  title: string;
  body: string;
  done: boolean;
  href: string;
  action: string;
}

export interface ApprovalRow {
  id: string;
  conversationId: string;
  toolId: string;
  args: unknown;
  status: string;
  createdAt: string;
  decidedAt?: string | null;
  customer: CustomerRef;
}

export interface ApprovalsResponse {
  pending: ApprovalRow[];
  history: ApprovalRow[];
}

export interface KnowledgeSource {
  id: string;
  type: string;
  name: string;
  scope: string;
  status: string;
  version: number;
  error: string | null;
  embeddingModel: string | null;
  chunks: number;
  vector: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface KnowledgeResponse {
  sources: KnowledgeSource[];
  counts: {
    total: number;
    ready: number;
    processing: number;
    failed: number;
    chunks: number;
    vectorSearch: boolean;
  };
  canWrite: boolean;
}

export interface AnalyticsResponse {
  days: number;
  metrics: {
    total_conversations: number;
    ai_resolution_rate: number;
    escalation_rate: number;
    p50_first_response_seconds: number | null;
    p50_resolution_seconds: number | null;
  } | null;
  trend: { date: string; total: number }[];
  statusMix: { status: string; label: string; count: number; percent: number }[];
  openNow: number;
  latestDate: string | null;
}

export interface CustomerRow {
  id: string;
  name: string;
  email: string;
  initial: string;
  attributes: Record<string, string>;
  createdAt: string;
  lastSeen: string | null;
  total: number;
  open: number;
  lastStatus: string | null;
  lastChannel: string | null;
}

export interface CustomersResponse {
  customers: CustomerRow[];
  query: string;
  totalOpen: number;
  availablePlans: string[];
}

export interface CustomerDetailResponse {
  customer: CustomerRow;
  conversations: {
    id: string;
    status: string;
    channel: string;
    createdAt: string;
    updatedAt: string;
    preview: string;
  }[];
  openCount: number;
  totalCount: number;
}

export interface AuditEvent {
  id: number;
  type: string;
  actorType: string;
  actorId: string | null;
  actorName: string;
  targetType: string;
  targetId: string | null;
  payload: unknown;
  createdAt: string;
}

export interface AuditResponse {
  events: AuditEvent[];
  types: string[];
  hasMore: boolean;
  nextBefore: number;
  members: Member[];
}

export interface SettingsResponse {
  org: { id: string; name: string };
  user: { id: string; name: string; email: string; role: Role };
  canManage: boolean;
  canAdmin: boolean;
  slaPolicies: SLAPolicy[];
  scim: SCIMSettings;
  providers: {
    id: string;
    providerId: string;
    displayName: string;
    keyLast4: string;
    baseUrl: string;
    modelCount: number;
    enabled: boolean;
  }[];
  providerOptions: { id: string; label: string; keyless: boolean; description: string }[];
  agent: AgentForm;
  widget: {
    origins: string[];
    enabled: boolean;
    displayName: string;
    greeting: string;
    primaryColor: string;
    position: "right" | "left";
    embedSnippet: string;
  };
  apiKeys: { id: string; name: string; prefix: string; lastUsed: string; createdAt: string }[];
  members: { id: string; name: string; email: string; role: string; joined: string; self: boolean }[];
  invitations: { id: string; email: string; role: string; expires: string; link: string }[];
}

export interface AgentForm {
  purpose: string;
  publicScope: boolean;
  internalScope: boolean;
  tools: { id: string; description: string; mode: string }[];
  keywords: string;
  threshold: string;
  providerId: string;
  modelId: string;
  maxSteps: string;
  maxToolCalls: string;
  maxInputTokens: string;
  maxOutputTokens: string;
  providers: { id: string; label: string }[];
  providerModels: { providerId: string; label: string; models: { id: string; displayName: string }[] }[];
  configuredProviders: number;
  hasKnowledge: boolean;
  hasEmbedding: boolean;
}

export interface SearchResponse {
  conversations: ConversationListItem[];
  customers: CustomerRow[];
  sources: KnowledgeSource[];
}

export interface InvitationPreview {
  email: string;
  role: string;
  orgName: string;
  expiresAt: string;
}


export interface AccountResponse {
  id: string;
  name: string;
  email: string;
  role: Role;
  initial: string;
  twoFactorEnabled: boolean;
}

export interface LoginResponse {
  user?: SessionUser;
  twoFactorRequired?: boolean;
  challengeToken?: string;
}

export interface TwoFactorSetupResponse {
  secret: string;
  otpauthUri: string;
}


export interface CannedReply {
  id: string;
  title: string;
  body: string;
  createdAt: string;
  updatedAt: string;
}

export interface CannedRepliesResponse {
  replies: CannedReply[];
}

export interface SLAPolicy {
  id: string;
  priority: TicketPriority;
  firstResponseMinutes: number;
  resolutionMinutes: number;
  enabled: boolean;
}

export interface SCIMToken {
  id: string;
  name: string;
  createdAt: string;
  lastUsedAt?: string | null;
}

export interface SCIMSettings {
  baseUrl: string;
  tokens: SCIMToken[];
  canManage: boolean;
}
