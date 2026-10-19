// SPDX-License-Identifier: AGPL-3.0-only
package model_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"chastedesk/internal/model"
	"chastedesk/internal/testutil"
)

func TestEmbeddingBatchAndCapabilityErrors(t *testing.T) {
	mock := testutil.StartMockOpenAI(t)
	cfg := model.Config{APIKey: "test-key", BaseURL: mock.URL, EmbeddingModelID: "mock-embed"}
	vectors, err := model.For(model.ProviderOpenAI).Embed(context.Background(), cfg, []string{"alpha", "beta"})
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	if len(vectors) != 2 {
		t.Fatalf("embedding batch size mismatch: %d", len(vectors))
	}
	for _, vector := range vectors {
		if len(vector) != 4 {
			t.Fatalf("embedding vector shape wrong: %d", len(vector))
		}
		for _, value := range vector {
			if value != value {
				t.Fatal("embedding vector contains NaN")
			}
		}
	}

	_, err = model.For(model.ProviderAnthropic).Embed(context.Background(), model.Config{APIKey: "k"}, []string{"text"})
	var unsupported *model.UnsupportedCapabilityError
	if !errors.As(err, &unsupported) {
		t.Fatalf("anthropic embed must return UnsupportedCapabilityError, got %v", err)
	}

	embeddingRequests := 0
	for _, request := range mock.Requests() {
		if strings.HasSuffix(request.Path, "/embeddings") {
			embeddingRequests++
		}
	}
	if embeddingRequests != 1 {
		t.Fatalf("expected exactly one embedding request, got %d", embeddingRequests)
	}
}
