// SPDX-License-Identifier: AGPL-3.0-only
import http from "node:http";
import https from "node:https";
import { lookup } from "node:dns/promises";
import { CRAWLER_LIMITS, allowPrivateIngestion, htmlToMarkdown, validateIngestUrl } from "@chastedesk/shared";
import type { AddressPolicy } from "@chastedesk/shared";

export interface CrawlResult {
  markdown: string;
  title: string;
  finalUrl: string;
  contentType: string;
  bytes: number;
}

function fetchHop(target: URL, pinnedIp: string, redirectsLeft: number, policy: AddressPolicy): Promise<CrawlResult> {
  return new Promise((resolve, reject) => {
    const mod = target.protocol === "https:" ? https : http;
    const req = mod.request(
      target,
      {
        method: "GET",
        headers: { "User-Agent": CRAWLER_LIMITS.userAgent, Accept: "text/html,text/plain,text/markdown,*/*;q=0.5" },
        lookup: (host, opts, cb) => {
          void host;
          void opts;
          cb(null, [{ address: pinnedIp, family: pinnedIp.includes(":") ? 6 : 4 }]);
        },
        servername: target.hostname,
        timeout: CRAWLER_LIMITS.timeoutMs,
      },
      (res) => {
        const status = res.statusCode ?? 0;
        if ([301, 302, 303, 307, 308].includes(status)) {
          res.resume();
          const location = res.headers.location;
          if (!location || redirectsLeft <= 0) {
            reject(new Error(`redirect loop or missing location at ${target.href}`));
            return;
          }
          const next = new URL(location, target);
          if (next.origin === target.origin) {
            resolve(fetchHop(next, pinnedIp, redirectsLeft - 1, policy));
            return;
          }
          resolve(
            validateIngestUrl(next.href, { allowPrivate: false }, async (h) => lookup(h, { all: true, verbatim: true })).then(
              (validated) => fetchHop(validated.url, validated.pinnedIp, redirectsLeft - 1, { allowPrivate: false }),
            ),
          );
          return;
        }
        if (status !== 200) {
          res.resume();
          reject(new Error(`unexpected status ${status} for ${target.href}`));
          return;
        }
        const contentType = String(res.headers["content-type"] ?? "")
          .split(";")[0]
          ?.trim()
          .toLowerCase() ?? "";
        if (!CRAWLER_LIMITS.allowedContentTypes.some((t) => contentType.startsWith(t))) {
          res.resume();
          reject(new Error(`content type not allowed: ${contentType}`));
          return;
        }
        const chunks: Buffer[] = [];
        let bytes = 0;
        res.on("data", (chunk: Buffer) => {
          bytes += chunk.length;
          if (bytes > CRAWLER_LIMITS.maxBodyBytes) {
            req.destroy(new Error(`body exceeds ${CRAWLER_LIMITS.maxBodyBytes} bytes`));
            return;
          }
          chunks.push(chunk);
        });
        res.on("end", () => {
          const parsed = htmlToMarkdown(Buffer.concat(chunks).toString("utf8"));
          resolve({ ...parsed, finalUrl: target.href, contentType, bytes });
        });
        res.on("error", reject);
      },
    );
    req.on("timeout", () => req.destroy(new Error(`timeout fetching ${target.href}`)));
    req.on("error", reject);
    req.end();
  });
}

export async function crawlUrl(rawUrl: string): Promise<CrawlResult> {
  const policy: AddressPolicy = { allowPrivate: allowPrivateIngestion(process.env.NODE_ENV) };
  const validated = await validateIngestUrl(rawUrl, policy, async (host) =>
    lookup(host, { all: true, verbatim: true }),
  );
  return fetchHop(validated.url, validated.pinnedIp, CRAWLER_LIMITS.maxRedirects, policy);
}
