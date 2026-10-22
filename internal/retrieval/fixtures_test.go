// SPDX-License-Identifier: AGPL-3.0-only
package retrieval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const fixtureEmbedModel = "test-embed-v1"

const (
	sourcePara      = "00000000-0000-4000-8000-000000000001"
	sourceFillers   = "00000000-0000-4000-8000-000000000002"
	sourceLexical   = "00000000-0000-4000-8000-000000000003"
	sourceUnrelated = "00000000-0000-4000-8000-000000000004"
	sourceInternal  = "00000000-0000-4000-8000-000000000005"
	sourceModelB    = "00000000-0000-4000-8000-000000000006"
)

var fixtureConcepts = []string{"access", "billing", "api", "setup"}

var fixtureLexicon = map[string][]string{
	"access":  {"password", "credential", "login", "reset", "secret", "phrase", "sign in", "signin", "security"},
	"billing": {"invoice", "refund", "billing", "plan", "payment", "pricing", "subscription"},
	"api":     {"api", "endpoint", "request", "webhook", "token", "payload"},
	"setup":   {"install", "setup", "getting started", "onboarding", "deploy", "configure"},
}

func embedFixture(text string) []float64 {
	lowered := strings.ToLower(text)
	counts := map[string]int{}
	total := 0
	for concept, words := range fixtureLexicon {
		for _, word := range words {
			counts[concept] += strings.Count(lowered, word)
		}
	}
	for _, concept := range fixtureConcepts {
		total += counts[concept]
	}
	if total == 0 {
		total = 1
	}
	vec := make([]float64, len(fixtureConcepts))
	for i, concept := range fixtureConcepts {
		vec[i] = float64(counts[concept]) / float64(total)
	}
	return vec
}

func fillSentence(concept string, seed int64, length int) string {
	fillerWords := []string{"lorem", "ipsum", "dolor", "amet", "kale", "provision", " Account", "region", "tenant", "notice"}
	words := make([]string, 0, length)
	state := seed
	for i := 0; i < length; i++ {
		state = (state*1103515245 + 12345) % 2147483648
		words = append(words, fillerWords[state%int64(len(fillerWords))])
	}
	return fmt.Sprintf("%s note: %s.", concept, strings.Join(words, " "))
}

func buildFixtureDoc() string {
	section := func(title string, seed int64) string {
		lines := make([]string, 0, 7)
		for i := 0; i < 7; i++ {
			lines = append(lines, fillSentence(title+" detail", seed+int64(i), 55))
		}
		return strings.Join(lines, "\n\n")
	}
	parts := []string{
		"# Guide",
		"This guide explains the workspace basics for administrators and members.",
		"## Section A",
		section("alpha", 7),
		"## Section B",
		section("beta", 91),
		"## Plans",
		"| Plan | Feature | Limit | Price |",
		"| --- | --- | --- | --- |",
	}
	for i := 0; i < 25; i++ {
		parts = append(parts, fmt.Sprintf("| Plan %d | feature %d | limit %d | price %d9 |", i+1, i+1, i*10+5, i+1))
	}
	parts = append(parts, "## Section C", section("gamma", 233))
	return strings.Join(parts, "\n")
}

func seedCorpus(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "insert into organization (id, name, slug) values ($1, 'Retrieval Org', 'retrieval-org')", orgID); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	sources := []struct{ id, name, scope string }{
		{sourcePara, "Password Help", "public"},
		{sourceFillers, "Login Guides", "public"},
		{sourceLexical, "API Reference", "public"},
		{sourceUnrelated, "Unrelated Docs", "public"},
		{sourceInternal, "Internal Runbook", "internal"},
		{sourceModelB, "Model B Notes", "public"},
	}
	for _, s := range sources {
		if _, err := pool.Exec(ctx, `insert into knowledge_sources (id, org_id, type, name, scope, status, version)
			values ($1, $2, 'text', $3, $4, 'ready', 3)`, s.id, orgID, s.name, s.scope); err != nil {
			t.Fatalf("insert source %s: %v", s.name, err)
		}
	}
}

func insertChunk(t *testing.T, pool *pgxpool.Pool, orgID, sourceID, content string, embeddingModel *string) string {
	t.Helper()
	ctx := context.Background()
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("rand: %v", err)
	}
	id := hex.EncodeToString(raw)
	vec := embedFixture(content)
	locator, err := json.Marshal(map[string]any{"headings": []string{"Doc"}, "charRange": []int{0, len(content)}})
	if err != nil {
		t.Fatalf("marshal locator: %v", err)
	}
	var literal *string
	if embeddingModel != nil {
		encoded := VectorLiteral(vec)
		literal = &encoded
	}
	if _, err := pool.Exec(ctx, `insert into knowledge_chunks (id, org_id, source_id, version, chunk_index, content, locator, token_count, embedding_model, embedding)
		values ($1, $2, $3, 3, 0, $4, $5::jsonb, $6, $7, $8::vector)`,
		id, orgID, sourceID, content, string(locator), EstimateTokens(content), embeddingModel, literal); err != nil {
		t.Fatalf("insert chunk: %v", err)
	}
	return id
}

const paraphraseContent = "qsigphrase credential. Forgot your secret phrase? Open the security settings page and choose a new credential. You can sign in afterwards with the new secret phrase."

const lexicalContent = "The password field of the API request body must be at least 8 characters. The API rejects password values shorter than 8 characters. Password handling in the API request validation rules."

const internalContent = "Internal runbook: when a customer forgets their secret phrase, verify identity, then reset the credential from the admin security console."
