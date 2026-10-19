// SPDX-License-Identifier: AGPL-3.0-only
// Gate: the crawler blocks loopback, private, link-local, and metadata addresses
// including redirect targets, while still fetching a public control URL.
import http from "node:http";
import dns from "node:dns/promises";
import { isPrivateAddress, validateIngestUrl } from "../../packages/shared/src/ingestion/ssrf.ts";
import { allowPrivateIngestion } from "../../packages/shared/src/ingestion/policy.ts";
import { assert, ok } from "./lib/verify-kit.mjs";

const lookup = async (host) => {
  const real = await dns.lookup(host, { all: true, verbatim: true }).catch(() => []);
  if (real.length > 0) return real;
  if (/^\d+\.\d+\.\d+\.\d+$/.test(host)) return [{ address: host, family: 4 }];
  return [];
};

const BLOCKED = [
  ["http://127.0.0.1/x", "127.0.0.1"],
  ["http://localhost/x", "127.0.0.1"],
  ["http://10.1.2.3/x", "10.1.2.3"],
  ["http://172.16.3.4/x", "172.16.3.4"],
  ["http://192.168.1.1/x", "192.168.1.1"],
  ["http://169.254.169.254/latest/meta-data/", "169.254.169.254"],
  ["http://100.64.0.1/x", "100.64.0.1"],
  ["http://0.0.0.0/x", "0.0.0.0"],
  ["http://[::1]/x", "::1"],
  ["http://[fd00::1]/x", "fd00::1"],
  ["http://[fe80::1]/x", "fe80::1"],
  ["http://[::ffff:127.0.0.1]/x", "::ffff:127.0.0.1"],
];

for (const [url, ip] of BLOCKED) {
  assert(isPrivateAddress(ip), `isPrivateAddress must flag ${ip}`);
  let blocked = false;
  try {
    await validateIngestUrl(url, { allowPrivate: false }, async () => [{ address: ip, family: ip.includes(":") ? 6 : 4 }]);
  } catch (err) {
    blocked = err.name === "BlockedAddressError";
  }
  assert(blocked, `crawler did not block ${url}`);
}

const PUBLIC_IP = "93.184.215.14";
let allowed = false;
try {
  const target = await validateIngestUrl(`http://${PUBLIC_IP}/x`, { allowPrivate: false }, async () => [
    { address: PUBLIC_IP, family: 4 },
  ]);
  allowed = target.pinnedIp === PUBLIC_IP;
} catch {
  allowed = false;
}
assert(allowed, "crawler must allow a public address (positive control)");

process.env.CHASTESK_INGEST_ALLOW_PRIVATE = "1";
assert(allowPrivateIngestion(undefined) === true, "test escape hatch must enable private ingestion outside production");
assert(allowPrivateIngestion("production") === false, "escape hatch must be inert when NODE_ENV=production");
delete process.env.CHASTESK_INGEST_ALLOW_PRIVATE;
assert(allowPrivateIngestion(undefined) === false, "private ingestion must default to denied");

const server = http.createServer((req, res) => {
  if (req.url === "/redirect-private") {
    res.writeHead(302, { location: "http://0.0.0.0:9/meta-data" });
    res.end();
    return;
  }
  if (req.url === "/redirect-ok") {
    res.writeHead(302, { location: "/final" });
    res.end();
    return;
  }
  res.writeHead(200, { "content-type": "text/html" });
  res.end("<html><head><title>Local Fixture</title></head><body><h1>Fixture</h1><p>reachable</p></body></html>");
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));
const port = server.address().port;

try {
  process.env.CHASTESK_INGEST_ALLOW_PRIVATE = "1";
  const { crawlUrl } = await import("../../apps/worker/src/crawler/crawler.ts");

  const ok1 = await crawlUrl(`http://127.0.0.1:${port}/`);
  assert(ok1.title === "Local Fixture", `crawl of allowed local target failed: ${ok1.title}`);
  assert(ok1.markdown.includes("Fixture"), "crawled content missing");

  let redirectBlocked = false;
  try {
    await crawlUrl(`http://127.0.0.1:${port}/redirect-private`);
  } catch (err) {
    redirectBlocked = err.name === "BlockedAddressError";
  }
  assert(redirectBlocked, "redirect to private address must be blocked even with the test escape hatch");

  const redirected = await crawlUrl(`http://127.0.0.1:${port}/redirect-ok`);
  assert(redirected.finalUrl.endsWith("/final"), "same-origin redirect must be followed");
} finally {
  delete process.env.CHASTESK_INGEST_ALLOW_PRIVATE;
  await new Promise((r) => server.close(r));
}

ok("ssrf guard verification passed");
