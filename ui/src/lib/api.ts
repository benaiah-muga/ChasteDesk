// SPDX-License-Identifier: AGPL-3.0-only
import type {
  AnalyticsResponse,
  AppSession,
  ApprovalsResponse,
  AuditResponse,
  ConversationDetail,
  ConversationListResponse,
  CustomerDetailResponse,
  CustomersResponse,
  InvitationPreview,
  KnowledgeResponse,
  Overview,
  SearchResponse,
  SettingsResponse,
  AccountResponse,
  CannedRepliesResponse,
  SLAPolicy,
  LoginResponse,
  TwoFactorSetupResponse,
} from "./types";

export class ApiError extends Error {
  status: number;
  code: string;

  constructor(message: string, status = 0, code = "network_error") {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

interface Envelope<T> {
  data: T;
}

interface ErrorEnvelope {
  error?: { code?: string; message?: string };
}

export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !(init.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch(path, {
    ...init,
    headers,
    credentials: "same-origin",
    redirect: "error",
  });
  const contentType = response.headers.get("Content-Type") ?? "";
  if (!response.ok) {
    let message = response.statusText || "Something went wrong";
    let code = "request_failed";
    if (contentType.includes("application/json")) {
      const body = (await response.json().catch(() => ({}))) as ErrorEnvelope;
      message = body.error?.message ?? message;
      code = body.error?.code ?? code;
    }
    throw new ApiError(message, response.status, code);
  }
  if (response.status === 204) return undefined as T;
  if (!contentType.includes("application/json")) {
    return (await response.text()) as T;
  }
  const body = (await response.json()) as Envelope<T>;
  return body.data;
}

export const api = {
  session: () => request<AppSession>("/api/app/session"),
  account: () => request<AccountResponse>("/api/app/account"),
  setupTwoFactor: () => request<TwoFactorSetupResponse>("/api/app/account/two-factor/setup", { method: "POST" }),
  enableTwoFactor: (code: string) =>
    request<{ enabled: boolean; recoveryCodes: string[] }>("/api/app/account/two-factor/enable", {
      method: "POST",
      body: JSON.stringify({ code }),
    }),
  disableTwoFactor: (currentPassword: string, code: string) =>
    request<{ enabled: boolean }>("/api/app/account/two-factor/disable", {
      method: "POST",
      body: JSON.stringify({ currentPassword, code }),
    }),
  setup: (body: { orgName: string; userName: string; email: string; password: string }) =>
    request<AppSession>("/api/app/setup", { method: "POST", body: JSON.stringify(body) }),
  login: (body: { email: string; password: string }) =>
    request<LoginResponse>("/api/auth/sign-in/email", { method: "POST", body: JSON.stringify(body) }),
  verifyTwoFactor: (challengeToken: string, code: string) =>
    request<LoginResponse>("/api/auth/two-factor/verify", {
      method: "POST",
      body: JSON.stringify({ challengeToken, code }),
    }),
  logout: () => request<{ success: boolean }>("/api/auth/sign-out", { method: "POST" }),
  completeOnboarding: (complete = true) =>
    request<{ complete: boolean }>("/api/onboarding/complete", {
      method: "POST",
      body: JSON.stringify({ complete }),
    }),
  overview: () => request<Overview>("/api/app/overview"),
  inbox: (params: URLSearchParams) => request<ConversationListResponse>(`/api/app/inbox?${params.toString()}`),
  conversation: (id: string) => request<ConversationDetail>(`/api/inbox/conversations/${id}`),
  reply: (id: string, content: string, internal = false) =>
    request<{ sent: boolean; role: string }>(`/api/inbox/conversations/${id}/reply`, {
      method: "POST",
      body: JSON.stringify({ content, internal }),
    }),
  resolve: (id: string, action: "resolve" | "close" | "reopen") =>
    request<{ done: boolean }>(`/api/inbox/conversations/${id}/resolve`, {
      method: "POST",
      body: JSON.stringify({ action }),
    }),
  assign: (id: string, assignedUserId: string | null) =>
    request<{ assigned: boolean }>(`/api/inbox/conversations/${id}/assign`, {
      method: "POST",
      body: JSON.stringify({ assignedUserId }),
    }),
  bulkInbox: (
    ids: string[],
    action: "resolve" | "close" | "reopen" | "assign" | "priority" | "snooze",
    options: {
      assignedUserId?: string | null;
      priority?: string;
      snoozedUntil?: string | null;
      clearSnooze?: boolean;
    } = {},
  ) =>
    request<{ updated: string[]; failed: { id: string; message: string }[] }>("/api/app/inbox/bulk", {
      method: "POST",
      body: JSON.stringify({ ids, action, ...options }),
    }),
  updateTicket: (
    id: string,
    body: {
      priority?: string;
      tags?: string[];
      snoozedUntil?: string;
      clearSnooze?: boolean;
    },
  ) =>
    request<{ ticketNumber: number; priority: string; tags: string[]; snoozedUntil: string | null }>(
      `/api/app/inbox/${id}/ticket`,
      { method: "PATCH", body: JSON.stringify(body) },
    ),
  cannedReplies: () => request<CannedRepliesResponse>("/api/app/canned-replies"),
  createCannedReply: (title: string, body: string) =>
    request<{ id: string; title: string; body: string }>("/api/app/canned-replies", {
      method: "POST",
      body: JSON.stringify({ title, body }),
    }),
  updateCannedReply: (id: string, title: string, body: string) =>
    request<{ id: string; title: string; body: string }>(`/api/app/canned-replies/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ title, body }),
    }),
  deleteCannedReply: (id: string) =>
    request<{ deleted: boolean }>(`/api/app/canned-replies/${id}`, { method: "DELETE" }),
  decideAction: (actionId: string, decision: "approve" | "reject") =>
    request<{ status: string }>("/api/actions/decide", {
      method: "POST",
      body: JSON.stringify({ actionId, decision }),
    }),
  copilotDraft: (conversationId: string) =>
    request<{ draft: string; citations: { sourceName?: string }[]; grounded: boolean }>("/api/copilot/draft", {
      method: "POST",
      body: JSON.stringify({ conversationId }),
    }),
  approvals: () => request<ApprovalsResponse>("/api/app/approvals"),
  decideApprovalsBulk: (ids: string[], decision: "approve" | "reject") =>
    request<{ done: string[]; failed: { id: string; message: string }[] }>("/api/app/approvals/bulk", {
      method: "POST",
      body: JSON.stringify({ ids, decision }),
    }),
  knowledge: () => request<KnowledgeResponse>("/api/app/knowledge"),
  createSource: (body: Record<string, unknown> | FormData) =>
    request<{ id: string }>("/api/app/knowledge", {
      method: "POST",
      body: body instanceof FormData ? body : JSON.stringify(body),
    }),
  updateSource: (sourceId: string, patch: { scope?: string }) =>
    request<{ updated: boolean }>(`/api/app/knowledge/${sourceId}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
    }),
  reindexSource: (sourceId: string) =>
    request<{ reindexing: boolean }>(`/api/app/knowledge/${sourceId}/reindex`, { method: "POST" }),
  deleteSource: (sourceId: string) =>
    request<{ deleted: boolean }>(`/api/app/knowledge/${sourceId}`, { method: "DELETE" }),
  analytics: (days: number) => request<AnalyticsResponse>(`/api/app/analytics?days=${days}`),
  rollup: () => request<{ queued: boolean }>("/api/analytics/rollup", { method: "POST", body: "{}" }),
  customers: (params: URLSearchParams) => request<CustomersResponse>(`/api/app/customers?${params.toString()}`),
  customer: (id: string) => request<CustomerDetailResponse>(`/api/app/customers/${id}`),
  audit: (params: URLSearchParams) => request<AuditResponse>(`/api/app/audit?${params.toString()}`),
  settings: () => request<SettingsResponse>("/api/app/settings"),
  saveProvider: (body: { providerId: string; displayName: string; apiKey: string; baseUrl: string }) =>
    request<{ saved: boolean }>("/api/app/settings/providers", { method: "POST", body: JSON.stringify(body) }),
  deleteProvider: (id: string) => request<{ deleted: boolean }>(`/api/app/settings/providers/${id}`, { method: "DELETE" }),
  testProvider: (id: string) =>
    request<{ ok: boolean; error?: string; models?: { id: string; displayName: string }[] }>(
      `/api/settings/providers/${id}/connect`,
      { method: "POST", body: JSON.stringify({ configId: id }) },
    ),
  saveWidget: (body: {
    origins: string[];
    enabled: boolean;
    displayName: string;
    greeting: string;
    primaryColor: string;
    position: string;
  }) => request<unknown>("/api/app/settings/widget", { method: "POST", body: JSON.stringify(body) }),
  saveAgent: (body: Record<string, unknown>) =>
    request<{ saved: boolean }>("/api/app/settings/agent", { method: "POST", body: JSON.stringify(body) }),
  createApiKey: (name: string) =>
    request<{ secret: string }>("/api/app/settings/api-keys", { method: "POST", body: JSON.stringify({ name }) }),
  revokeApiKey: (id: string) =>
    request<{ deleted: boolean }>(`/api/app/settings/api-keys/${id}`, { method: "DELETE" }),
  saveWorkspace: (name: string) =>
    request<{ saved: boolean }>("/api/app/settings/workspace", { method: "POST", body: JSON.stringify({ name }) }),
  saveSLAPolicies: (policies: { priority: string; firstResponseMinutes: number; resolutionMinutes: number; enabled: boolean }[]) =>
    request<{ policies: SLAPolicy[] }>("/api/app/settings/sla", {
      method: "POST",
      body: JSON.stringify({ policies }),
    }),
  createSCIMToken: (name: string) =>
    request<{ id: string; name: string; token: string; baseUrl: string }>("/api/app/settings/scim/tokens", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  revokeSCIMToken: (id: string) =>
    request<{ revoked: boolean }>(`/api/app/settings/scim/tokens/${id}`, { method: "DELETE" }),
  updateMemberRole: (id: string, role: string) =>
    request<{ updated: boolean; role: string; sessionsRevoked: number }>(`/api/app/settings/members/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ role }),
    }),
  removeMember: (id: string) =>
    request<{ removed: boolean; sessionsRevoked: number }>(`/api/app/settings/members/${id}`, { method: "DELETE" }),
  revokeMemberSessions: (id: string) =>
    request<{ revoked: number }>(`/api/app/settings/members/${id}/revoke-sessions`, { method: "POST" }),
  saveAccount: (body: { name: string; currentPassword?: string; newPassword?: string }) =>
    request<{ saved: boolean }>("/api/app/account", { method: "POST", body: JSON.stringify(body) }),
  invite: (email: string, role: string) =>
    request<{ id: string; email: string; role: string; link: string }>("/api/app/settings/invitations", {
      method: "POST",
      body: JSON.stringify({ email, role }),
    }),
  revokeInvite: (id: string) =>
    request<{ revoked: boolean }>(`/api/app/settings/invitations/${id}`, { method: "DELETE" }),
  search: (query: string) => request<SearchResponse>(`/api/app/search?q=${encodeURIComponent(query)}`),
  invitation: (id: string) => request<InvitationPreview>(`/api/app/invitations/${id}`),
  acceptInvitation: (id: string, body: { name: string; password: string }) =>
    request<AppSession>(`/api/app/invitations/${id}/accept`, { method: "POST", body: JSON.stringify(body) }),
};
