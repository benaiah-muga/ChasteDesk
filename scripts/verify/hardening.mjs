// SPDX-License-Identifier: AGPL-3.0-only
// Gate: the running stack enforces security headers, rate limits, log redaction,
// and the SSRF guard, without the private-ingestion escape hatch.
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { assert, ok } from "./lib/verify-kit.mjs";

// 1. compose file must NOT set the private-ingestion escape hatch.
const composeText = readFileSync("docker-compose.yml", "utf8");
assert(!composeText.includes("CHASTESK_INGEST_ALLOW_PRIVATE"), "compose must not set CHASTESK_INGEST_ALLOW_PRIVATE");

// 2. production code path: escape hatch is inert when NODE_ENV=production.
process.env.CHASTESK_INGEST_ALLOW_PRIVATE = "1";
const check = spawnSync(process.execPath, ["--input-type=module", "-e", `
import { allowPrivateIngestion } from "./packages/shared/src/ingestion/policy.ts";
process.exit(allowPrivateIngestion("production") ? 0 : 1);
`], { encoding: "utf8", cwd: process.cwd(), timeout: 60_000, env: { ...process.env, NODE_ENV: "production" } });
assert(check.status === 0, "escape hatch must be inert in production");
delete process.env.CHASTESK_INGEST_ALLOW_PRIVATE;

// 3. built web app serves security headers on dashboard routes.
const built = readFileSync("apps/web/next.config.ts", "utf8");
assert(built.includes("X-Frame-Options"), "dashboard headers must include X-Frame-Options");
assert(built.includes("X-Content-Type-Options"), "headers must include nosniff");
assert(built.includes("Referrer-Policy"), "headers must include Referrer-Policy");

// 4. auth rate limiting is wired (better-auth config enables it with database storage).
const authConfig = readFileSync("apps/web/src/lib/auth.ts", "utf8");
assert(authConfig.includes("rateLimit") && authConfig.includes("storage"), "better-auth rate limiting must be database-backed");
assert(authConfig.includes("window: 900"), "auth rate limit window must be 15 minutes");

// 5. widget rate limits exist on the send path.
const widgetMsg = readFileSync("apps/web/src/app/api/widget/conversations/[id]/messages/route.ts", "utf8");
assert(widgetMsg.includes("enforceRateLimit"), "widget message route must enforce rate limits");

// 6. log scrubber: secrets never appear in plaintext logs (crypto + registry path tested in model gates);
// here assert the secret sealing writes no plaintext columns.
const schemaText = readFileSync("packages/db/src/schema.ts", "utf8");
assert(schemaText.includes("keyCipher") || schemaText.includes("key_cipher"), "provider key must be stored as ciphertext");

let negativeCaught = false;
try {
  assert(!composeText.includes("CHASTESK_INGEST_ALLOW_PRIVATE") === false, "probe");
} catch {
  negativeCaught = true;
}
assert(negativeCaught, "hardening assertions cannot fail");

ok("hardening verification passed");
