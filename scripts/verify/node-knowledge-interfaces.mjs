// SPDX-License-Identifier: AGPL-3.0-only
// Node-1.2: chunk and Evidence shapes written by the worker must match what
// the engine and citations read.
import { readFileSync } from "node:fs";
import { assert, ok } from "./lib/verify-kit.mjs";

const shared = readFileSync("packages/shared/src/ingestion/pipeline.ts", "utf8");
assert(shared.includes("ChunkWrite") && shared.includes("locator") && shared.includes("tokenCount"), "shared pipeline must define complete chunk write shape");
assert(shared.includes("htmlToMarkdown") || shared.includes("ParsedDocument"), "shared pipeline must expose parsing");

const hybrid = readFileSync("packages/retrieval/src/hybrid.ts", "utf8");
for (const field of ["chunkId", "sourceId", "sourceName", "version", "scope", "locator", "content", "score", "confidence"]) {
  assert(hybrid.includes(field), `builtin engine Evidence missing field ${field}`);
}

const handlers = readFileSync("apps/worker/src/jobs/ingest.ts", "utf8");
assert(handlers.includes("insert into knowledge_chunks"), "worker must write chunks");
assert(handlers.includes("locator") && handlers.includes("token_count"), "worker chunk insert must persist locator and token count");

let negativeCaught = false;
try {
  assert(hybrid.includes("definitely_not_a_field_xyz"), "probe");
} catch {
  negativeCaught = true;
}
assert(negativeCaught, "interface assertions cannot fail");

ok("knowledge interfaces verification passed");
