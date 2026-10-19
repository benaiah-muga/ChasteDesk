// SPDX-License-Identifier: AGPL-3.0-only
import { createCipheriv, createDecipheriv, hkdfSync, randomBytes } from "node:crypto";

const KEY_INFO = "secrets-v1";
export const KEY_VERSION = 1;

function deriveKey(appSecret: string): Buffer {
  return Buffer.from(hkdfSync("sha256", appSecret, Buffer.alloc(0), KEY_INFO, 32));
}

export interface SealedSecret {
  ciphertext: string;
  iv: string;
  authTag: string;
  keyVersion: number;
  last4: string;
}

export function sealSecret(plaintext: string, appSecret: string): SealedSecret {
  const key = deriveKey(appSecret);
  const iv = randomBytes(12);
  const cipher = createCipheriv("aes-256-gcm", key, iv);
  const ciphertext = Buffer.concat([cipher.update(plaintext, "utf8"), cipher.final()]);
  const authTag = cipher.getAuthTag();
  return {
    ciphertext: ciphertext.toString("base64"),
    iv: iv.toString("base64"),
    authTag: authTag.toString("base64"),
    keyVersion: KEY_VERSION,
    last4: plaintext.slice(-4),
  };
}

export function openSecret(sealed: Omit<SealedSecret, "last4">, appSecret: string): string {
  const key = deriveKey(appSecret);
  const decipher = createDecipheriv("aes-256-gcm", key, Buffer.from(sealed.iv, "base64"));
  decipher.setAuthTag(Buffer.from(sealed.authTag, "base64"));
  const plaintext = Buffer.concat([
    decipher.update(Buffer.from(sealed.ciphertext, "base64")),
    decipher.final(),
  ]);
  return plaintext.toString("utf8");
}

export function maskSecret(plaintext: string): string {
  if (plaintext.length <= 4) return "****";
  return `${plaintext.slice(0, 3)}...${plaintext.slice(-4)}`;
}

export function deriveSubkey(appSecret: string, info: string): Buffer {
  return Buffer.from(hkdfSync("sha256", appSecret, Buffer.alloc(0), info, 32));
}
