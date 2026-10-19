// SPDX-License-Identifier: AGPL-3.0-only
import { createHmac, timingSafeEqual } from "node:crypto";
import { deriveSubkey } from "@chastedesk/db";

const APP_SECRET = () => process.env.APP_SECRET ?? "";

export interface WidgetTokenPayload {
  orgId: string;
  origins: string[];
  iat: number;
  exp: number;
}

function widgetKey(orgId: string) {
  return deriveSubkey(APP_SECRET(), `widget-${orgId}`);
}

export function signWidgetToken(orgId: string, origins: string[], now = Date.now()): string {
  const payload: WidgetTokenPayload = { orgId, origins, iat: Math.floor(now / 1000), exp: Math.floor(now / 1000) + 30 * 24 * 3600 };
  const body = Buffer.from(JSON.stringify(payload)).toString("base64url");
  const sig = createHmac("sha256", widgetKey(orgId)).update(body).digest("base64url");
  return `${body}.${sig}`;
}

export function verifyWidgetToken(token: string, orgId: string): WidgetTokenPayload | null {
  const dot = token.indexOf(".");
  if (dot <= 0) return null;
  const body = token.slice(0, dot);
  const sig = token.slice(dot + 1);
  const expected = createHmac("sha256", widgetKey(orgId)).update(body).digest("base64url");
  const a = Buffer.from(sig);
  const b = Buffer.from(expected);
  if (a.length !== b.length || !timingSafeEqual(a, b)) return null;
  return parsePayload(body, orgId);
}

export function verifyWidgetTokenAnyOrg(token: string): WidgetTokenPayload | null {
  const dot = token.indexOf(".");
  if (dot <= 0) return null;
  const body = token.slice(0, dot);
  try {
    const payload = JSON.parse(Buffer.from(body, "base64url").toString("utf8")) as WidgetTokenPayload;
    return verifyWidgetToken(token, payload.orgId);
  } catch {
    return null;
  }
}

function parsePayload(body: string, orgId: string): WidgetTokenPayload | null {
  try {
    const payload = JSON.parse(Buffer.from(body, "base64url").toString("utf8")) as WidgetTokenPayload;
    if (payload.orgId !== orgId) return null;
    if (payload.exp * 1000 < Date.now()) return null;
    return payload;
  } catch {
    return null;
  }
}

export function originAllowed(origin: string | null, origins: string[]): boolean {
  if (!origin) return false;
  const normalized = origin.replace(/\/+$/, "").toLowerCase();
  return origins.map((o) => o.replace(/\/+$/, "").toLowerCase()).includes(normalized);
}

export function signConversationToken(orgId: string, conversationId: string): string {
  const body = Buffer.from(JSON.stringify({ orgId, conversationId })).toString("base64url");
  const sig = createHmac("sha256", widgetKey(orgId)).update("conv:" + body).digest("base64url");
  return `${body}.${sig}`;
}

export function verifyConversationToken(orgId: string, conversationId: string, token: string): boolean {
  const dot = token.indexOf(".");
  if (dot <= 0) return false;
  const body = token.slice(0, dot);
  const sig = token.slice(dot + 1);
  const expected = createHmac("sha256", widgetKey(orgId)).update("conv:" + body).digest("base64url");
  const a = Buffer.from(sig);
  const b = Buffer.from(expected);
  if (a.length !== b.length || !timingSafeEqual(a, b)) return false;
  try {
    const parsed = JSON.parse(Buffer.from(body, "base64url").toString("utf8")) as { orgId: string; conversationId: string };
    return parsed.orgId === orgId && parsed.conversationId === conversationId;
  } catch {
    return false;
  }
}
