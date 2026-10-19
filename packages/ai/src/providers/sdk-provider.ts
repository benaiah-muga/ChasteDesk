// SPDX-License-Identifier: AGPL-3.0-only
import { embedMany, generateText, jsonSchema } from "ai";
import type { EmbeddingModel, LanguageModel, ModelMessage } from "ai";
import { UnsupportedCapabilityError, type ChatMessage, type ChatRequest, type ChatResult, type ModelInfo, type ModelProvider, type ProviderCfg, type ProviderId, type ToolDeclaration } from "../model-port.ts";

function convertMessages(messages: ChatMessage[]): Array<Record<string, unknown>> {
  return messages.map((m) => {
    if (m.role === "assistant" && m.toolCallId && m.toolName) {
      return {
        role: "assistant",
        content: [{ type: "tool-call", toolCallId: m.toolCallId, toolName: m.toolName, input: JSON.parse(m.content) as unknown }],
      };
    }
    if (m.role === "tool") {
      return {
        role: "tool",
        content: [{ type: "tool-result", toolCallId: m.toolCallId ?? "", toolName: m.toolName ?? "", output: m.content }],
      };
    }
    return { role: m.role, content: m.content };
  });
}

function convertTools(tools?: ToolDeclaration[]): Record<string, unknown> | undefined {
  if (!tools || tools.length === 0) return undefined;
  const set: Record<string, unknown> = {};
  for (const t of tools) {
    set[t.id] = { description: t.description, inputSchema: jsonSchema(t.parameters) };
  }
  return set;
}

export function mapFinishReason(raw: string | undefined): ChatResult["finishReason"] {
  switch (raw) {
    case "stop":
    case "end_turn":
    case "STOP":
    case "stop_sequence":
      return "stop";
    case "tool-calls":
    case "tool_calls":
    case "tool_use":
      return "tool_calls";
    case "length":
    case "max_tokens":
      return "length";
    default:
      return "error";
  }
}

export function createSdkProvider(
  id: ProviderId,
  modelFactory: (cfg: ProviderCfg) => (modelId: string) => LanguageModel,
  embeddingFactory?: (cfg: ProviderCfg) => (modelId: string) => EmbeddingModel<string>,
): ModelProvider {
  return {
    id,
    async listModels(cfg: ProviderCfg): Promise<ModelInfo[]> {
      const f = cfg.fetchImpl ?? fetch;
      const res = await f(listModelsUrl(id, cfg.baseUrl), { headers: listModelsHeaders(id, cfg.apiKey) });
      if (!res.ok) throw new Error(`listModels failed for ${id}: ${res.status}`);
      const body = (await res.json()) as {
        data?: Array<{ id: string }>;
        models?: Array<{ id?: string; name?: string }>;
      };
      const entries: Array<{ id: string }> = (body.data ?? body.models ?? []).map((m) => ({
        id: String(m.id ?? ("name" in m ? m.name : "") ?? ""),
      }));
      return entries
        .filter((mid) => mid.id.length > 0)
        .map((mid) => ({ id: mid.id, displayName: mid.id, supportsTools: true, supportsEmbedding: id !== "anthropic" }));
    },
    async embed(cfg: ProviderCfg, texts: string[]): Promise<number[][]> {
      if (!embeddingFactory) throw new UnsupportedCapabilityError("embeddings", id);
      const model = embeddingFactory(cfg)(cfg.embeddingModelId ?? "text-embedding-3-small");
      const { embeddings } = await embedMany({ model, values: texts });
      return embeddings;
    },
    async chat(cfg: ProviderCfg, req: ChatRequest): Promise<ChatResult> {
      const model = modelFactory(cfg)(req.modelId);
      const messages = convertMessages(req.messages) as unknown as ModelMessage[];
      const result = (await generateText({
        model,
        messages,
        tools: convertTools(req.tools) as never,
        maxOutputTokens: req.maxOutputTokens,
        temperature: req.temperature,
        abortSignal: req.signal,
      })) as {
        text: string | null;
        toolCalls?: Array<{ toolCallId?: string; toolName: string; input: unknown }>;
        usage?: { inputTokens?: number; outputTokens?: number };
        finishReason?: string;
      };
      return {
        text: result.text,
        toolCalls: (result.toolCalls ?? []).map((tc, i) => ({
          id: tc.toolCallId ?? `call_${i}`,
          toolId: tc.toolName,
          args: tc.input,
        })),
        usage: {
          inputTokens: result.usage?.inputTokens ?? 0,
          outputTokens: result.usage?.outputTokens ?? 0,
        },
        finishReason: mapFinishReason(result.finishReason),
      };
    },
  };
}

function listModelsUrl(id: ProviderId, baseUrl?: string): string {
  switch (id) {
    case "openai":
      return `${stripSlash(baseUrl) ?? "https://api.openai.com/v1"}/models`;
    case "anthropic":
      return `${stripSlash(baseUrl) ?? "https://api.anthropic.com"}/v1/models`;
    case "google":
      return `${stripSlash(baseUrl) ?? "https://generativelanguage.googleapis.com"}/v1beta/models`;
    case "ollama":
      return baseUrl
        ? `${stripSlash(baseUrl)}/models`
        : `${stripSlash(baseUrl) ?? "http://localhost:11434"}/api/tags`;
    case "openai-compatible":
      return `${stripSlash(baseUrl) ?? "http://localhost:8080/v1"}/models`;
  }
}

function listModelsHeaders(id: ProviderId, apiKey?: string): Record<string, string> {
  switch (id) {
    case "openai":
      return apiKey ? { Authorization: `Bearer ${apiKey}` } : {};
    case "anthropic":
      return { "x-api-key": apiKey ?? "", "anthropic-version": "2023-06-01" };
    case "google":
      return apiKey ? { "x-goog-api-key": apiKey } : {};
    case "ollama":
      return {};
    case "openai-compatible":
      return apiKey ? { Authorization: `Bearer ${apiKey}` } : {};
  }
}

function stripSlash(url?: string): string | undefined {
  return url ? url.replace(/\/+$/, "") : undefined;
}
