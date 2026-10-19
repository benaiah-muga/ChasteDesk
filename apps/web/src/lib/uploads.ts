// SPDX-License-Identifier: AGPL-3.0-only
import { writeFile, mkdir } from "node:fs/promises";
import path from "node:path";
import { randomBytes } from "node:crypto";

export const UPLOADS_DIR = process.env.UPLOADS_DIR ?? "data/uploads";

const ALLOWED = ["pdf", "md", "txt", "docx"];

export function extensionOf(filename: string): string | null {
  const ext = filename.split(".").pop()?.toLowerCase() ?? "";
  return ALLOWED.includes(ext) ? ext : null;
}

export async function saveUpload(file: File): Promise<{ storedName: string; originalName: string; size: number }> {
  const ext = extensionOf(file.name);
  if (!ext) throw new Error(`file type not allowed: ${file.name}`);
  if (file.size > 10 * 1024 * 1024) throw new Error("file exceeds 10 MiB");
  await mkdir(UPLOADS_DIR, { recursive: true });
  const storedName = `${randomBytes(12).toString("hex")}.${ext}`;
  const buffer = Buffer.from(await file.arrayBuffer());
  await writeFile(path.join(UPLOADS_DIR, storedName), buffer);
  return { storedName, originalName: file.name, size: buffer.length };
}
