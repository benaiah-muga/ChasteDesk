// SPDX-License-Identifier: AGPL-3.0-only
package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type MockOpenAIRequest struct {
	Path string
	Auth string
	Body map[string]any
}

type MockOpenAIToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

type MockOpenAIStep struct {
	Text         string
	InputTokens  int
	OutputTokens int
	Finish       string
	ToolCalls    []MockOpenAIToolCall
}

type MockOpenAI struct {
	URL    string
	server *httptest.Server

	mu       sync.Mutex
	requests []MockOpenAIRequest
	script   []MockOpenAIStep
}

func StartMockOpenAI(t *testing.T) *MockOpenAI {
	t.Helper()
	mock := &MockOpenAI{
		script: []MockOpenAIStep{{Text: "Hello from the mock model.", InputTokens: 5, OutputTokens: 3, Finish: "stop"}},
	}
	mock.server = httptest.NewServer(http.HandlerFunc(mock.handle))
	mock.URL = mock.server.URL + "/v1"
	t.Cleanup(mock.server.Close)
	return mock
}

func (m *MockOpenAI) SetScript(steps ...MockOpenAIStep) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.script = steps
}

func (m *MockOpenAI) Requests() []MockOpenAIRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MockOpenAIRequest(nil), m.requests...)
}

func (m *MockOpenAI) nextStep() MockOpenAIStep {
	if len(m.script) > 1 {
		step := m.script[0]
		m.script = m.script[1:]
		return step
	}
	return m.script[0]
}

func (m *MockOpenAI) handle(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	auth := r.Header.Get("Authorization")
	var body map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
	entry := MockOpenAIRequest{Path: r.URL.Path, Auth: auth, Body: body}
	m.mu.Lock()
	m.requests = append(m.requests, entry)

	switch {
	case strings.HasSuffix(r.URL.Path, "/models"):
		m.mu.Unlock()
		writeJSON(w, map[string]any{"data": []map[string]any{{"id": "mock-mini"}, {"id": "mock-embed"}}})
		return
	case strings.HasSuffix(r.URL.Path, "/embeddings"):
		inputs := []string{}
		switch value := entry.Body["input"].(type) {
		case []any:
			for _, item := range value {
				if text, ok := item.(string); ok {
					inputs = append(inputs, text)
				}
			}
		case string:
			inputs = append(inputs, value)
		}
		data := make([]map[string]any, 0, len(inputs))
		for i, text := range inputs {
			data = append(data, map[string]any{
				"index":     i,
				"embedding": []float64{0.1 * float64(i+1), 0.2, 0.3, float64(len(text)) / 100},
			})
		}
		m.mu.Unlock()
		writeJSON(w, map[string]any{"data": data})
		return
	case strings.HasSuffix(r.URL.Path, "/chat/completions"):
		if strings.Contains(auth, "FORCE_500") {
			m.mu.Unlock()
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]any{"error": map[string]any{"message": "mock server error"}})
			return
		}
		step := m.nextStep()
		m.mu.Unlock()
		message := map[string]any{"role": "assistant", "content": nil}
		if step.Text != "" {
			message["content"] = step.Text
		}
		if len(step.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(step.ToolCalls))
			for i, call := range step.ToolCalls {
				id := call.ID
				if id == "" {
					id = "call_" + strconv.Itoa(i)
				}
				args, _ := json.Marshal(call.Args)
				calls = append(calls, map[string]any{
					"id": id, "type": "function",
					"function": map[string]any{"name": call.Name, "arguments": string(args)},
				})
			}
			message["tool_calls"] = calls
		}
		input, output := step.InputTokens, step.OutputTokens
		if input == 0 {
			input = 5
		}
		if output == 0 {
			output = 3
		}
		finish := step.Finish
		if finish == "" {
			finish = "stop"
		}
		writeJSON(w, map[string]any{
			"id":     "chatcmpl-mock",
			"object": "chat.completion",
			"choices": []map[string]any{{
				"index": 0, "message": message, "finish_reason": finish,
			}},
			"usage": map[string]any{
				"prompt_tokens": input, "completion_tokens": output,
				"input_tokens": input, "output_tokens": output, "total_tokens": input + output,
			},
		})
		return
	default:
		m.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
