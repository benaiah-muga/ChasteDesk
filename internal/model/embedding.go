// SPDX-License-Identifier: AGPL-3.0-only
package model

import (
	"context"
	"fmt"
)

const embeddingBatchSize = 32

type EmbeddingClient struct {
	provider Provider
	cfg      Config
	model    string
}

func NewEmbeddingClient(providerID, baseURL, apiKey, modelID string) (*EmbeddingClient, error) {
	if providerID == "" || modelID == "" {
		return nil, fmt.Errorf("embedding provider and model are required")
	}
	if !KnownProvider(providerID) {
		return nil, fmt.Errorf("unknown embedding provider %q", providerID)
	}
	if providerID == string(ProviderAnthropic) {
		return nil, fmt.Errorf("anthropic does not provide embeddings")
	}
	return &EmbeddingClient{
		provider: For(ProviderID(providerID)),
		cfg:      Config{BaseURL: baseURL, APIKey: apiKey, EmbeddingModelID: modelID},
		model:    modelID,
	}, nil
}

func (c *EmbeddingClient) Model() string { return c.model }

func (c *EmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}
	out := make([][]float64, 0, len(texts))
	for start := 0; start < len(texts); start += embeddingBatchSize {
		end := start + embeddingBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		vectors, err := c.provider.Embed(ctx, c.cfg, texts[start:end])
		if err != nil {
			return nil, fmt.Errorf("embed with %s: %w", c.model, err)
		}
		if len(vectors) != end-start {
			return nil, fmt.Errorf("embed with %s returned %d vectors for %d inputs", c.model, len(vectors), end-start)
		}
		for _, vector := range vectors {
			if len(vector) == 0 {
				return nil, fmt.Errorf("embed with %s returned an empty vector", c.model)
			}
			out = append(out, vector)
		}
	}
	return out, nil
}
