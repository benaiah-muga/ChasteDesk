// SPDX-License-Identifier: AGPL-3.0-only
package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ProviderID string

const (
	ProviderOpenAI           ProviderID = "openai"
	ProviderAnthropic        ProviderID = "anthropic"
	ProviderGoogle           ProviderID = "google"
	ProviderOllama           ProviderID = "ollama"
	ProviderOpenAICompatible ProviderID = "openai-compatible"
)

var KnownProviders = []ProviderID{ProviderOpenAI, ProviderAnthropic, ProviderGoogle, ProviderOllama, ProviderOpenAICompatible}

func KnownProvider(id string) bool {
	for _, known := range KnownProviders {
		if string(known) == id {
			return true
		}
	}
	return false
}

type Info struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	SupportsTools     bool   `json:"supportsTools"`
	SupportsEmbedding bool   `json:"supportsEmbedding"`
}

type Config struct {
	BaseURL string
	APIKey  string
}

func stripSlash(raw string) string {
	return strings.TrimRight(raw, "/")
}

func listModelsURL(id ProviderID, baseURL string) string {
	stripped := stripSlash(baseURL)
	switch id {
	case ProviderOpenAI:
		if stripped == "" {
			stripped = "https://api.openai.com/v1"
		}
		return stripped + "/models"
	case ProviderAnthropic:
		if stripped == "" {
			stripped = "https://api.anthropic.com"
		}
		return stripped + "/v1/models"
	case ProviderGoogle:
		if stripped == "" {
			stripped = "https://generativelanguage.googleapis.com"
		}
		return stripped + "/v1beta/models"
	case ProviderOllama:
		if stripped == "" {
			stripped = "http://localhost:11434"
		}
		if baseURL != "" {
			return stripped + "/models"
		}
		return stripped + "/api/tags"
	default:
		if stripped == "" {
			stripped = "http://localhost:8080/v1"
		}
		return stripped + "/models"
	}
}

func listModelsHeaders(id ProviderID, apiKey string) map[string]string {
	switch id {
	case ProviderOpenAI, ProviderOpenAICompatible:
		if apiKey != "" {
			return map[string]string{"Authorization": "Bearer " + apiKey}
		}
	case ProviderAnthropic:
		return map[string]string{"x-api-key": apiKey, "anthropic-version": "2023-06-01"}
	case ProviderGoogle:
		if apiKey != "" {
			return map[string]string{"x-goog-api-key": apiKey}
		}
	}
	return map[string]string{}
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

func ListModels(ctx context.Context, id ProviderID, cfg Config) ([]Info, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, listModelsURL(id, cfg.BaseURL), nil)
	if err != nil {
		return nil, err
	}
	for key, value := range listModelsHeaders(id, cfg.APIKey) {
		request.Header.Set(key, value)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("listModels failed for %s: %d", id, resp.StatusCode)
	}
	var envelope modelsEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("listModels failed for %s: invalid response", id)
	}
	ids := envelope.ids()
	models := make([]Info, 0, len(ids))
	for _, modelID := range ids {
		models = append(models, Info{
			ID:                modelID,
			DisplayName:       modelID,
			SupportsTools:     true,
			SupportsEmbedding: id != ProviderAnthropic,
		})
	}
	return models, nil
}

func ValidBaseURL(raw string) bool {
	if raw == "" {
		return true
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}
