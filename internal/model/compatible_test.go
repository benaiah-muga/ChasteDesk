// SPDX-License-Identifier: AGPL-3.0-only
package model_test

import (
	"context"
	"testing"

	"chastedesk/internal/model"
	"chastedesk/internal/testutil"
)

func TestCompatibleProviders(t *testing.T) {
	mock := testutil.StartMockOpenAI(t)
	mock.SetScript(testutil.MockOpenAIStep{Text: "Compatible provider reply.", InputTokens: 4, OutputTokens: 6, Finish: "stop"})
	ctx := context.Background()

	cases := []struct {
		id  model.ProviderID
		cfg model.Config
	}{
		{model.ProviderOllama, model.Config{BaseURL: mock.URL}},
		{model.ProviderOpenAICompatible, model.Config{APIKey: "test-key", BaseURL: mock.URL}},
	}
	for _, tc := range cases {
		provider := model.For(tc.id)
		result, err := provider.Chat(ctx, tc.cfg, model.ChatRequest{
			ModelID: "mock-mini", Messages: []model.Message{{Role: "user", Content: "hi"}}, MaxOutputTokens: 50,
		})
		if err != nil {
			t.Fatalf("%s: chat failed: %v", tc.id, err)
		}
		if result.Text == nil || *result.Text != "Compatible provider reply." {
			t.Fatalf("%s: text mismatch: %v", tc.id, result.Text)
		}
		if result.Usage.OutputTokens != 6 {
			t.Fatalf("%s: usage mismatch: %+v", tc.id, result.Usage)
		}
		models, err := provider.ListModels(ctx, tc.cfg)
		if err != nil {
			t.Fatalf("%s: listModels failed: %v", tc.id, err)
		}
		if len(models) != 2 {
			t.Fatalf("%s: model list length wrong: %d", tc.id, len(models))
		}
	}

	recorded := 0
	for _, request := range mock.Requests() {
		if request.Body != nil && request.Body["model"] == "mock-mini" {
			recorded++
		}
	}
	if recorded < 2 {
		t.Fatalf("mock server did not see both provider requests: %d", recorded)
	}
}

func TestCompatibleProviderSanitizesToolNames(t *testing.T) {
	mock := testutil.StartMockOpenAI(t)
	mock.SetScript(testutil.MockOpenAIStep{
		Text: "Looking that up.", InputTokens: 4, OutputTokens: 6, Finish: "tool_calls",
		ToolCalls: []testutil.MockOpenAIToolCall{{ID: "call_1", Name: "knowledge_search", Args: map[string]any{"query": "refund"}}},
	})
	provider := model.For(model.ProviderOpenAICompatible)
	result, err := provider.Chat(context.Background(), model.Config{APIKey: "test-key", BaseURL: mock.URL}, model.ChatRequest{
		ModelID:         "deepseek-flash",
		Messages:        []model.Message{{Role: "user", Content: "What is the refund policy?"}},
		Tools:           []model.ToolDeclaration{{ID: "knowledge.search", Description: "Search the knowledge base.", Parameters: map[string]any{"type": "object"}}},
		MaxOutputTokens: 100,
	})
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].ToolID != "knowledge.search" {
		t.Fatalf("tool call name was not mapped back: %+v", result.ToolCalls)
	}
	requests := mock.Requests()
	if len(requests) != 1 {
		t.Fatalf("expected one request, got %d", len(requests))
	}
	tools, ok := requests[0].Body["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools missing from request: %+v", requests[0].Body)
	}
	function, _ := tools[0].(map[string]any)["function"].(map[string]any)
	if function["name"] != "knowledge_search" {
		t.Fatalf("wire tool name was not sanitized: %+v", function)
	}
}
