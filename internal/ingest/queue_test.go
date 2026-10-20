// SPDX-License-Identifier: AGPL-3.0-only
package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"chastedesk/internal/queue"
)

func TestQueueLifecycleRetriesFailuresAndDedupesSingletons(t *testing.T) {
	var hits atomic.Int64
	var inflight atomic.Int64
	var maxInflight atomic.Int64
	var delayMs atomic.Int64
	delayMs.Store(700)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := inflight.Add(1)
		for {
			previous := maxInflight.Load()
			if current <= previous || maxInflight.CompareAndSwap(previous, current) {
				break
			}
		}
		time.Sleep(time.Duration(delayMs.Load()) * time.Millisecond)
		inflight.Add(-1)
		hits.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><head><title>Lifecycle</title></head><body><p>queued ingestion payload</p></body></html>"))
	}))
	defer server.Close()

	app := startIngestApp(t, "")
	ctx := context.Background()
	const orgID = "org-ingest-queue"
	if _, err := app.pool.Exec(ctx, "insert into organization (id, name, slug) values ($1, 'Queue Org', 'queue-org')", orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}

	const failSourceID = "00000000-0000-4000-8000-000000000401"
	insertSource(t, app.pool, orgID, failSourceID, "url", "Unreachable source", "pending", map[string]any{"url": "http://127.0.0.1:1/unreachable"})
	if _, err := app.queue.Send(ctx, "ingest-url", map[string]any{
		"orgId": orgID, "sourceId": failSourceID, "url": "http://127.0.0.1:1/unreachable",
	}, queue.SendOptions{RetryLimit: 2, RetryDelaySeconds: 1}); err != nil {
		t.Fatalf("send failing job: %v", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	var failAudits int
	for time.Now().Before(deadline) {
		if err := app.pool.QueryRow(ctx, `select count(*)::int from audit_events where org_id = $1 and type = 'knowledge.ingest.failed' and target_id = $2`,
			orgID, failSourceID).Scan(&failAudits); err != nil {
			t.Fatalf("failed audit count: %v", err)
		}
		if failAudits >= 2 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if failAudits < 2 {
		t.Fatalf("expected >= 2 knowledge.ingest.failed audits after retries, got %d", failAudits)
	}
	status, _ := waitForStatus(t, app.pool, failSourceID, "failed", 10*time.Second)
	if status != "failed" {
		t.Fatalf("source status after exhausted retries = %s, want failed", status)
	}

	const okSourceID = "00000000-0000-4000-8000-000000000402"
	insertSource(t, app.pool, orgID, okSourceID, "url", "Slow source", "pending", map[string]any{"url": server.URL + "/"})
	const singletonKey = "ingest-queue-lifecycle"
	first, err := app.queue.Send(ctx, "ingest-url", map[string]any{
		"orgId": orgID, "sourceId": okSourceID, "url": server.URL + "/",
	}, queue.SendOptions{SingletonKey: singletonKey})
	if err != nil {
		t.Fatalf("first ok send: %v", err)
	}
	if first == nil {
		t.Fatal("first ok send was deduped unexpectedly")
	}
	duplicate, err := app.queue.Send(ctx, "ingest-url", map[string]any{
		"orgId": orgID, "sourceId": okSourceID, "url": server.URL + "/",
	}, queue.SendOptions{SingletonKey: singletonKey})
	if err != nil {
		t.Fatalf("duplicate send: %v", err)
	}
	if duplicate != nil {
		t.Fatal("duplicate singleton job must be deduped")
	}

	status, _ = waitForStatus(t, app.pool, okSourceID, "ready", 20*time.Second)
	if status != "ready" {
		t.Fatalf("slow source did not become ready: %s", status)
	}
	deadline = time.Now().Add(15 * time.Second)
	completed := false
	for time.Now().Before(deadline) {
		var value string
		err := app.pool.QueryRow(ctx, `select status from jobs where name = 'ingest-url' and singleton_key = $1 order by created_at desc limit 1`, singletonKey).Scan(&value)
		if err == nil && value == "completed" {
			completed = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !completed {
		t.Fatal("ingest job did not reach completed before the next send")
	}

	delayMs.Store(0)
	third, err := app.queue.Send(ctx, "ingest-url", map[string]any{
		"orgId": orgID, "sourceId": okSourceID, "url": server.URL + "/",
	}, queue.SendOptions{SingletonKey: singletonKey})
	if err != nil {
		t.Fatalf("post-completion send: %v", err)
	}
	if third == nil {
		t.Fatal("job after completion must be accepted, not deduped")
	}

	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && hits.Load() < 2 {
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	if hits.Load() < 2 {
		t.Fatalf("expected the source to be crawled at least twice, hits=%d", hits.Load())
	}
	if maxInflight.Load() > 1 {
		t.Fatalf("singleton job ran concurrently: max inflight=%d", maxInflight.Load())
	}
}
