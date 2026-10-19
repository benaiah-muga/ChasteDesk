// SPDX-License-Identifier: AGPL-3.0-only
import type { SessionCtx } from "./auth";
import { AppError } from "./app-error";

export type Capability =
  | "settings.read"
  | "settings.write"
  | "knowledge.read"
  | "knowledge.write"
  | "audit.read"
  | "inbox.use"
  | "org.admin";

const MATRIX: Record<string, Capability[]> = {
  owner: [
    "settings.read",
    "settings.write",
    "knowledge.read",
    "knowledge.write",
    "audit.read",
    "inbox.use",
    "org.admin",
  ],
  admin: [
    "settings.read",
    "settings.write",
    "knowledge.read",
    "knowledge.write",
    "audit.read",
    "inbox.use",
  ],
  agent: ["knowledge.read", "inbox.use"],
};

export function can(ctx: SessionCtx, capability: Capability): boolean {
  return MATRIX[ctx.role]?.includes(capability) ?? false;
}

export function requireCapability(ctx: SessionCtx | null, capability: Capability): SessionCtx {
  if (!ctx) {
    throw new AppError("unauthorized", "authentication required", 401);
  }
  if (!can(ctx, capability)) {
    throw new AppError("forbidden", `role ${ctx.role} lacks ${capability}`, 403);
  }
  return ctx;
}
