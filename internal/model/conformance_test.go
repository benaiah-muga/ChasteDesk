// SPDX-License-Identifier: AGPL-3.0-only
package model_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chastedesk/internal/model"
	"chastedesk/internal/testutil"
)

var textScript = []testutil.MockOpenAIStep{{Text: "Hello from the mock model.", InputTokens: 5, OutputTokens: 3, Finish: "stop"}}

var toolScript = []testutil.MockOpenAIStep{{
	InputTokens: 7, OutputTokens: 4, Finish: "tool_calls",
	ToolCalls: []testutil.MockOpenAIToolCall{{ID: "call_1", Name: "get_weather", Args: map[string]any{"city": "Paris"}}},
}}

var weatherTool = model.ToolDeclaration{
	ID: "get_weather", Description: "Get weather",
	Parameters: map[string]any{
		"type":       "object",
		"properties": map[string]any{"city": map[string]any{"type": "string"}},
		"required":   []any{"city"},
	},
}

func conformance(t *testing.T, provider model.Provider, label string, cfg model.Config, mock *testutil.MockOpenAI) {
	t.Helper()
	ctx := context.Background()

	if mock != nil {
		mock.SetScript(textScript...)
	}
	text, err := provider.Chat(ctx, cfg, model.ChatRequest{
		ModelID: "mock-mini", Messages: []model.Message{{Role: "user", Content: "Say hello"}}, MaxOutputTokens: 100,
	})
	if err != nil {
		t.Fatalf("%s: text chat failed: %v", label, err)
	}
	if text.Text == nil || *text.Text == "" {
		t.Fatalf("%s: text missing", label)
	}
	if text.FinishReason != model.FinishStop {
		t.Fatalf("%s: finishReason must be stop, got %s", label, text.FinishReason)
	}
	if text.Usage.InputTokens <= 0 || text.Usage.OutputTokens <= 0 {
		t.Fatalf("%s: usage not mapped: %+v", label, text.Usage)
	}
	if len(text.ToolCalls) != 0 {
		t.Fatalf("%s: unexpected tool calls on text step", label)
	}

	if mock != nil {
		mock.SetScript(toolScript...)
	}
	tool, err := provider.Chat(ctx, cfg, model.ChatRequest{
		ModelID: "mock-mini", Messages: []model.Message{{Role: "user", Content: "What is the weather?"}},
		Tools: []model.ToolDeclaration{weatherTool}, MaxOutputTokens: 100,
	})
	if err != nil {
		t.Fatalf("%s: tool chat failed: %v", label, err)
	}
	if tool.FinishReason != model.FinishToolCalls {
		t.Fatalf("%s: finishReason must be tool_calls, got %s", label, tool.FinishReason)
	}
	if len(tool.ToolCalls) != 1 {
		t.Fatalf("%s: tool call count wrong: %d", label, len(tool.ToolCalls))
	}
	if tool.ToolCalls[0].ToolID != "get_weather" {
		t.Fatalf("%s: tool name mismatch: %s", label, tool.ToolCalls[0].ToolID)
	}
	args, ok := tool.ToolCalls[0].Args.(map[string]any)
	if !ok || args["city"] != "Paris" {
		t.Fatalf("%s: tool args not parsed to an object: %#v", label, tool.ToolCalls[0].Args)
	}

	abortCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = provider.Chat(abortCtx, cfg, model.ChatRequest{
		ModelID: "mock-mini", Messages: []model.Message{{Role: "user", Content: "hi"}}, MaxOutputTokens: 10,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "abort") {
		t.Fatalf("%s: abort not surfaced: %v", label, err)
	}

	forced := cfg
	forced.APIKey = "FORCE_500"
	_, err = provider.Chat(ctx, forced, model.ChatRequest{
		ModelID: "mock-mini", Messages: []model.Message{{Role: "user", Content: "hi"}}, MaxOutputTokens: 10,
	})
	if err == nil {
		t.Fatalf("%s: provider error not surfaced", label)
	}
}

func startAnthropicMock(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("x-api-key"), "FORCE_500") {
			w.WriteHeader(http.StatusInternalServerError)
			writeMockJSON(w, map[string]any{"error": map[string]any{"message": "mock server error"}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/models") {
			writeMockJSON(w, map[string]any{"data": []map[string]any{{"id": "mock-mini"}}})
			return
		}
		var body struct {
			Tools []any `json:"tools"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Tools) > 0 {
			writeMockJSON(w, map[string]any{
				"id": "msg_mock", "type": "message", "role": "assistant",
				"content":     []map[string]any{{"type": "tool_use", "id": "call_1", "name": "get_weather", "input": map[string]any{"city": "Paris"}}},
				"stop_reason": "tool_use",
				"usage":       map[string]any{"input_tokens": 7, "output_tokens": 4},
			})
			return
		}
		writeMockJSON(w, map[string]any{
			"id": "msg_mock", "type": "message", "role": "assistant",
			"content":     []map[string]any{{"type": "text", "text": "Hello from the mock model."}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 5, "output_tokens": 3},
		})
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func startGoogleMock(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("x-goog-api-key"), "FORCE_500") {
			w.WriteHeader(http.StatusInternalServerError)
			writeMockJSON(w, map[string]any{"error": map[string]any{"message": "mock server error"}})
			return
		}
		if r.URL.Path == "/v1beta/models" {
			writeMockJSON(w, map[string]any{"models": []map[string]any{{"name": "mock-mini"}}})
			return
		}
		var body struct {
			Tools []any `json:"tools"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		parts := []map[string]any{{"text": "Hello from the mock model."}}
		usage := map[string]any{"promptTokenCount": 5, "candidatesTokenCount": 3}
		if len(body.Tools) > 0 {
			parts = []map[string]any{{"functionCall": map[string]any{"name": "get_weather", "args": map[string]any{"city": "Paris"}}}}
			usage = map[string]any{"promptTokenCount": 7, "candidatesTokenCount": 4}
		}
		writeMockJSON(w, map[string]any{
			"candidates":    []map[string]any{{"content": map[string]any{"role": "model", "parts": parts}, "finishReason": "STOP"}},
			"usageMetadata": usage,
		})
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func writeMockJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func TestProviderConformance(t *testing.T) {
	mock := testutil.StartMockOpenAI(t)
	anthropicURL := startAnthropicMock(t)
	googleURL := startGoogleMock(t)

	cases := []struct {
		id     model.ProviderID
		cfg    model.Config
		mock   *testutil.MockOpenAI
		models bool
	}{
		{model.ProviderOpenAI, model.Config{APIKey: "test-key", BaseURL: mock.URL}, mock, true},
		{model.ProviderOllama, model.Config{APIKey: "test-key", BaseURL: mock.URL}, mock, true},
		{model.ProviderOpenAICompatible, model.Config{APIKey: "test-key", BaseURL: mock.URL}, mock, true},
		{model.ProviderAnthropic, model.Config{APIKey: "test-key", BaseURL: anthropicURL}, nil, true},
		{model.ProviderGoogle, model.Config{APIKey: "test-key", BaseURL: googleURL}, nil, true},
	}
	for _, tc := range cases {
		provider := model.For(tc.id)
		conformance(t, provider, string(tc.id), tc.cfg, tc.mock)
		if tc.models {
			models, err := provider.ListModels(context.Background(), tc.cfg)
			if err != nil {
				t.Fatalf("%s: listModels failed: %v", tc.id, err)
			}
			found := false
			for _, info := range models {
				if info.ID == "mock-mini" {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s: listModels missing mock-mini", tc.id)
			}
		}
	}
}
