// SPDX-License-Identifier: AGPL-3.0-only
package logging

import (
	"log/slog"
	"os"
	"strings"
)

type redactor struct {
	secrets []string
}

func (r redactor) Redact(text string) string {
	for _, secret := range r.secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
	}
	return text
}

func New(level string, secrets ...string) *slog.Logger {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	handler := slog.Handler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
	nonEmpty := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if secret != "" {
			nonEmpty = append(nonEmpty, secret)
		}
	}
	if len(nonEmpty) > 0 {
		handler = WithRedactor(handler, redactor{secrets: nonEmpty})
	}
	return slog.New(handler)
}
