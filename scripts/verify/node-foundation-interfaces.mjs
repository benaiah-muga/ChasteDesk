// SPDX-License-Identifier: AGPL-3.0-only
// Node-1.1: drizzle schema, shared types, and better-auth adapter tables must
// describe the same entities.
import { assert, ok } from "./lib/verify-kit.mjs";

const { readFileSync } = await import("node:fs");
const schemaText = readFileSync("packages/db/src/schema.ts", "utf8");

const REQUIRED_TABLES = [
  '"user"', '"session"', '"account"', '"verification"', '"organization"', '"member"',
  '"invitation"', '"rate_limit"', '"customers"', '"conversations"', '"messages"',
  '"knowledge_sources"', '"knowledge_chunks"', '"agents"', '"provider_configs"',
  '"api_keys"', '"widget_configs"', '"ai_runs"', '"actions"', '"audit_events"',
  '"analytics_daily"', '"rate_limit_windows"',
];
for (const t of REQUIRED_TABLES) {
  const bare = t.replace(/"/g, "");
  const pattern = new RegExp(`pgTable\\(\\s*"${bare}"`);
  assert(pattern.test(schemaText), `schema.ts missing table ${bare}`);
}

const domainTables = ["customers", "conversations", "messages", "knowledge_sources", "knowledge_chunks", "agents", "provider_configs", "api_keys", "widget_configs", "ai_runs", "actions", "analytics_daily"];
for (const t of domainTables) {
  const pattern = new RegExp(`pgTable\\(\\s*"${t}"[\\s\\S]{0,400}?orgId: text\\("org_id"\\)`);
  assert(pattern.test(schemaText), `table ${t} must declare orgId`);
}

const sharedText = readFileSync("packages/shared/src/index.ts", "utf8");
assert(sharedText.includes("SSE_CHANNEL"), "shared must export the SSE channel constant");
assert(sharedText.includes('"owner", "admin", "agent"'), "shared must declare the role catalog");

const rbacText = readFileSync("apps/web/src/lib/rbac.ts", "utf8");
assert(/owner:\s*\[/.test(rbacText), "rbac matrix must cover the owner role");
assert(/admin:\s*\[/.test(rbacText), "rbac matrix must cover the admin role");
assert(/agent:\s*\[/.test(rbacText), "rbac matrix must cover the agent role");

const authText = readFileSync("apps/web/src/lib/auth.ts", "utf8");
assert(authText.includes("organization()"), "auth config must mount the organization plugin");
assert(authText.includes("drizzleAdapter"), "auth must use the drizzle adapter");
assert(authText.includes('provider: "pg"'), "adapter must target postgres");

let negativeCaught = false;
try {
  assert(schemaText.includes("pgTable(\"definitely_missing_table\""), "probe");
} catch {
  negativeCaught = true;
}
assert(negativeCaught, "interface assertions cannot fail");

ok("foundation interfaces verification passed");
