// SPDX-License-Identifier: AGPL-3.0-only
package model_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chastedesk/internal/crypto"
	"chastedesk/internal/db"
	"chastedesk/internal/logging"
	"chastedesk/internal/model"
	"chastedesk/internal/testutil"
)

const (
	secretsAppSecret    = "secrets-gate-app-secret-material-0123456789"
	secretsPlaintextKey = "sk-gate-plaintext-key-99887766"
)

func TestProviderSecretsResolutionAndRedaction(t *testing.T) {
	dsn := testutil.StartEphemeralPG(t)
	ctx := context.Background()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, `insert into organization (id, name, slug) values ('org-keys', 'Keys Org', 'keys-org')`); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	sealed, err := crypto.SealSecret(secretsPlaintextKey, secretsAppSecret)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := pool.Exec(ctx, `insert into provider_configs
		(org_id, provider_id, display_name, key_cipher, key_iv, key_tag, key_version, key_last4, enabled)
		values ('org-keys', 'openai', 'OpenAI', $1, $2, $3, $4, $5, true)`,
		sealed.Ciphertext, sealed.IV, sealed.AuthTag, sealed.KeyVersion, sealed.Last4); err != nil {
		t.Fatalf("insert provider config: %v", err)
	}

	resolved, err := model.ResolveConfig(ctx, pool, secretsAppSecret, "org-keys", model.ProviderOpenAI)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.APIKey != secretsPlaintextKey {
		t.Fatalf("resolved key must equal the plaintext stored encrypted")
	}

	recorded := []struct {
		url  string
		auth string
	}{}
	recording := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded = append(recorded, struct {
			url  string
			auth string
		}{url: r.URL.String(), auth: r.Header.Get("Authorization")})
		writeMockJSON(w, map[string]any{
			"id": "c", "object": "chat.completion",
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
		})
	}))
	defer recording.Close()

	resolved.BaseURL = recording.URL
	result, err := model.For(model.ProviderOpenAI).Chat(ctx, resolved, model.ChatRequest{
		ModelID: "mock-mini", Messages: []model.Message{{Role: "user", Content: "hi"}}, MaxOutputTokens: 10,
	})
	if err != nil {
		t.Fatalf("chat through resolved config failed: %v", err)
	}
	if result.Text == nil || *result.Text != "ok" {
		t.Fatalf("chat through resolved config failed: %v", result.Text)
	}
	if len(recorded) != 1 || !strings.Contains(recorded[0].auth, secretsPlaintextKey) {
		t.Fatalf("provider did not receive the decrypted key")
	}

	redacted := model.RedactSecrets("user key "+secretsPlaintextKey+" was used for "+recorded[0].url, []string{secretsPlaintextKey})
	if strings.Contains(redacted, secretsPlaintextKey) {
		t.Fatalf("redaction failed to remove the key material")
	}
	guard := model.NewSecretGuard([]string{secretsPlaintextKey})
	if !guard.ContainsSecret("contains " + secretsPlaintextKey) {
		t.Fatalf("secret guard failed to detect the key")
	}
	if guard.ContainsSecret("nothing to see") {
		t.Fatalf("secret guard false positive")
	}

	var buf bytes.Buffer
	logger := slog.New(logging.WithRedactor(slog.NewJSONHandler(&buf, nil), guard))
	logger.Info("provider key "+secretsPlaintextKey+" in use", slog.String("apiKey", secretsPlaintextKey), slog.String("note", "key material must not leak"))
	output := buf.String()
	if strings.Contains(output, secretsPlaintextKey) {
		t.Fatalf("logs must never contain the key material")
	}
	if !strings.Contains(output, "[redacted]") || !strings.Contains(output, "key material must not leak") {
		t.Fatalf("log scrubber did not preserve scrubbed context: %s", output)
	}
}
