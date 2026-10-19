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

type googleProvider struct{}

func (p googleProvider) ID() ProviderID { return ProviderGoogle }

func (p googleProvider) baseURL(cfg Config) string {
	if cfg.BaseURL != "" {
		return stripSlash(cfg.BaseURL)
	}
	return "https://generativelanguage.googleapis.com"
}

type googlePart struct {
	Text             string              `json:"text,omitempty"`
	FunctionCall     *googleFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *googleFunctionResp `json:"functionResponse,omitempty"`
}

type googleFunctionCall struct {
	Name string `json:"name"`
	Args any    `json:"args"`
}

type googleFunctionResp struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type googleContent struct {
	Role  string       `json:"role"`
	Parts []googlePart `json:"parts"`
}

func convertGoogleMessages(messages []Message) []googleContent {
	out := make([]googleContent, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case "system":
			continue
		case "assistant":
			if message.ToolCallID != "" && message.ToolName != "" {
				var args any
				_ = json.Unmarshal([]byte(message.Content), &args)
				out = append(out, googleContent{Role: "model", Parts: []googlePart{{FunctionCall: &googleFunctionCall{Name: message.ToolName, Args: args}}}})
				continue
			}
			out = append(out, googleContent{Role: "model", Parts: []googlePart{{Text: message.Content}}})
		case "tool":
			out = append(out, googleContent{Role: "user", Parts: []googlePart{{FunctionResponse: &googleFunctionResp{
				Name: message.ToolName, Response: map[string]any{"result": message.Content},
			}}}})
		default:
			out = append(out, googleContent{Role: "user", Parts: []googlePart{{Text: message.Content}}})
		}
	}
	return out
}

func convertGoogleTools(tools []ToolDeclaration) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	declarations := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		declarations = append(declarations, map[string]any{
			"name":        tool.ID,
			"description": tool.Description,
			"parameters":  tool.Parameters,
		})
	}
	return []map[string]any{{"functionDeclarations": declarations}}
}

type googleResponse struct {
	Candidates []struct {
		Content struct {
			Role  string       `json:"role"`
			Parts []googlePart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

func (p googleProvider) Chat(ctx context.Context, cfg Config, req ChatRequest) (ChatResult, error) {
	payload := map[string]any{
		"contents":         convertGoogleMessages(req.Messages),
		"generationConfig": map[string]any{"maxOutputTokens": req.MaxOutputTokens},
	}
	if tools := convertGoogleTools(req.Tools); tools != nil {
		payload["tools"] = tools
	}
	if req.Temperature != nil {
		payload["generationConfig"].(map[string]any)["temperature"] = *req.Temperature
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ChatResult{}, err
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", p.baseURL(cfg), req.ModelID)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return ChatResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-goog-api-key", cfg.APIKey)
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
		return ChatResult{}, &APIError{ProviderID: string(ProviderGoogle), Operation: "chat", Status: resp.StatusCode, Message: string(body)}
	}
	var decoded googleResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return ChatResult{}, err
	}
	result := ChatResult{FinishReason: FinishError}
	if len(decoded.Candidates) > 0 {
		candidate := decoded.Candidates[0]
		text := ""
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				text += part.Text
			}
			if part.FunctionCall != nil {
				result.ToolCalls = append(result.ToolCalls, ToolCall{
					ID: fmt.Sprintf("call_%d", len(result.ToolCalls)), ToolID: part.FunctionCall.Name, Args: part.FunctionCall.Args,
				})
			}
		}
		if text != "" {
			result.Text = &text
		}
		result.FinishReason = mapFinishReason(candidate.FinishReason)
		if len(result.ToolCalls) > 0 {
			result.FinishReason = FinishToolCalls
		}
	}
	result.Usage = Usage{
		InputTokens:  decoded.UsageMetadata.PromptTokenCount,
		OutputTokens: decoded.UsageMetadata.CandidatesTokenCount,
	}
	return result, nil
}

type googleEmbeddingResponse struct {
	Embeddings []struct {
		Values []float64 `json:"values"`
	} `json:"embeddings"`
}

func (p googleProvider) Embed(ctx context.Context, cfg Config, texts []string) ([][]float64, error) {
	embeddingModel := cfg.EmbeddingModelID
	if embeddingModel == "" {
		embeddingModel = defaultEmbeddingModel
	}
	requests := make([]map[string]any, 0, len(texts))
	for _, text := range texts {
		requests = append(requests, map[string]any{
			"model":   "models/" + embeddingModel,
			"content": map[string]any{"parts": []map[string]any{{"text": text}}},
		})
	}
	encoded, err := json.Marshal(map[string]any{"requests": requests})
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:batchEmbedContents", p.baseURL(cfg), embeddingModel)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-goog-api-key", cfg.APIKey)
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
		return nil, &APIError{ProviderID: string(ProviderGoogle), Operation: "embed", Status: resp.StatusCode, Message: string(body)}
	}
	var decoded googleEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	vectors := make([][]float64, 0, len(decoded.Embeddings))
	for _, entry := range decoded.Embeddings {
		vectors = append(vectors, entry.Values)
	}
	return vectors, nil
}

func (p googleProvider) ListModels(ctx context.Context, cfg Config) ([]Info, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL(cfg)+"/v1beta/models", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("x-goog-api-key", cfg.APIKey)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{ProviderID: string(ProviderGoogle), Operation: "listModels", Status: resp.StatusCode}
	}
	var envelope modelsEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("listModels failed for %s: invalid response", ProviderGoogle)
	}
	ids := envelope.ids()
	models := make([]Info, 0, len(ids))
	for _, id := range ids {
		models = append(models, Info{ID: id, DisplayName: id, SupportsTools: true, SupportsEmbedding: true})
	}
	return models, nil
}
