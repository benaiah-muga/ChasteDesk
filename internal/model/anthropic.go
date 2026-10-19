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

type anthropicProvider struct{}

func (p anthropicProvider) ID() ProviderID { return ProviderAnthropic }

func (p anthropicProvider) baseURL(cfg Config) string {
	if cfg.BaseURL != "" {
		return stripSlash(cfg.BaseURL)
	}
	return "https://api.anthropic.com"
}

type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

func convertAnthropicMessages(messages []Message) (string, []anthropicMessage) {
	system := ""
	out := make([]anthropicMessage, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case "system":
			if system != "" {
				system += "\n"
			}
			system += message.Content
		case "assistant":
			if message.ToolCallID != "" && message.ToolName != "" {
				out = append(out, anthropicMessage{Role: "assistant", Content: []anthropicContentBlock{{
					Type: "tool_use", ID: message.ToolCallID, Name: message.ToolName, Input: json.RawMessage(message.Content),
				}}})
				continue
			}
			out = append(out, anthropicMessage{Role: "assistant", Content: []anthropicContentBlock{{Type: "text", Text: message.Content}}})
		case "tool":
			out = append(out, anthropicMessage{Role: "user", Content: []anthropicContentBlock{{
				Type: "tool_result", ToolUseID: message.ToolCallID, Content: message.Content,
			}}})
		default:
			out = append(out, anthropicMessage{Role: "user", Content: []anthropicContentBlock{{Type: "text", Text: message.Content}}})
		}
	}
	return system, out
}

func convertAnthropicTools(tools []ToolDeclaration) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{
			"name":         tool.ID,
			"description":  tool.Description,
			"input_schema": tool.Parameters,
		})
	}
	return out
}

type anthropicResponse struct {
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p anthropicProvider) Chat(ctx context.Context, cfg Config, req ChatRequest) (ChatResult, error) {
	system, messages := convertAnthropicMessages(req.Messages)
	payload := map[string]any{
		"model":      req.ModelID,
		"max_tokens": req.MaxOutputTokens,
		"messages":   messages,
	}
	if system != "" {
		payload["system"] = system
	}
	if tools := convertAnthropicTools(req.Tools); tools != nil {
		payload["tools"] = tools
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ChatResult{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL(cfg)+"/v1/messages", bytes.NewReader(encoded))
	if err != nil {
		return ChatResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-api-key", cfg.APIKey)
	request.Header.Set("anthropic-version", "2023-06-01")
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
		return ChatResult{}, &APIError{ProviderID: string(ProviderAnthropic), Operation: "chat", Status: resp.StatusCode, Message: string(body)}
	}
	var decoded anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return ChatResult{}, err
	}
	result := ChatResult{FinishReason: mapFinishReason(decoded.StopReason)}
	text := ""
	for _, block := range decoded.Content {
		switch block.Type {
		case "text":
			text += block.Text
		case "tool_use":
			var args any
			_ = json.Unmarshal(block.Input, &args)
			result.ToolCalls = append(result.ToolCalls, ToolCall{ID: block.ID, ToolID: block.Name, Args: args})
		}
	}
	if text != "" {
		result.Text = &text
	}
	if len(result.ToolCalls) > 0 {
		result.FinishReason = FinishToolCalls
	}
	result.Usage = Usage{InputTokens: decoded.Usage.InputTokens, OutputTokens: decoded.Usage.OutputTokens}
	return result, nil
}

func (p anthropicProvider) Embed(ctx context.Context, cfg Config, texts []string) ([][]float64, error) {
	return nil, &UnsupportedCapabilityError{Capability: "embeddings", ProviderID: string(ProviderAnthropic)}
}

func (p anthropicProvider) ListModels(ctx context.Context, cfg Config) ([]Info, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL(cfg)+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("x-api-key", cfg.APIKey)
	request.Header.Set("anthropic-version", "2023-06-01")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{ProviderID: string(ProviderAnthropic), Operation: "listModels", Status: resp.StatusCode}
	}
	var envelope modelsEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("listModels failed for %s: invalid response", ProviderAnthropic)
	}
	ids := envelope.ids()
	models := make([]Info, 0, len(ids))
	for _, id := range ids {
		models = append(models, Info{ID: id, DisplayName: id, SupportsTools: true, SupportsEmbedding: false})
	}
	return models, nil
}
