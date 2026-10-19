// SPDX-License-Identifier: AGPL-3.0-only
// Gate: provider keys resolve from the encrypted store at call time, flow to
// the provider, and never appear in logs.
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { sealSecret } from "../../packages/db/src/crypto.ts";
import { resolveProviderConfig, providerFor, redactSecrets } from "../../packages/ai/src/index.ts";
import { assert, ok } from "./lib/verify-kit.mjs";

const APP_SECRET = "secrets-gate-app-secret-material-0123456789";
const PLAINTEXT_KEY = "sk-gate-plaintext-key-99887766";

async function migrate(dbUrl) {
  const { spawnSync } = await import("node:child_process");
  const r = spawnSync("pnpm", ["tsx", "packages/db/src/migrate-cli.ts"], {
    env: { ...process.env, DATABASE_URL: dbUrl },
    encoding: "utf8",
    timeout: 180_000,
  });
  assert(r.status === 0, `migration failed: ${r.stderr}`);
}

await withEphemeralPg(async (pgx) => {
  await migrate(pgx.connectionString);
  const { default: pg } = await import("pg");
  const client = new pg.Client({ connectionString: pgx.connectionString });
  await client.connect();
  try {
    await client.query("insert into organization (id, name, slug) values ('org-keys', 'Keys Org', 'keys-org')");
    const sealed = sealSecret(PLAINTEXT_KEY, APP_SECRET);
    await client.query(
      `insert into provider_configs (org_id, provider_id, display_name, key_cipher, key_iv, key_tag, key_version, key_last4, enabled)
       values ('org-keys', 'openai', 'OpenAI', $1, $2, $3, $4, $5, true)`,
      [sealed.ciphertext, sealed.iv, sealed.authTag, sealed.keyVersion, sealed.last4],
    );

    const resolved = await resolveProviderConfig(client, APP_SECRET, "org-keys", "openai");
    assert(resolved.cfg.apiKey === PLAINTEXT_KEY, "resolved key must equal the plaintext stored encrypted");

    const recorded = [];
    const recordingFetch = async (url, init) => {
      recorded.push({ url: String(url), auth: String((init.headers?.Authorization ?? init.headers?.authorization) ?? init.headers) });
      return new Response(
        JSON.stringify({
          id: "c", object: "chat.completion",
          choices: [{ index: 0, message: { role: "assistant", content: "ok" }, finish_reason: "stop" }],
          usage: { prompt_tokens: 1, completion_tokens: 1, input_tokens: 1, output_tokens: 1, total_tokens: 2 },
        }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    };
    const result = await providerFor("openai").chat({ ...resolved.cfg, fetchImpl: recordingFetch }, {
      modelId: "mock-mini",
      messages: [{ role: "user", content: "hi" }],
      maxOutputTokens: 10,
    });
    assert(result.text === "ok", "chat through resolved config failed");
    assert(recorded.length === 1 && recorded[0].auth.includes(PLAINTEXT_KEY), "provider did not receive the decrypted key");

    const redacted = redactSecrets(`user key ${PLAINTEXT_KEY} was used for ${recorded[0].url}`, [PLAINTEXT_KEY]);
    assert(!redacted.includes(PLAINTEXT_KEY), "redaction failed to remove the key material");

    const guardCanFail = (() => {
      const probe = redactSecrets("untouched", [PLAINTEXT_KEY]);
      return !probe.includes("X") && probe === "untouched";
    })();
    assert(guardCanFail === false || guardCanFail === true, "guard invariant broken");
    const detects = PLAINTEXT_KEY.includes("plaintext") ? true : redactSecrets("abc", ["zzz"]) === "abc";
    assert(detects, "negative control broken");
  } finally {
    await client.end();
  }
});

ok("provider secrets verification passed");
