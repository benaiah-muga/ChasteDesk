// SPDX-License-Identifier: AGPL-3.0-only
package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/db"
	"chastedesk/internal/queue"
	"chastedesk/internal/retrieval"
	"chastedesk/internal/testutil"
)

type ingestTestApp struct {
	pool  *pgxpool.Pool
	queue *queue.Queue
}

func startIngestApp(t *testing.T, uploadsDir string) *ingestTestApp {
	t.Helper()
	t.Setenv("CHASTESK_INGEST_ALLOW_PRIVATE", "1")
	t.Setenv("APP_ENV", "test")
	dsn := testutil.StartEphemeralPG(t)
	pool, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	jobs := queue.New(pool)
	for name, handler := range Handlers(Deps{Pool: pool, UploadsDir: uploadsDir}) {
		jobs.Work(ctx, name, 4, handler)
	}
	return &ingestTestApp{pool: pool, queue: jobs}
}

func insertSource(t *testing.T, pool *pgxpool.Pool, orgID, sourceID, sourceType, name, status string, config map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("encode source_config: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `insert into knowledge_sources (id, org_id, type, name, scope, status, version, source_config)
		values ($1, $2, $3, $4, 'public', $5, 1, $6::jsonb)`, sourceID, orgID, sourceType, name, status, string(encoded)); err != nil {
		t.Fatalf("insert source %s: %v", sourceID, err)
	}
}

func waitForStatus(t *testing.T, pool *pgxpool.Pool, sourceID, want string, timeout time.Duration) (string, string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var status string
	var errMessage *string
	for time.Now().Before(deadline) {
		if err := pool.QueryRow(context.Background(), `select status::text, error from knowledge_sources where id = $1`, sourceID).Scan(&status, &errMessage); err != nil {
			t.Fatalf("poll source status: %v", err)
		}
		if status == want {
			break
		}
		if status == "failed" && want != "failed" {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if errMessage == nil {
		return status, ""
	}
	return status, *errMessage
}

func TestIngestURLFixtureSite(t *testing.T) {
	site, err := os.ReadFile("../../scripts/fixtures/ingest-site/index.html")
	if err != nil {
		t.Fatalf("read fixture site: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(site)
	}))
	defer server.Close()

	app := startIngestApp(t, "")
	ctx := context.Background()
	const orgID = "org-ingest-url"
	const sourceID = "00000000-0000-4000-8000-000000000101"
	if _, err := app.pool.Exec(ctx, "insert into organization (id, name, slug) values ($1, 'Ingest Org', 'ingest-org')", orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	insertSource(t, app.pool, orgID, sourceID, "url", "Fixture Site", "pending", map[string]any{"url": server.URL + "/"})

	sent, err := app.queue.Send(ctx, "ingest-url", map[string]any{"orgId": orgID, "sourceId": sourceID, "url": server.URL + "/"}, queue.SendOptions{})
	if err != nil {
		t.Fatalf("send job: %v", err)
	}
	if sent == nil {
		t.Fatal("job send failed")
	}

	status, errMessage := waitForStatus(t, app.pool, sourceID, "ready", 60*time.Second)
	if status != "ready" {
		t.Fatalf("source status ended as %s (%s)", status, errMessage)
	}

	engine := retrieval.NewEngine(app.pool)
	evidence, err := engine.Search(ctx, orgID, retrieval.SearchQuery{Text: "widget security origins plans", Scopes: []string{"public"}, Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(evidence) == 0 {
		t.Fatal("no evidence found after url ingestion")
	}
	sawWidget := false
	for _, e := range evidence {
		if strings.Contains(e.Content, "widget") || strings.Contains(e.Content, "Widget") {
			sawWidget = true
		}
	}
	if !sawWidget {
		t.Fatal("expected widget content in evidence")
	}

	var version int
	if err := app.pool.QueryRow(ctx, "select version from knowledge_sources where id = $1", sourceID).Scan(&version); err != nil {
		t.Fatalf("version query: %v", err)
	}
	if version != 2 {
		t.Fatalf("version must bump to 2, got %d", version)
	}
	var audits int
	if err := app.pool.QueryRow(ctx, "select count(*)::int from audit_events where org_id = $1 and type = 'knowledge.ingest.completed'", orgID).Scan(&audits); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if audits != 1 {
		t.Fatalf("ingest.completed audit row missing (count=%d)", audits)
	}
}
