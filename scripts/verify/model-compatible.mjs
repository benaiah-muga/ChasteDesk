// SPDX-License-Identifier: AGPL-3.0-only
// Gate: ollama and openai-compatible providers work against the mock server
// including base-url override.
import { startMockOpenAI } from "./lib/mock-openai.mjs";
import { PROVIDERS } from "../../packages/ai/src/index.ts";
import { assert, ok } from "./lib/verify-kit.mjs";

const mock = await startMockOpenAI();

try {
  mock.setScript([{ text: "Compatible provider reply.", inputTokens: 4, outputTokens: 6, finish: "stop" }]);

  for (const id of ["ollama", "openai-compatible"]) {
    const cfg = { apiKey: id === "ollama" ? undefined : "test-key", baseUrl: mock.url, fetchImpl: fetch };
    const result = await PROVIDERS[id].chat(cfg, {
      modelId: "mock-mini",
      messages: [{ role: "user", content: "hi" }],
      maxOutputTokens: 50,
    });
    assert(result.text === "Compatible provider reply.", `${id}: text mismatch`);
    assert(result.usage.outputTokens === 6, `${id}: usage mismatch`);
    const models = await PROVIDERS[id].listModels(cfg);
    assert(models.length === 2, `${id}: model list length wrong`);
  }

  const recorded = mock.requests.filter((r) => r.body && r.body.model === "mock-mini");
  assert(recorded.length >= 2, "mock server did not see both provider requests");

  let negativeCaught = false;
  try {
    assert(mock.requests.length >= 999, "probe");
  } catch {
    negativeCaught = true;
  }
  assert(negativeCaught, "recording assertions cannot fail");
} finally {
  await mock.stop();
}

ok("compatible provider verification passed");
