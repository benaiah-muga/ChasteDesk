// SPDX-License-Identifier: AGPL-3.0-only
package ingest

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"chastedesk/internal/queue"
)

func copyFixture(t *testing.T, dir, name string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("../../scripts/fixtures/files", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

func TestIngestFilesFormats(t *testing.T) {
	uploads := t.TempDir()
	for _, name := range []string{"notes.pdf", "notes.md", "notes.docx", "notes.txt"} {
		copyFixture(t, uploads, name)
	}
	if err := os.WriteFile(filepath.Join(uploads, "evil.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatalf("write evil.exe: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uploads, "fake.pdf"), []byte("this is not a pdf"), 0o644); err != nil {
		t.Fatalf("write fake.pdf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uploads, "huge.txt"), bytes.Repeat([]byte("x"), 11*1024*1024), 0o644); err != nil {
		t.Fatalf("write huge.txt: %v", err)
	}

	app := startIngestApp(t, uploads)
	ctx := context.Background()
	const orgID = "org-ingest-files"
	if _, err := app.pool.Exec(ctx, "insert into organization (id, name, slug) values ($1, 'Files Org', 'files-org')", orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}

	files := []struct{ id, filename, needle string }{
		{"00000000-0000-4000-8000-000000000201", "notes.pdf", "ChasteDesk password reset guide"},
		{"00000000-0000-4000-8000-000000000202", "notes.md", "Password resets and billing plans"},
		{"00000000-0000-4000-8000-000000000203", "notes.docx", "password reset steps and billing plan"},
		{"00000000-0000-4000-8000-000000000204", "notes.txt", "password resets, billing plans"},
	}
	for _, file := range files {
		insertSource(t, app.pool, orgID, file.id, "file", file.filename, "pending", map[string]any{
			"path": filepath.Join(uploads, file.filename), "filename": file.filename,
		})
		if _, err := app.queue.Send(ctx, "ingest-file", map[string]any{
			"orgId": orgID, "sourceId": file.id, "filePath": filepath.Join(uploads, file.filename), "filename": file.filename,
		}, queue.SendOptions{}); err != nil {
			t.Fatalf("send ingest-file %s: %v", file.filename, err)
		}
	}
	for _, file := range files {
		status, errMessage := waitForStatus(t, app.pool, file.id, "ready", 60*time.Second)
		if status != "ready" {
			t.Fatalf("ingestion of %s did not reach ready: %s (%s)", file.id, status, errMessage)
		}
		var contents []string
		rows, err := app.pool.Query(ctx, "select content from knowledge_chunks where source_id = $1", file.id)
		if err != nil {
			t.Fatalf("chunks query: %v", err)
		}
		for rows.Next() {
			var content string
			if err := rows.Scan(&content); err != nil {
				t.Fatalf("scan chunk: %v", err)
			}
			contents = append(contents, content)
		}
		rows.Close()
		all := strings.Join(contents, "\n")
		if !strings.Contains(all, file.needle) {
			t.Fatalf("parsed content missing needle for %s: %s", file.id, all[:min(200, len(all))])
		}
	}

	bad := []struct{ id, filename string }{
		{"00000000-0000-4000-8000-000000000301", "evil.exe"},
		{"00000000-0000-4000-8000-000000000302", "fake.pdf"},
		{"00000000-0000-4000-8000-000000000303", "huge.txt"},
	}
	for _, file := range bad {
		insertSource(t, app.pool, orgID, file.id, "file", file.filename, "pending", map[string]any{
			"path": filepath.Join(uploads, file.filename), "filename": file.filename,
		})
		if _, err := app.queue.Send(ctx, "ingest-file", map[string]any{
			"orgId": orgID, "sourceId": file.id, "filePath": filepath.Join(uploads, file.filename), "filename": file.filename,
		}, queue.SendOptions{}); err != nil {
			t.Fatalf("send reject case %s: %v", file.filename, err)
		}
	}
	for _, file := range bad {
		status, errMessage := waitForStatus(t, app.pool, file.id, "failed", 60*time.Second)
		if status != "failed" {
			t.Fatalf("reject case %s did not fail: %s", file.id, status)
		}
		if errMessage == "" {
			t.Fatalf("reject case %s missing error message", file.id)
		}
	}

	var audits int
	if err := app.pool.QueryRow(ctx, "select count(*)::int from audit_events where org_id = $1 and type in ('knowledge.ingest.completed','knowledge.ingest.failed')", orgID).Scan(&audits); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if audits < 7 {
		t.Fatalf("expected audit rows for every ingest outcome, got %d", audits)
	}
}
