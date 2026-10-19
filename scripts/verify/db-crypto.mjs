// SPDX-License-Identifier: AGPL-3.0-only
// Gate: AES-256-GCM secret sealing round-trips, rejects wrong keys and tampered
// ciphertext, uses unique IVs, and masks display values.
import { maskSecret, openSecret, sealSecret } from "../../packages/db/src/crypto.ts";
import { assert, ok } from "./lib/verify-kit.mjs";

const secret = "sk-live-abcdef1234567890";
const appSecret = "unit-test-app-secret-material-0123456789";

const sealed = sealSecret(secret, appSecret);
assert(sealed.keyVersion === 1, "key version must be 1");
assert(sealed.last4 === "7890", "last4 must capture the final 4 chars");
const opened = openSecret(sealed, appSecret);
assert(opened === secret, "round-trip plaintext mismatch");

let wrongKeyFailed = false;
try {
  openSecret(sealed, "another-app-secret-entirely-0123456789");
} catch {
  wrongKeyFailed = true;
}
assert(wrongKeyFailed, "decryption with wrong key must fail");

const tampered = { ...sealed, ciphertext: Buffer.from("tampered-ciphertext-bytes").toString("base64") };
let tamperFailed = false;
try {
  openSecret(tampered, appSecret);
} catch {
  tamperFailed = true;
}
assert(tamperFailed, "tampered ciphertext must fail authentication");

const second = sealSecret(secret, appSecret);
assert(second.ciphertext !== sealed.ciphertext || second.iv !== sealed.iv, "IVs must be random per seal");

assert(maskSecret(secret).endsWith("7890"), "mask must end with last4");
assert(maskSecret("abc") === "****", "short secrets must be fully masked");

ok("secret encryption verification passed");
