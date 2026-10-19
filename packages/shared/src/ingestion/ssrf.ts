// SPDX-License-Identifier: AGPL-3.0-only
import { isIP } from "node:net";

export interface AddressPolicy {
  allowPrivate: boolean;
}

export class BlockedAddressError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "BlockedAddressError";
  }
}

function ipv4ToBytes(ip: string): number[] | null {
  const parts = ip.split(".");
  if (parts.length !== 4) return null;
  const bytes: number[] = [];
  for (const part of parts) {
    const n = Number(part);
    if (!Number.isInteger(n) || n < 0 || n > 255) return null;
    bytes.push(n);
  }
  return bytes;
}

function inCidr4(bytes: number[], prefix: number[], bits: number): boolean {
  const fullBytes = Math.floor(bits / 8);
  const remBits = bits % 8;
  for (let i = 0; i < fullBytes; i++) {
    const b = bytes[i];
    const p = prefix[i];
    if (b === undefined || p === undefined || b !== p) return false;
  }
  if (remBits > 0) {
    const b = bytes[fullBytes];
    const p = prefix[fullBytes];
    if (b === undefined || p === undefined) return false;
    const mask = 256 - 2 ** (8 - remBits);
    if ((b & mask) !== (p & mask)) return false;
  }
  return true;
}

export function isPrivateAddress(ip: string): boolean {
  const v4 = ipv4ToBytes(ip);
  if (v4 && isIP(ip) === 4) {
    return (
      inCidr4(v4, [0, 0, 0, 0], 8) ||
      inCidr4(v4, [10, 0, 0, 0], 8) ||
      inCidr4(v4, [100, 64, 0, 0], 10) ||
      inCidr4(v4, [127, 0, 0, 0], 8) ||
      inCidr4(v4, [169, 254, 0, 0], 16) ||
      inCidr4(v4, [172, 16, 0, 0], 12) ||
      inCidr4(v4, [192, 168, 0, 0], 16) ||
      inCidr4(v4, [192, 0, 0, 0], 24)
    );
  }
  const lower = ip.toLowerCase();
  if (lower === "::" || lower === "::1") return true;
  const v4mapped = /^::ffff:(\d+\.\d+\.\d+\.\d+)$/i.exec(lower);
  if (v4mapped && v4mapped[1]) return isPrivateAddress(v4mapped[1]);
  if (lower.startsWith("fe80:")) return true;
  if (/^f[cd][0-9a-f]{2}:/.test(lower)) return true;
  return false;
}

export interface ValidatedTarget {
  url: URL;
  pinnedIp: string;
}

export async function validateIngestUrl(
  rawUrl: string,
  policy: AddressPolicy,
  lookup: (host: string) => Promise<Array<{ address: string; family: number }>>,
): Promise<ValidatedTarget> {
  let url: URL;
  try {
    url = new URL(rawUrl);
  } catch {
    throw new BlockedAddressError(`invalid url: ${rawUrl}`);
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new BlockedAddressError(`scheme not allowed: ${url.protocol}`);
  }
  const addresses = await lookup(url.hostname);
  if (addresses.length === 0) {
    throw new BlockedAddressError(`no addresses resolved for ${url.hostname}`);
  }
  for (const { address } of addresses) {
    if (!policy.allowPrivate && isPrivateAddress(address)) {
      throw new BlockedAddressError(`address ${address} for ${url.hostname} is not a public target`);
    }
  }
  const pinnedIp = addresses[0]?.address;
  if (!pinnedIp) {
    throw new BlockedAddressError(`no usable address for ${url.hostname}`);
  }
  return { url, pinnedIp };
}
