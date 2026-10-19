// SPDX-License-Identifier: AGPL-3.0-only
import { openSecret, type SealedSecret } from "@chastedesk/db";
import { getProvider, PROVIDERS } from "./providers/index.ts";
import type { ProviderCfg, ProviderId } from "./model-port.ts";
import { redactSecrets } from "./redact.ts";
import type pg from "pg";

export interface ResolvedProvider {
  providerId: ProviderId;
  cfg: ProviderCfg;
}

export interface ProviderConfigRow {
  provider_id: string;
  key_cipher: string | null;
  key_iv: string | null;
  key_tag: string | null;
  key_version: number | null;
  base_url: string | null;
}

export async function resolveProviderConfig(
  pool: pg.Pool,
  appSecret: string,
  orgId: string,
  providerId: ProviderId,
): Promise<ResolvedProvider> {
  const result = await pool.query(
    "select provider_id, key_cipher, key_iv, key_tag, key_version, base_url from provider_configs where org_id = $1 and provider_id = $2 and enabled = true",
    [orgId, providerId],
  );
  const row = result.rows[0] as ProviderConfigRow | undefined;
  const cfg: ProviderCfg = {};
  if (row) {
    if (row.key_cipher && row.key_iv && row.key_tag) {
      const sealed: Omit<SealedSecret, "last4"> = {
        ciphertext: row.key_cipher,
        iv: row.key_iv,
        authTag: row.key_tag,
        keyVersion: row.key_version ?? 1,
      };
      cfg.apiKey = openSecret(sealed, appSecret);
    }
    if (row.base_url) cfg.baseUrl = row.base_url;
  }
  return { providerId, cfg };
}

export function providerFor(providerId: ProviderId) {
  return getProvider(providerId);
}

export { PROVIDERS, redactSecrets };
