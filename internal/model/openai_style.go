// SPDX-License-Identifier: AGPL-3.0-only
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultEmbeddingModel = "text-embedding-3-small"

type openAIProvider struct {
	id ProviderID
}

func (p openAIProvider) ID() ProviderID { return p.id }

func (p openAIProvider) chatBase(cfg Config) string {
	base := stripSlash(cfg.BaseURL)
	switch p.id {
	case ProviderOpenAI:
		if base == "" {
			base = "https://api.openai.com/v1"
		}
	case ProviderOllama:
		if base == "" {
			base = "http://localhost:11434"
		}
		return base + "/v1"
	default:
		if base == "" {
			base = "http://localhost:8080/v1"
		}
	}
	return base
}

func (p openAIProvider) apiKey(cfg Config) string {
	if cfg.APIKey == "" && p.id == ProviderOllama {
		return "ollama"
	}
	return cfg.APIKey
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

func convertOpenAIMessages(messages []Message) []openAIMessage {
	out := make([]openAIMessage, 0, len(messages))
	for _, message := range messages {
		converted := openAIMessage{Role: message.Role, Content: message.Content}
		if message.Role == "assistant" && message.ToolCallID != "" && message.ToolName != "" {
			call := openAIToolCall{ID: message.ToolCallID, Type: "function"}
			call.Function.Name = message.ToolName
			call.Function.Arguments = message.Content
			converted.ToolCalls = []openAIToolCall{call}
			converted.Content = ""
		}
		if message.Role == "tool" {
			converted.ToolCallID = message.ToolCallID
		}
		out = append(out, converted)
	}
	return out
}

func convertOpenAITools(tools []ToolDeclaration) []openAITool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]openAITool, 0, len(tools))
	for _, tool := range tools {
		converted := openAITool{Type: "function"}
		converted.Function.Name = tool.ID
		converted.Function.Description = tool.Description
		converted.Function.Parameters = tool.Parameters
		out = append(out, converted)
	}
	return out
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content   *string          `json:"content"`
			ToolCalls []openAIToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		InputTokens      int `json:"input_tokens"`
		OutputTokens     int `json:"output_tokens"`
	} `json:"usage"`
}

func (p openAIProvider) Chat(ctx context.Context, cfg Config, req ChatRequest) (ChatResult, error) {
	payload := map[string]any{
		"model":      req.ModelID,
		"messages":   convertOpenAIMessages(req.Messages),
		"max_tokens": req.MaxOutputTokens,
	}
	if tools := convertOpenAITools(req.Tools); tools != nil {
		payload["tools"] = tools
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ChatResult{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.chatBase(cfg)+"/chat/completions", bytes.NewReader(encoded))
	if err != nil {
		return ChatResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	if key := p.apiKey(cfg); key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ChatResult{}, fmt.Errorf("abort: %w", ctx.Err())
		}
		return ChatResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return ChatResult{}, &APIError{ProviderID: string(p.id), Operation: "chat", Status: resp.StatusCode, Message: string(body)}
	}
	var decoded openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return ChatResult{}, err
	}
	result := ChatResult{FinishReason: FinishError}
	if len(decoded.Choices) > 0 {
		choice := decoded.Choices[0]
		result.Text = choice.Message.Content
		for i, call := range choice.Message.ToolCalls {
			id := call.ID
			if id == "" {
				id = fmt.Sprintf("call_%d", i)
			}
			var args any
			_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
			result.ToolCalls = append(result.ToolCalls, ToolCall{ID: id, ToolID: call.Function.Name, Args: args})
		}
		result.FinishReason = mapFinishReason(choice.FinishReason)
		if len(result.ToolCalls) > 0 && result.FinishReason != FinishToolCalls {
			result.FinishReason = FinishToolCalls
		}
	}
	result.Usage = Usage{
		InputTokens:  firstNonZero(decoded.Usage.PromptTokens, decoded.Usage.InputTokens),
		OutputTokens: firstNonZero(decoded.Usage.CompletionTokens, decoded.Usage.OutputTokens),
	}
	return result, nil
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func (p openAIProvider) Embed(ctx context.Context, cfg Config, texts []string) ([][]float64, error) {
	embeddingModel := cfg.EmbeddingModelID
	if embeddingModel == "" {
		embeddingModel = defaultEmbeddingModel
	}
	payload := map[string]any{"model": embeddingModel, "input": texts}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.chatBase(cfg)+"/embeddings", bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if key := p.apiKey(cfg); key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("abort: %w", ctx.Err())
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &APIError{ProviderID: string(p.id), Operation: "embed", Status: resp.StatusCode, Message: string(body)}
	}
	var decoded openAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	vectors := make([][]float64, len(decoded.Data))
	for _, entry := range decoded.Data {
		if entry.Index >= 0 && entry.Index < len(vectors) {
			vectors[entry.Index] = entry.Embedding
		}
	}
	return vectors, nil
}

type modelsEnvelope struct {
	Data   []map[string]any `json:"data"`
	Models []map[string]any `json:"models"`
}

func (e modelsEnvelope) ids() []string {
	entries := e.Data
	if entries == nil {
		entries = e.Models
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		id, _ := entry["id"].(string)
		if id == "" {
			id, _ = entry["name"].(string)
		}
		if id == "" {
			continue
		}
		out = append(out, id)
	}
	return out
}

func (p openAIProvider) listModelsURL(cfg Config) string {
	switch p.id {
	case ProviderOllama:
		if cfg.BaseURL == "" {
			return "http://localhost:11434/api/tags"
		}
		return stripSlash(cfg.BaseURL) + "/models"
	default:
		return p.chatBase(cfg) + "/models"
	}
}

func (p openAIProvider) ListModels(ctx context.Context, cfg Config) ([]Info, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.listModelsURL(cfg), nil)
	if err != nil {
		return nil, err
	}
	if key := p.apiKey(cfg); key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{ProviderID: string(p.id), Operation: "listModels", Status: resp.StatusCode}
	}
	var envelope modelsEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("listModels failed for %s: invalid response", p.id)
	}
	ids := envelope.ids()
	models := make([]Info, 0, len(ids))
	for _, id := range ids {
		models = append(models, Info{ID: id, DisplayName: id, SupportsTools: true, SupportsEmbedding: true})
	}
	return models, nil
}
