// SPDX-License-Identifier: AGPL-3.0-only
export const INGEST_ALLOW_PRIVATE_ENV = "CHASTESK_INGEST_ALLOW_PRIVATE";

export function allowPrivateIngestion(nodeEnv: string | undefined): boolean {
  if (nodeEnv === "production") return false;
  return process.env[INGEST_ALLOW_PRIVATE_ENV] === "1";
}
