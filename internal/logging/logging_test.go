// SPDX-License-Identifier: AGPL-3.0-only
package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactorScrubsMessagesAndAttributes(t *testing.T) {
	var output bytes.Buffer
	handler := WithRedactor(slog.NewTextHandler(&output, nil), redactor{secrets: []string{"super-secret"}})
	slog.New(handler).Info("provider super-secret failed", "key", "super-secret")
	if strings.Contains(output.String(), "super-secret") {
		t.Fatalf("secret leaked into log: %s", output.String())
	}
	if !strings.Contains(output.String(), "[redacted]") {
		t.Fatalf("redaction marker missing: %s", output.String())
	}
}
