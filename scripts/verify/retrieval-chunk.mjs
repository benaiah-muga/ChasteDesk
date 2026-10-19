// SPDX-License-Identifier: AGPL-3.0-only
// Gate: fixture documents chunk deterministically into bounded chunks with
// heading breadcrumbs and intact tables.
import { chunkMarkdown, estimateTokens } from "../../packages/retrieval/src/chunker.ts";
import { buildFixtureDoc } from "./lib/retrieval-fixtures.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";

function runChunker() {
  return chunkMarkdown("src-fixture", 1, buildFixtureDoc());
}

const chunks = runChunker();
assert(chunks.length >= 3, `expected multiple chunks, got ${chunks.length}`);

const totalInputTokens = estimateTokens(buildFixtureDoc());
const totalChunkTokens = chunks.reduce((s, c) => s + c.tokenCount, 0);
assert(
  totalChunkTokens >= totalInputTokens * 0.85,
  `chunk coverage too low: ${totalChunkTokens} of ${totalInputTokens} tokens`,
);

for (const chunk of chunks) {
  assert(chunk.tokenCount <= 800, `chunk ${chunk.chunkIndex} exceeds 800 tokens (${chunk.tokenCount})`);
  assert(chunk.tokenCount >= 200, `sliver chunk ${chunk.chunkIndex} under 200 tokens (${chunk.tokenCount})`);
  assert(Array.isArray(chunk.locator.headings) && chunk.locator.headings.length > 0, `chunk ${chunk.chunkIndex} missing heading breadcrumb`);
}

const withTable = chunks.filter((c) => c.content.includes("| Plan 13 |"));
assert(withTable.length === 1, `table must be preserved inside exactly one chunk, found ${withTable.length}`);
assert(
  withTable[0].content.includes("| Plan 1 |") && withTable[0].content.includes("| Plan 25 |"),
  "table chunk is missing table rows",
);

const sectionBChunk = chunks.find((c) => c.locator.headings.includes("Section B"));
assert(sectionBChunk !== undefined, "no chunk carries the Section B breadcrumb");
assert(sectionBChunk.locator.headings[0] === "Guide", "breadcrumb root must be the document title");

const again = runChunker();
assert(JSON.stringify(again) === JSON.stringify(chunks), "chunking is not deterministic");
const idsUnique = new Set(chunks.map((c) => c.id));
assert(idsUnique.size === chunks.length, "chunk ids are not unique");

const giant = "word ".repeat(20000);
const giantChunks = chunkMarkdown("src-giant", 1, giant);
assert(giantChunks.length > 1, "oversized single block must be hard-split");
for (const chunk of giantChunks) {
  assert(chunk.tokenCount <= 800, `hard-split chunk exceeds 800 tokens (${chunk.tokenCount})`);
}
assert(giantChunks[0].content.includes("word"), "hard-split lost content");
const giantCoverage = giantChunks.reduce((s, c) => s + c.tokenCount, 0);
assert(giantCoverage >= estimateTokens(giant) * 0.9, "hard-split dropped most content");

ok("chunking verification passed");
