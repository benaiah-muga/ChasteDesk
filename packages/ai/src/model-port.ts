// SPDX-License-Identifier: AGPL-3.0-only

export type JsonSchema7 = Record<string, unknown>;

export interface ModelInfo {
  id: string;
  displayName: string;
  supportsTools: boolean;
  supportsEmbedding: boolean;
}

export interface ProviderCfg {
  apiKey?: string;
  baseUrl?: string;
  fetchImpl?: typeof fetch;
  embeddingModelId?: string;
}

export interface ChatMessage {
  role: "system" | "user" | "assistant" | "tool";
  content: string;
  toolCallId?: string;
  toolName?: string;
}

export interface ToolDeclaration {
  id: string;
  description: string;
  parameters: JsonSchema7;
}

export interface ToolCall {
  id: string;
  toolId: string;
  args: unknown;
}

export interface ChatRequest {
  modelId: string;
  messages: ChatMessage[];
  tools?: ToolDeclaration[];
  maxOutputTokens: number;
  temperature?: number;
  signal?: AbortSignal;
}

export interface ChatUsage {
  inputTokens: number;
  outputTokens: number;
}

export type ChatFinishReason = "stop" | "tool_calls" | "length" | "error";

export interface ChatResult {
  text: string | null;
  toolCalls: ToolCall[];
  usage: ChatUsage;
  finishReason: ChatFinishReason;
}

export interface ModelProvider {
  readonly id: ProviderId;
  listModels(cfg: ProviderCfg): Promise<ModelInfo[]>;
  embed(cfg: ProviderCfg, texts: string[]): Promise<number[][]>;
  chat(cfg: ProviderCfg, req: ChatRequest): Promise<ChatResult>;
}

export const PROVIDER_IDS = ["openai", "anthropic", "google", "ollama", "openai-compatible"] as const;
export type ProviderId = (typeof PROVIDER_IDS)[number];

export class UnsupportedCapabilityError extends Error {
  constructor(capability: string, providerId: string) {
    super(`provider ${providerId} does not support ${capability}`);
    this.name = "UnsupportedCapabilityError";
  }
}
