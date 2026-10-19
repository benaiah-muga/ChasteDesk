// SPDX-License-Identifier: AGPL-3.0-only
import { createOpenAI } from "@ai-sdk/openai";
import { createAnthropic } from "@ai-sdk/anthropic";
import { createGoogleGenerativeAI } from "@ai-sdk/google";
import { createOpenAICompatible } from "@ai-sdk/openai-compatible";
import { createSdkProvider } from "./sdk-provider.ts";
import type { ModelProvider, ProviderId } from "../model-port.ts";

const makeOpenAI = (cfg: Parameters<typeof createOpenAI>[0]) => {
  const provider = createOpenAI(cfg);
  return (modelId: string) => provider.chat(modelId);
};

const makeOpenAIEmbedding = (cfg: Parameters<typeof createOpenAI>[0]) => {
  const provider = createOpenAI(cfg);
  return (modelId: string) => provider.embedding(modelId);
};

export const PROVIDERS: Record<ProviderId, ModelProvider> = {
  openai: createSdkProvider(
    "openai",
    (cfg) => (modelId: string) => makeOpenAI({ apiKey: cfg.apiKey ?? "", baseURL: cfg.baseUrl, fetch: cfg.fetchImpl })(modelId),
    (cfg) => (modelId: string) => makeOpenAIEmbedding({ apiKey: cfg.apiKey ?? "", baseURL: cfg.baseUrl, fetch: cfg.fetchImpl })(modelId),
  ),
  anthropic: createSdkProvider(
    "anthropic",
    (cfg) => (modelId: string) => createAnthropic({ apiKey: cfg.apiKey ?? "", baseURL: cfg.baseUrl, fetch: cfg.fetchImpl })(modelId),
  ),
  google: createSdkProvider(
    "google",
    (cfg) => (modelId: string) => createGoogleGenerativeAI({ apiKey: cfg.apiKey ?? "", baseURL: cfg.baseUrl, fetch: cfg.fetchImpl })(modelId),
    (cfg) => (modelId: string) => createGoogleGenerativeAI({ apiKey: cfg.apiKey ?? "", baseURL: cfg.baseUrl, fetch: cfg.fetchImpl }).textEmbeddingModel(modelId),
  ),
  ollama: createSdkProvider(
    "ollama",
    (cfg) => (modelId: string) =>
      createOpenAICompatible({ name: "ollama", baseURL: `${(cfg.baseUrl ?? "http://localhost:11434").replace(/\/+$/, "")}/v1`, apiKey: cfg.apiKey ?? "ollama", fetch: cfg.fetchImpl })(modelId),
    (cfg) => (modelId: string) =>
      createOpenAICompatible({ name: "ollama", baseURL: `${(cfg.baseUrl ?? "http://localhost:11434").replace(/\/+$/, "")}/v1`, apiKey: cfg.apiKey ?? "ollama", fetch: cfg.fetchImpl }).textEmbeddingModel(modelId),
  ),
  "openai-compatible": createSdkProvider(
    "openai-compatible",
    (cfg) => (modelId: string) =>
      createOpenAICompatible({ name: "chastedesk-compatible", baseURL: cfg.baseUrl ?? "http://localhost:8080/v1", apiKey: cfg.apiKey ?? "", fetch: cfg.fetchImpl })(modelId),
    (cfg) => (modelId: string) =>
      createOpenAICompatible({ name: "chastedesk-compatible", baseURL: cfg.baseUrl ?? "http://localhost:8080/v1", apiKey: cfg.apiKey ?? "", fetch: cfg.fetchImpl }).textEmbeddingModel(modelId),
  ),
};

export function getProvider(id: ProviderId): ModelProvider {
  return PROVIDERS[id];
}
