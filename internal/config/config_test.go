// SPDX-License-Identifier: AGPL-3.0-only
package config

import (
	"strings"
	"testing"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"PORT", "APP_URL", "LOG_LEVEL", "DATABASE_URL", "APP_SECRET", "SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_TLS", "EMAIL_INBOUND_ADDRESS", "EMAIL_INBOUND_SECRET"} {
		t.Setenv(key, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgresql://user:pass@localhost:5432/db")
	t.Setenv("APP_SECRET", strings.Repeat("s", 32))
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != 3000 {
		t.Errorf("Port = %d, want 3000", cfg.Port)
	}
	if cfg.AppURL != "http://localhost:3000" {
		t.Errorf("AppURL = %q", cfg.AppURL)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
}

func TestLoadCustom(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgresql://user:pass@localhost:5432/db")
	t.Setenv("APP_SECRET", strings.Repeat("s", 32))
	t.Setenv("PORT", "8080")
	t.Setenv("APP_URL", "https://desk.example.com/")
	t.Setenv("LOG_LEVEL", "debug")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.AppURL != "https://desk.example.com" {
		t.Errorf("AppURL = %q, want trailing slash trimmed", cfg.AppURL)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	base := map[string]string{
		"DATABASE_URL": "postgresql://user:pass@localhost:5432/db",
		"APP_SECRET":   strings.Repeat("s", 32),
	}
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"port not a number", map[string]string{"PORT": "abc"}},
		{"port out of range", map[string]string{"PORT": "70000"}},
		{"unknown log level", map[string]string{"LOG_LEVEL": "verbose"}},
		{"missing database url", map[string]string{"DATABASE_URL": ""}},
		{"short app secret", map[string]string{"APP_SECRET": "too-short"}},
		{"smtp without from", map[string]string{"SMTP_HOST": "smtp.example.com"}},
		{"smtp invalid from", map[string]string{"SMTP_HOST": "smtp.example.com", "SMTP_FROM": "not-an-email"}},
		{"smtp invalid tls", map[string]string{"SMTP_HOST": "smtp.example.com", "SMTP_FROM": "support@example.com", "SMTP_TLS": "ssl"}},
		{"short inbound secret", map[string]string{"EMAIL_INBOUND_SECRET": "too-short"}},
		{"inbound secret without address", map[string]string{"EMAIL_INBOUND_SECRET": "a-valid-inbound-secret"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			for k, v := range base {
				t.Setenv(k, v)
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Fatalf("Load() succeeded, want error for %s", tc.name)
			}
		})
	}
}
