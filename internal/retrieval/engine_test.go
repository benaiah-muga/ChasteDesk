// SPDX-License-Identifier: AGPL-3.0-only
package retrieval

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/db"
	"chastedesk/internal/testutil"
)

func startRetrievalPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testutil.StartEphemeralPG(t)
	pool, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

func TestHybridRetrieval(t *testing.T) {
	pool := startRetrievalPool(t)
	ctx := context.Background()
	orgID := "org-retrieval"
	seedCorpus(t, pool, orgID)

	model := fixtureEmbedModel
	paraID := insertChunk(t, pool, orgID, sourcePara, paraphraseContent, &model)
	insertChunk(t, pool, orgID, sourceLexical, lexicalContent, &model)
	insertChunk(t, pool, orgID, sourceInternal, internalContent, &model)
	for i := 0; i < 6; i++ {
		insertChunk(t, pool, orgID, sourceFillers,
			fmt.Sprintf("Reset your login credential, then review billing plan payment settings. Filler guide %d for ranking depth.", i), &model)
	}
	for i := 0; i < 20; i++ {
		insertChunk(t, pool, orgID, sourceUnrelated,
			fmt.Sprintf("Unrelated doc %d: lorem ipsum dolor amet kale provision region tenant notice channel.", i), &model)
	}

	engine := NewEngine(pool)
	queryEmbedding := embedFixture("reset my password")
	hybrid, err := engine.Search(ctx, orgID, SearchQuery{
		Text:           "qsigphrase",
		Scopes:         []string{"public"},
		QueryEmbedding: queryEmbedding,
		EmbeddingModel: model,
	})
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	if len(hybrid) <= 1 {
		t.Fatalf("hybrid returned too few results: %d", len(hybrid))
	}
	found := false
	for _, e := range hybrid {
		if e.ChunkID == paraID {
			found = true
		}
	}
	if !found {
		t.Fatal("paraphrase chunk missing from hybrid results")
	}
	if hybrid[0].ChunkID != paraID {
		t.Fatalf("hybrid top1 must be the paraphrase chunk, got %s", hybrid[0].ChunkID)
	}
	if hybrid[0].Score <= hybrid[1].Score {
		t.Fatalf("hybrid top1 score must strictly exceed top2: %f vs %f", hybrid[0].Score, hybrid[1].Score)
	}
	for _, e := range hybrid {
		if e.SourceName == "" {
			t.Error("evidence missing sourceName")
		}
		if e.Version != 3 {
			t.Errorf("evidence version mismatch: %d", e.Version)
		}
		if e.Scope != "public" && e.Scope != "internal" {
			t.Errorf("evidence scope invalid: %q", e.Scope)
		}
		if e.Locator.Headings == nil {
			t.Error("evidence missing locator headings")
		}
		if e.Confidence <= 0 || e.Confidence >= 1 {
			t.Errorf("confidence out of range: %f", e.Confidence)
		}
	}
	for _, e := range hybrid {
		if e.Scope == "internal" {
			t.Fatal("internal evidence must not leak with public scope")
		}
	}

	withInternal, err := engine.Search(ctx, orgID, SearchQuery{
		Text:           "secret phrase",
		Scopes:         []string{"public", "internal"},
		QueryEmbedding: queryEmbedding,
		EmbeddingModel: model,
	})
	if err != nil {
		t.Fatalf("internal search: %v", err)
	}
	sawInternal := false
	for _, e := range withInternal {
		if e.Scope == "internal" {
			sawInternal = true
		}
	}
	if !sawInternal {
		t.Fatal("internal evidence missing when internal scope requested")
	}

	ftsOnly, err := engine.Search(ctx, orgID, SearchQuery{Text: "qsigphrase", Scopes: []string{"public"}})
	if err != nil {
		t.Fatalf("fts-only search: %v", err)
	}
	if len(ftsOnly) < 1 {
		t.Fatal("fts-only returned nothing")
	}
	if ftsOnly[0].ChunkID != paraID {
		t.Fatalf("fts-only top1 must be the only lexical match, got %s", ftsOnly[0].ChunkID)
	}
	for _, e := range ftsOnly {
		if e.Score <= 0 {
			t.Error("fts-only rows must all be lexical matches")
		}
	}
	if len(hybrid) <= len(ftsOnly) {
		t.Fatalf("hybrid must surface concept matches beyond lexical matches: %d vs %d", len(hybrid), len(ftsOnly))
	}
}

func TestFTSFallback(t *testing.T) {
	pool := startRetrievalPool(t)
	ctx := context.Background()
	orgID := "org-retrieval"
	seedCorpus(t, pool, orgID)

	for i := 0; i < 12; i++ {
		insertChunk(t, pool, orgID, sourceUnrelated,
			fmt.Sprintf("Billing doc %d: invoice payment plan pricing subscription refund notes and billing policy section %d.", i, i), nil)
	}
	insertChunk(t, pool, orgID, sourcePara,
		"Password reset instructions: to reset your password use the security page and credential options.", nil)

	engine := NewEngine(pool)
	results, err := engine.Search(ctx, orgID, SearchQuery{Text: "reset password", Scopes: []string{"public"}, Limit: 10})
	if err != nil {
		t.Fatalf("fts search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("fts-only search returned nothing")
	}
	if !strings.Contains(results[0].Content, "reset") {
		t.Fatal("fts-only top result should contain the query term")
	}
	for _, e := range results {
		if e.Confidence < 0 || e.Confidence > 1 {
			t.Errorf("confidence out of bounds: %f", e.Confidence)
		}
		if e.SourceName == "" {
			t.Error("evidence missing source name")
		}
	}

	noMatch, err := engine.Search(ctx, orgID, SearchQuery{Text: "zzzunmatchable", Scopes: []string{"public"}})
	if err != nil {
		t.Fatalf("no-match search must not throw: %v", err)
	}
	if noMatch == nil {
		t.Error("no-match search should return an empty slice, not nil")
	}

	withVecRequest, err := engine.Search(ctx, orgID, SearchQuery{
		Text:           "reset password",
		Scopes:         []string{"public"},
		QueryEmbedding: embedFixture("reset password"),
		EmbeddingModel: fixtureEmbedModel,
	})
	if err != nil {
		t.Fatalf("vector request search: %v", err)
	}
	if len(withVecRequest) == 0 {
		t.Fatal("vector request against missing vectors must fall back to lexical results")
	}
}

func TestEmbeddingDims(t *testing.T) {
	pool := startRetrievalPool(t)
	ctx := context.Background()
	orgID := "org-retrieval"
	seedCorpus(t, pool, orgID)

	modelA := "model-a"
	modelB := "model-b"
	for i := 0; i < 4; i++ {
		insertChunk(t, pool, orgID, sourcePara,
			fmt.Sprintf("Password reset guide part %d: use the security settings page and credential options to reset access.", i), &modelA)
	}
	for i := 0; i < 4; i++ {
		insertChunk(t, pool, orgID, sourceModelB,
			fmt.Sprintf("Billing plan invoice notes part %d: payment pricing subscription refund table.", i), &modelB)
	}
	if _, err := pool.Exec(ctx, `update knowledge_sources set embedding_model = 'model-a' where id = $1`, sourcePara); err != nil {
		t.Fatalf("update para model: %v", err)
	}
	if _, err := pool.Exec(ctx, `update knowledge_sources set embedding_model = 'model-b' where id = $1`, sourceModelB); err != nil {
		t.Fatalf("update modelB model: %v", err)
	}

	engine := NewEngine(pool)
	results, err := engine.Search(ctx, orgID, SearchQuery{
		Text:           "reset password credential",
		Scopes:         []string{"public"},
		QueryEmbedding: embedFixture("reset password credential access security"),
		EmbeddingModel: "model-a",
		Limit:          20,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("search returned nothing")
	}
	for _, e := range results {
		if e.SourceID == sourceModelB {
			t.Fatalf("model-b chunks leaked into a model-a search")
		}
	}

	stale, err := engine.SourcesNeedingReindex(ctx, orgID, "model-a")
	if err != nil {
		t.Fatalf("stale sources: %v", err)
	}
	sawModelB := false
	for _, s := range stale {
		if s.SourceID == sourceModelB {
			sawModelB = true
		}
	}
	if !sawModelB {
		t.Fatal("model-b source not flagged for reindex")
	}

	if _, err := pool.Exec(ctx, `update knowledge_sources set embedding_model = 'model-a' where id = $1`, sourceModelB); err != nil {
		t.Fatalf("reindex source: %v", err)
	}
	if _, err := pool.Exec(ctx, `update knowledge_chunks set embedding_model = 'model-a' where source_id = $1`, sourceModelB); err != nil {
		t.Fatalf("reindex chunks: %v", err)
	}
	staleAfter, err := engine.SourcesNeedingReindex(ctx, orgID, "model-a")
	if err != nil {
		t.Fatalf("stale after: %v", err)
	}
	for _, s := range staleAfter {
		if s.SourceID == sourceModelB {
			t.Fatal("reindex flag did not clear after reindexing")
		}
	}
}
