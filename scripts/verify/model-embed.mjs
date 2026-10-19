// SPDX-License-Identifier: AGPL-3.0-only
// Gate: embedding batches embed through the SDK, unsupported providers fail
// with a typed capability error.
import { startMockOpenAI } from "./lib/mock-openai.mjs";
import { PROVIDERS, UnsupportedCapabilityError } from "../../packages/ai/src/index.ts";
import { assert, ok } from "./lib/verify-kit.mjs";

const mock = await startMockOpenAI();

try {
  const cfg = { apiKey: "test-key", baseUrl: mock.url, fetchImpl: fetch, embeddingModelId: "mock-embed" };
  const vectors = await PROVIDERS.openai.embed(cfg, ["alpha", "beta"]);
  assert(vectors.length === 2, "embedding batch size mismatch");
  for (const v of vectors) {
    assert(Array.isArray(v) && v.length === 4 && v.every((n) => typeof n === "number"), "embedding vector shape wrong");
  }

  let typedError = false;
  try {
    await PROVIDERS.anthropic.embed({ apiKey: "k" }, ["text"]);
  } catch (err) {
    typedError = err instanceof UnsupportedCapabilityError;
  }
  assert(typedError, "anthropic embed must throw UnsupportedCapabilityError");

  const embeddingRequests = mock.requests.filter((r) => r.path.endsWith("/embeddings"));
  assert(embeddingRequests.length === 1, "expected exactly one embedding request");

  let negativeCaught = false;
  try {
    assert(vectors[0].length === 999, "probe");
  } catch {
    negativeCaught = true;
  }
  assert(negativeCaught, "vector assertions cannot fail");
} finally {
  await mock.stop();
}

ok("embedding client verification passed");
