// SPDX-License-Identifier: AGPL-3.0-only
package model

import (
	"encoding/json"
	"testing"
)

func TestOpenAIToolMessageKeepsReasoning(t *testing.T) {
	messages := convertOpenAIMessages([]Message{{
		Role: "assistant", Content: `{"query":"refund"}`, Reasoning: "I need to check the refund policy.",
		ToolCallID: "call_1", ToolName: "knowledge.search",
	}})
	encoded, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded) != 1 || decoded[0]["reasoning_content"] != "I need to check the refund policy." {
		t.Fatalf("reasoning content missing: %s", encoded)
	}
	if got := wireToolName("conversations.add_note"); got != "conversations_add_note" {
		t.Fatalf("tool name was not wire safe: %s", got)
	}
}
