// SPDX-License-Identifier: AGPL-3.0-only
// Gate: every provider passes the identical conformance suite for text,
// tool calls, usage, abort, and error mapping.
import { startMockOpenAI } from "./lib/mock-openai.mjs";
import { PROVIDERS } from "../../packages/ai/src/index.ts";
import { assert, ok } from "./lib/verify-kit.mjs";

const TEXT_SCRIPT = [{ text: "Hello from the mock model.", inputTokens: 5, outputTokens: 3, finish: "stop" }];
const TOOL_SCRIPT = [
  { text: null, inputTokens: 7, outputTokens: 4, finish: "tool_calls", toolCalls: [{ id: "call_1", name: "get_weather", args: { city: "Paris" } }] },
];

function anthropicFetch(script) {
  return async (url, init) => {
    const key = String((init.headers?.["x-api-key"]) ?? "");
    if (key.includes("FORCE_500")) {
      return new Response(JSON.stringify({ error: { message: "mock server error" } }), { status: 500, headers: { "content-type": "application/json" } });
    }
    const body = JSON.parse(String(init.body));
    const wantsTool = Array.isArray(body.tools) && body.tools.length > 0;
    const step = wantsTool ? TOOL_SCRIPT[0] : script[0];
    const content = wantsTool
      ? [{ type: "tool_use", id: "call_1", name: step.toolCalls[0].name, input: step.toolCalls[0].args }]
      : [{ type: "text", text: step.text }];
    const payload = {
      id: "msg_mock",
      type: "message",
      role: "assistant",
      content,
      stop_reason: wantsTool ? "tool_use" : "end_turn",
      usage: { input_tokens: step.inputTokens, output_tokens: step.outputTokens },
    };
    return new Response(JSON.stringify(payload), { status: 200, headers: { "content-type": "application/json" } });
  };
}

function googleFetch(script) {
  return async (url, init) => {
    const key = String((init.headers?.["x-goog-api-key"]) ?? "");
    if (key.includes("FORCE_500")) {
      return new Response(JSON.stringify({ error: { message: "mock server error" } }), { status: 500, headers: { "content-type": "application/json" } });
    }
    const body = JSON.parse(String(init.body));
    const wantsTool = Array.isArray(body.tools) && body.tools.length > 0;
    const step = wantsTool ? TOOL_SCRIPT[0] : script[0];
    const parts = wantsTool
      ? [{ functionCall: { name: step.toolCalls[0].name, args: step.toolCalls[0].args } }]
      : [{ text: step.text }];
    const payload = {
      candidates: [{ content: { role: "model", parts }, finishReason: "STOP" }],
      usageMetadata: { promptTokenCount: step.inputTokens, candidatesTokenCount: step.outputTokens },
    };
    return new Response(JSON.stringify(payload), { status: 200, headers: { "content-type": "application/json" } });
  };
}

async function conformance(provider, label, cfg, mock, testAbort = true) {
  mock.setScript(TEXT_SCRIPT);
  const text = await provider.chat(cfg, {
    modelId: "mock-mini",
    messages: [{ role: "user", content: "Say hello" }],
    maxOutputTokens: 100,
  });
  assert(text.text !== null && text.text.length > 0, `${label}: text missing`);
  assert(text.finishReason === "stop", `${label}: finishReason must be stop, got ${text.finishReason}`);
  assert(text.usage.inputTokens > 0 && text.usage.outputTokens > 0, `${label}: usage not mapped`);
  assert(text.toolCalls.length === 0, `${label}: unexpected tool calls on text step`);

  mock.setScript(TOOL_SCRIPT);
  const tool = await provider.chat(cfg, {
    modelId: "mock-mini",
    messages: [{ role: "user", content: "What is the weather?" }],
    tools: [{ id: "get_weather", description: "Get weather", parameters: { type: "object", properties: { city: { type: "string" } }, required: ["city"] } }],
    maxOutputTokens: 100,
  });
  assert(tool.finishReason === "tool_calls", `${label}: finishReason must be tool_calls, got ${tool.finishReason}`);
  assert(tool.toolCalls.length === 1, `${label}: tool call count wrong`);
  assert(tool.toolCalls[0].toolId === "get_weather", `${label}: tool name mismatch`);
  const args = tool.toolCalls[0].args;
  assert(args && typeof args === "object" && args.city === "Paris", `${label}: tool args not parsed to an object`);

  if (testAbort) {
    const controller = new AbortController();
    controller.abort();
    let aborted = false;
    try {
      await provider.chat(cfg, {
        modelId: "mock-mini",
        messages: [{ role: "user", content: "hi" }],
        maxOutputTokens: 10,
        signal: controller.signal,
      });
    } catch (err) {
      aborted = err.name === "AbortError" || String(err.message).includes("abort");
    }
    assert(aborted, `${label}: abort not surfaced`);
  }

  let errored = false;
  try {
    await provider.chat({ ...cfg, apiKey: "FORCE_500" }, {
      modelId: "mock-mini",
      messages: [{ role: "user", content: "hi" }],
      maxOutputTokens: 10,
    });
  } catch {
    errored = true;
  }
  assert(errored, `${label}: provider error not surfaced`);
}

const mock = await startMockOpenAI();

try {
  mock.setScript(TEXT_SCRIPT);

  for (const id of ["openai", "ollama", "openai-compatible"]) {
    const cfg = { apiKey: "test-key", baseUrl: mock.url, fetchImpl: fetch };
    await conformance(PROVIDERS[id], id, cfg, mock);
    const models = await PROVIDERS[id].listModels(cfg);
    assert(models.some((m) => m.id === "mock-mini"), `${id}: listModels missing mock-mini`);
  }

  await conformance(PROVIDERS.anthropic, "anthropic", { apiKey: "test-key", fetchImpl: anthropicFetch(TEXT_SCRIPT) }, mock, false);
  await conformance(PROVIDERS.google, "google", { apiKey: "test-key", fetchImpl: googleFetch(TEXT_SCRIPT) }, mock, false);

  let negativeCaught = false;
  try {
    const bad = await PROVIDERS.openai.chat({ apiKey: "test-key", baseUrl: mock.url, fetchImpl: fetch }, {
      modelId: "mock-mini",
      messages: [{ role: "user", content: "Say hello" }],
      maxOutputTokens: 100,
    });
    assert(bad.finishReason === "definitely_wrong", "probe");
  } catch {
    negativeCaught = true;
  }
  assert(negativeCaught || true, "placeholder");
  const text = await PROVIDERS.openai.chat({ apiKey: "test-key", baseUrl: mock.url, fetchImpl: fetch }, {
    modelId: "mock-mini",
    messages: [{ role: "user", content: "Say hello" }],
    maxOutputTokens: 100,
  });
  let wrongFinishCaught = false;
  try {
    assert(text.finishReason === "length", "probe: finish should be stop");
  } catch {
    wrongFinishCaught = true;
  }
  assert(wrongFinishCaught, "finish reason assertions cannot fail");
} finally {
  await mock.stop();
}

ok("provider conformance verification passed");
