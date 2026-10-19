// SPDX-License-Identifier: AGPL-3.0-only
import { readFile, stat } from "node:fs/promises";
import type { ParsedDocument } from "@chastedesk/shared";

export const ALLOWED_EXTENSIONS = ["pdf", "md", "txt", "docx"] as const;
export type AllowedExtension = (typeof ALLOWED_EXTENSIONS)[number];
export const MAX_FILE_BYTES = 10 * 1024 * 1024;

export function extensionOf(filename: string): AllowedExtension | null {
  const ext = filename.split(".").pop()?.toLowerCase() ?? "";
  return (ALLOWED_EXTENSIONS as readonly string[]).includes(ext) ? (ext as AllowedExtension) : null;
}

const PDF_MAGIC = [0x25, 0x50, 0x44, 0x46];
const DOCX_MAGIC = [0x50, 0x4b, 0x03, 0x04];

function magicMatches(buffer: Buffer, magic: number[]): boolean {
  return magic.every((byte, i) => buffer[i] === byte);
}

export async function parseFile(path: string, filename: string): Promise<ParsedDocument> {
  const ext = extensionOf(filename);
  if (!ext) {
    throw new Error(`file type not allowed: ${filename}`);
  }
  const info = await stat(path);
  if (info.size > MAX_FILE_BYTES) throw new Error("file exceeds 10 MiB");
  const buffer = await readFile(path);
  if (ext === "pdf") {
    if (!magicMatches(buffer, PDF_MAGIC)) throw new Error("pdf magic bytes missing");
    const { extractText, getDocumentProxy } = await import("unpdf");
    const pdf = await getDocumentProxy(new Uint8Array(buffer));
    const { text } = await extractText(pdf, { mergePages: true });
    return { title: filename, markdown: text };
  }
  if (ext === "docx") {
    if (!magicMatches(buffer, DOCX_MAGIC)) throw new Error("docx magic bytes missing");
    const mammoth = (await import("mammoth")) as { extractRawText: (i: { buffer: Buffer }) => Promise<{ value: string }> };
    const result = await mammoth.extractRawText({ buffer });
    return { title: filename, markdown: result.value };
  }
  return { title: filename, markdown: buffer.toString("utf8") };
}
