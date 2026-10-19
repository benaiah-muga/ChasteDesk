// SPDX-License-Identifier: AGPL-3.0-only

export function redactSecrets(text: string, secrets: string[]): string {
  let out = text;
  for (const secret of secrets) {
    if (secret.length < 4) continue;
    out = out.split(secret).join("[redacted]");
  }
  return out;
}

export function createSecretGuard(secrets: string[]) {
  return {
    redact: (text: string) => redactSecrets(text, secrets),
    containsSecret: (text: string) => secrets.some((s) => s.length >= 4 && text.includes(s)),
  };
}
