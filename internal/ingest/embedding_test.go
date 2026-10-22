// SPDX-License-Identifier: AGPL-3.0-only
package ingest

import (
	"context"
	"testing"

	"chastedesk/internal/db"
	"chastedesk/internal/retrieval"
	"chastedesk/internal/testutil"
)

type fixedEmbedder struct{}

func (fixedEmbedder) Model() string { return "fixed-embed" }

func (fixedEmbedder) Embed(_ context.Context, texts []string) ([][]float64, error) {
	vectors := make([][]float64, 0, len(texts))
	for range texts {
		vectors = append(vectors, []float64{0.1, 0.2, 0.3})
	}
	return vectors, nil
}

func TestIndexDocumentEmbedsChunks(t *testing.T) {
	dsn := testutil.StartEphemeralPG(t)
	pool, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	const orgID = "org-embed"
	if _, err := pool.Exec(ctx, "insert into organization (id, name, slug) values ($1, 'Embed Org', 'embed-org')", orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	const sourceID = "00000000-0000-4000-8000-000000000901"
	insertSource(t, pool, orgID, sourceID, "text", "Embedding policy", "pending", map[string]any{})
	deps := Deps{Pool: pool, Embedder: fixedEmbedder{}}
	if _, err := deps.IndexDocument(ctx, orgID, sourceID, retrieval.ParsedDocument{
		Title:    "Embedding policy",
		Markdown: "Unused seats may be credited within 14 days. The original payment method receives the credit.",
	}); err != nil {
		t.Fatalf("index document: %v", err)
	}
	var chunks, embedded, dimensions int
	var model string
	if err := pool.QueryRow(ctx, `select count(*)::int, count(embedding)::int, coalesce(min(vector_dims(embedding)), 0)::int, coalesce(min(embedding_model), '')
		from knowledge_chunks where source_id = $1 and org_id = $2`, sourceID, orgID).Scan(&chunks, &embedded, &dimensions, &model); err != nil {
		t.Fatalf("read embeddings: %v", err)
	}
	if chunks == 0 || embedded != chunks || dimensions != 3 || model != "fixed-embed" {
		t.Fatalf("embedding state mismatch: chunks=%d embedded=%d dimensions=%d model=%s", chunks, embedded, dimensions, model)
	}
	var sourceModel string
	if err := pool.QueryRow(ctx, `select coalesce(embedding_model, '') from knowledge_sources where id = $1`, sourceID).Scan(&sourceModel); err != nil {
		t.Fatalf("read source embedding model: %v", err)
	}
	if sourceModel != "fixed-embed" {
		t.Fatalf("source embedding model mismatch: %s", sourceModel)
	}
}
