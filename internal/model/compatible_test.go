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
