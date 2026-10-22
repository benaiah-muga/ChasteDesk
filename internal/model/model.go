// SPDX-License-Identifier: AGPL-3.0-only
package model

import (
	"context"
	"fmt"
	"net/url"
	"strings"
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

type Info struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	SupportsTools     bool   `json:"supportsTools"`
	SupportsEmbedding bool   `json:"supportsEmbedding"`
}

type Config struct {
	BaseURL          string
	APIKey           string
	EmbeddingModelID string
}

type Message struct {
	Role       string
	Content    string
	ToolCallID string
	ToolName   string
	Reasoning  string
}

type ToolDeclaration struct {
	ID          string
	Description string
	Parameters  map[string]any
}

type ToolCall struct {
	ID     string
	ToolID string
	Args   any
}

type ChatRequest struct {
	ModelID         string
	Messages        []Message
	Tools           []ToolDeclaration
	MaxOutputTokens int
	Temperature     *float64
}

type Usage struct {
	InputTokens  int
	OutputTokens int
}

type FinishReason string

const (
	FinishStop      FinishReason = "stop"
	FinishToolCalls FinishReason = "tool_calls"
	FinishLength    FinishReason = "length"
	FinishError     FinishReason = "error"
)

type ChatResult struct {
	Text         *string
	Reasoning    *string
	ToolCalls    []ToolCall
	Usage        Usage
	FinishReason FinishReason
}

type UnsupportedCapabilityError struct {
	Capability string
	ProviderID string
}

func (e *UnsupportedCapabilityError) Error() string {
	return fmt.Sprintf("provider %s does not support %s", e.ProviderID, e.Capability)
}

type APIError struct {
	ProviderID string
	Operation  string
	Status     int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s %s failed for %s: %s", e.ProviderID, e.Operation, e.ProviderID, e.Message)
	}
	return fmt.Sprintf("%s failed for %s: %d", e.Operation, e.ProviderID, e.Status)
}

type Provider interface {
	ID() ProviderID
	ListModels(ctx context.Context, cfg Config) ([]Info, error)
	Chat(ctx context.Context, cfg Config, req ChatRequest) (ChatResult, error)
	Embed(ctx context.Context, cfg Config, texts []string) ([][]float64, error)
}

func For(id ProviderID) Provider {
	switch id {
	case ProviderAnthropic:
		return anthropicProvider{}
	case ProviderGoogle:
		return googleProvider{}
	default:
		return openAIProvider{id: id}
	}
}

func stripSlash(raw string) string {
	return strings.TrimRight(raw, "/")
}

func mapFinishReason(raw string) FinishReason {
	switch raw {
	case "stop", "end_turn", "STOP", "stop_sequence":
		return FinishStop
	case "tool-calls", "tool_calls", "tool_use":
		return FinishToolCalls
	case "length", "max_tokens", "MAX_TOKENS":
		return FinishLength
	default:
		return FinishError
	}
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func numberFrom(raw any) int {
	switch value := raw.(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}
