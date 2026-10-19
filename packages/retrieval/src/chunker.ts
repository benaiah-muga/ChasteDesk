// SPDX-License-Identifier: AGPL-3.0-only
import { createHash } from "node:crypto";
import type { ChunkLocator } from "./port.ts";

export interface Chunk {
  id: string;
  sourceId: string;
  version: number;
  chunkIndex: number;
  content: string;
  locator: ChunkLocator;
  tokenCount: number;
}

const TARGET_MIN_TOKENS = 500;
const TARGET_MAX_TOKENS = 800;
const OVERLAP_TOKENS = 80;

export function estimateTokens(text: string): number {
  return Math.ceil(text.length / 4);
}

export function chunkId(sourceId: string, version: number, index: number): string {
  return createHash("sha1").update(`${sourceId}:${version}:${index}`).digest("hex");
}

interface Block {
  text: string;
  isTable: boolean;
  headings: string[];
  start: number;
  end: number;
}

export function parseBlocks(markdown: string): Block[] {
  const lines = markdown.split("\n");
  const blocks: Block[] = [];
  const headings: string[] = [];
  let charCursor = 0;
  let buffer: string[] = [];
  let bufferStart = 0;
  let bufferHeadings: string[] = [...headings];

  const flush = () => {
    if (buffer.length === 0) return;
    const text = buffer.join("\n");
    blocks.push({
      text,
      isTable: buffer.every((l) => l.trim() === "" || l.trim().startsWith("|")),
      headings: [...bufferHeadings],
      start: bufferStart,
      end: bufferStart + text.length,
    });
    buffer = [];
  };

  for (const line of lines) {
    const headingMatch = /^(#{1,6})\s+(.*)$/.exec(line);
    if (headingMatch && headingMatch[1] && headingMatch[2]) {
      flush();
      const level = headingMatch[1].length;
      headings.length = level - 1;
      headings.push(headingMatch[2].trim());
      bufferHeadings = [...headings];
      charCursor += line.length + 1;
      continue;
    }
    const startingTable =
      buffer.length > 0 && buffer.every((l) => l.trim() === "" || l.trim().startsWith("|"));
    const lineIsTable = line.trim().startsWith("|");
    if (buffer.length > 0 && !startingTable && lineIsTable) {
      flush();
      bufferHeadings = [...headings];
    }
    if (buffer.length === 0) {
      bufferStart = charCursor;
      bufferHeadings = [...headings];
    }
    buffer.push(line);
    charCursor += line.length + 1;
    if (line.trim() === "" && buffer.some((l) => l.trim() !== "")) {
      flush();
    }
  }
  flush();
  return blocks.filter((b) => b.text.trim() !== "");
}

function hardSplit(text: string, maxChars: number, overlapChars: number): string[] {
  if (text.length <= maxChars) return [text];
  const pieces: string[] = [];
  let cursor = 0;
  while (cursor < text.length) {
    let end = Math.min(cursor + maxChars, text.length);
    if (end < text.length) {
      const lastBreak = text.lastIndexOf("\n", end);
      if (lastBreak > cursor + maxChars / 2) end = lastBreak;
    }
    pieces.push(text.slice(cursor, end));
    if (end >= text.length) break;
    cursor = end - overlapChars > cursor ? end - overlapChars : end;
  }
  return pieces;
}

export function chunkMarkdown(sourceId: string, version: number, markdown: string): Chunk[] {
  const blocks = parseBlocks(markdown);
  const chunks: Chunk[] = [];
  let index = 0;
  let carry: { text: string; headings: string[]; start: number; end: number } | null = null;

  const emit = (text: string, headings: string[], charRange: [number, number]) => {
    const trimmed = text.trim();
    if (!trimmed) return;
    chunks.push({
      id: chunkId(sourceId, version, index),
      sourceId,
      version,
      chunkIndex: index,
      content: trimmed,
      locator: { headings, charRange },
      tokenCount: estimateTokens(trimmed),
    });
    index++;
  };

  for (const block of blocks) {
    const currentCarry = carry as { text: string; headings: string[]; start: number; end: number } | null;
    if (block.isTable) {
      const tableWithCarry = currentCarry
        ? estimateTokens(currentCarry.text) + estimateTokens(block.text)
        : estimateTokens(block.text);
      if (tableWithCarry <= TARGET_MAX_TOKENS) {
        carry = {
          text: currentCarry ? `${currentCarry.text}\n\n${block.text}` : block.text,
          headings: block.headings,
          start: currentCarry?.start ?? block.start,
          end: block.end,
        };
        continue;
      }
      if (currentCarry) {
        emit(currentCarry.text, currentCarry.headings, [currentCarry.start, currentCarry.end]);
        carry = null;
      }
      const pieces = hardSplit(block.text, TARGET_MAX_TOKENS * 4, OVERLAP_TOKENS * 4);
      for (const piece of pieces) {
        emit(piece, block.headings, [block.start, block.end]);
      }
      continue;
    }

    let pieces = hardSplit(block.text, TARGET_MAX_TOKENS * 4, OVERLAP_TOKENS * 4);
    if (
      currentCarry &&
      estimateTokens(currentCarry.text) + estimateTokens(pieces[0] ?? "") <= TARGET_MAX_TOKENS &&
      pieces[0] !== undefined
    ) {
      pieces = [`${currentCarry.text}\n\n${pieces[0]}`, ...pieces.slice(1)];
      carry = null;
    } else if (currentCarry) {
      emit(currentCarry.text, currentCarry.headings, [currentCarry.start, currentCarry.end]);
      carry = null;
    }

    for (let i = 0; i < pieces.length; i++) {
      const piece = pieces[i];
      if (piece === undefined) continue;
      const size = estimateTokens(piece);
      const isLast = i === pieces.length - 1;
      if (isLast && size < TARGET_MIN_TOKENS) {
        carry = {
          text: piece,
          headings: block.headings,
          start: block.start,
          end: block.end,
        };
      } else {
        emit(piece, block.headings, [block.start, block.end]);
      }
    }
  }
  if (carry) {
    emit(carry.text, carry.headings, [carry.start, carry.end]);
  }
  return chunks;
}

export function chunkPlainText(sourceId: string, version: number, text: string): Chunk[] {
  return chunkMarkdown(sourceId, version, text);
}
