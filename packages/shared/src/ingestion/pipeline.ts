// SPDX-License-Identifier: AGPL-3.0-only

export interface ParsedDocument {
  title: string;
  markdown: string;
}

export interface HtmlToTextResult {
  markdown: string;
  title: string;
}

const SCRIPT_STYLE_RE = /<(script|style|noscript|svg|nav|footer|header)[\s\S]*?<\/\1>/gi;
const COMMENT_RE = /<!--[\s\S]*?-->/g;
const TAG_RE = /<[^>]+>/g;
const ENTITY_MAP: Record<string, string> = {
  "&amp;": "&",
  "&lt;": "<",
  "&gt;": ">",
  "&quot;": '"',
  "&#39;": "'",
  "&nbsp;": " ",
};

export function htmlToMarkdown(html: string): HtmlToTextResult {
  let text = html.replace(COMMENT_RE, "").replace(SCRIPT_STYLE_RE, "");
  const titleMatch = /<title[^>]*>([\s\S]*?)<\/title>/i.exec(text);
  const title = titleMatch && titleMatch[1] ? decodeEntities(titleMatch[1]).trim() : "Untitled";
  text = text
    .replace(/<h1[^>]*>/gi, "\n# ")
    .replace(/<h2[^>]*>/gi, "\n## ")
    .replace(/<h3[^>]*>/gi, "\n### ")
    .replace(/<\/h[1-6]>/gi, "\n")
    .replace(/<li[^>]*>/gi, "\n- ")
    .replace(/<\/p>|<br\s*\/?>|<\/div>|<\/tr>/gi, "\n")
    .replace(/<\/t[dh]>/gi, " | ");
  text = text.replace(TAG_RE, "");
  text = decodeEntities(text);
  text = text
    .split("\n")
    .map((l) => l.trim())
    .filter((l, i, arr) => l !== "" || (i > 0 && arr[i - 1] !== ""))
    .join("\n");
  return { markdown: text, title };
}

function decodeEntities(text: string): string {
  return text.replace(/&(amp|lt|gt|quot|#39|nbsp);/g, (m) => ENTITY_MAP[m] ?? m);
}

export interface ChunkWrite {
  id: string;
  sourceId: string;
  version: number;
  chunkIndex: number;
  content: string;
  locator: unknown;
  tokenCount: number;
}

export const CRAWLER_LIMITS = {
  maxRedirects: 3,
  maxBodyBytes: 5 * 1024 * 1024,
  timeoutMs: 15_000,
  allowedContentTypes: ["text/html", "text/plain", "text/markdown"],
  userAgent: "ChasteDeskIngest/0.1 (+https://chastedesk.local/bot)",
} as const;
