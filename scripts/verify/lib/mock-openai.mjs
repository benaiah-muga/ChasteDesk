// SPDX-License-Identifier: AGPL-3.0-only
// Scriptable OpenAI-compatible mock server for provider gates. Records every
// request; responses come from a script queue (last item repeats).
import http from "node:http";

export function startMockOpenAI({ port = 0 } = {}) {
  const requests = [];
  let script = [
    { text: "Hello from the mock model.", inputTokens: 5, outputTokens: 3, finish: "stop" },
  ];

  const server = http.createServer((req, res) => {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      const auth = String(req.headers.authorization ?? "");
      const entry = { path: req.url, auth, body: body ? JSON.parse(body) : null };
      requests.push(entry);

      if (req.url.endsWith("/models")) {
        res.writeHead(200, { "content-type": "application/json" });
        res.end(JSON.stringify({ data: [{ id: "mock-mini" }, { id: "mock-embed" }] }));
        return;
      }
      if (req.url.endsWith("/embeddings")) {
        const parsed = entry.body ?? {};
        const inputs = Array.isArray(parsed.input) ? parsed.input : [parsed.input ?? ""];
        res.writeHead(200, { "content-type": "application/json" });
        res.end(
          JSON.stringify({
            data: inputs.map((t, i) => ({ index: i, embedding: [0.1 * (i + 1), 0.2, 0.3, t.length / 100] })),
          }),
        );
        return;
      }
      if (req.url.endsWith("/chat/completions")) {
        if (auth.includes("FORCE_500")) {
          res.writeHead(500, { "content-type": "application/json" });
          res.end(JSON.stringify({ error: { message: "mock server error" } }));
          return;
        }
        const step = script.length > 1 ? script.shift() : script[0];
        const message = { role: "assistant", content: step.text ?? null };
        if (step.toolCalls && step.toolCalls.length > 0) {
          message.tool_calls = step.toolCalls.map((tc, i) => ({
            id: tc.id ?? `call_${i}`,
            type: "function",
            function: { name: tc.name, arguments: JSON.stringify(tc.args ?? {}) },
          }));
        }
        res.writeHead(200, { "content-type": "application/json" });
        res.end(
          JSON.stringify({
            id: "chatcmpl-mock",
            object: "chat.completion",
            choices: [{ index: 0, message, finish_reason: step.finish ?? "stop" }],
            usage: {
              prompt_tokens: step.inputTokens ?? 5,
              completion_tokens: step.outputTokens ?? 3,
              input_tokens: step.inputTokens ?? 5,
              output_tokens: step.outputTokens ?? 3,
              total_tokens: (step.inputTokens ?? 5) + (step.outputTokens ?? 3),
            },
          }),
        );
        return;
      }
      res.writeHead(404);
      res.end();
    });
  });

  return new Promise((resolve) => {
    server.listen(port, "127.0.0.1", () => {
      const address = server.address();
      resolve({
        url: `http://127.0.0.1:${address.port}/v1`,
        requests,
        setScript: (s) => {
          script = s;
        },
        stop: () => new Promise((r) => server.close(r)),
      });
    });
  });
}
