// SPDX-License-Identifier: AGPL-3.0-only
package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/db"
	"chastedesk/internal/queue"
	"chastedesk/internal/retrieval"
)

type Deps struct {
	Pool       *pgxpool.Pool
	UploadsDir string
}

func (d Deps) IndexDocument(ctx context.Context, orgID, sourceID string, doc retrieval.ParsedDocument) (int, error) {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var currentVersion int
	err = tx.QueryRow(ctx, `select version from knowledge_sources where id = $1 and org_id = $2 for update`, sourceID, orgID).Scan(&currentVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("source %s not found", sourceID)
	}
	if err != nil {
		return 0, err
	}
	newVersion := currentVersion + 1
	chunks := retrieval.ChunkDocument(doc, sourceID, newVersion)
	if _, err := tx.Exec(ctx, `delete from knowledge_chunks where source_id = $1 and org_id = $2`, sourceID, orgID); err != nil {
		return 0, err
	}
	for _, chunk := range chunks {
		locator, err := json.Marshal(chunk.Locator)
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `insert into knowledge_chunks (id, org_id, source_id, version, chunk_index, content, locator, token_count)
			values ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)`,
			chunk.ID, orgID, sourceID, newVersion, chunk.ChunkIndex, chunk.Content, string(locator), chunk.TokenCount); err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec(ctx, `update knowledge_sources set version = $2, status = 'ready', error = null, updated_at = now() where id = $1 and org_id = $3`,
		sourceID, newVersion, orgID); err != nil {
		return 0, err
	}
	if err := db.WriteAuditQuery(ctx, tx, orgID, db.AuditInput{
		ActorType: db.ActorSystem, Type: "knowledge.ingest.completed",
		TargetType: "knowledge_source", TargetID: &sourceID,
		Payload: map[string]any{"version": newVersion, "chunks": len(chunks)},
	}); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return newVersion, nil
}

func (d Deps) MarkSourceFailed(ctx context.Context, orgID, sourceID, message string) {
	truncated := message
	if len(truncated) > 500 {
		truncated = truncated[:500]
	}
	failedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	tx, err := d.Pool.Begin(failedCtx)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback(failedCtx) }()
	if _, err := tx.Exec(failedCtx, `update knowledge_sources set status = 'failed', error = $3, updated_at = now() where id = $1 and org_id = $2`,
		sourceID, orgID, truncated); err != nil {
		return
	}
	if err := db.WriteAuditQuery(failedCtx, tx, orgID, db.AuditInput{
		ActorType:  db.ActorSystem,
		Type:       "knowledge.ingest.failed",
		TargetType: "knowledge_source",
		TargetID:   &sourceID,
		Payload:    map[string]any{"error": truncated},
	}); err != nil {
		return
	}
	_ = tx.Commit(failedCtx)
}

func (d Deps) IngestURL(ctx context.Context, orgID, sourceID, rawURL string) error {
	crawled, err := Crawl(ctx, rawURL)
	if err != nil {
		d.MarkSourceFailed(ctx, orgID, sourceID, err.Error())
		return err
	}
	title := crawled.Title
	if title == "" {
		title = rawURL
	}
	if _, err := d.IndexDocument(ctx, orgID, sourceID, retrieval.ParsedDocument{Title: title, Markdown: crawled.Markdown}); err != nil {
		d.MarkSourceFailed(ctx, orgID, sourceID, err.Error())
		return err
	}
	return nil
}

func (d Deps) IngestFile(ctx context.Context, orgID, sourceID, filePath, filename string) error {
	doc, err := ParseFile(resolveUploadPath(d.UploadsDir, filePath), filename)
	if err != nil {
		d.MarkSourceFailed(ctx, orgID, sourceID, err.Error())
		return err
	}
	doc.Title = filename
	if _, err := d.IndexDocument(ctx, orgID, sourceID, doc); err != nil {
		d.MarkSourceFailed(ctx, orgID, sourceID, err.Error())
		return err
	}
	return nil
}

func (d Deps) IngestText(ctx context.Context, orgID, sourceID, text, name string) error {
	title := name
	if title == "" {
		title = "Pasted text"
	}
	if _, err := d.IndexDocument(ctx, orgID, sourceID, retrieval.ParsedDocument{Title: title, Markdown: text}); err != nil {
		d.MarkSourceFailed(ctx, orgID, sourceID, err.Error())
		return err
	}
	return nil
}

type ingestURLData struct {
	OrgID    string `json:"orgId"`
	SourceID string `json:"sourceId"`
	URL      string `json:"url"`
}

type ingestFileData struct {
	OrgID    string `json:"orgId"`
	SourceID string `json:"sourceId"`
	FilePath string `json:"filePath"`
	Filename string `json:"filename"`
}

type ingestTextData struct {
	OrgID    string `json:"orgId"`
	SourceID string `json:"sourceId"`
	Text     string `json:"text"`
	Name     string `json:"name"`
}

func Handlers(deps Deps) map[string]queue.Handler {
	return map[string]queue.Handler{
		"ingest-url": func(ctx context.Context, job queue.Job) error {
			var data ingestURLData
			if err := json.Unmarshal(job.Data, &data); err != nil {
				return err
			}
			return deps.IngestURL(ctx, data.OrgID, data.SourceID, data.URL)
		},
		"ingest-file": func(ctx context.Context, job queue.Job) error {
			var data ingestFileData
			if err := json.Unmarshal(job.Data, &data); err != nil {
				return err
			}
			return deps.IngestFile(ctx, data.OrgID, data.SourceID, data.FilePath, data.Filename)
		},
		"ingest-text": func(ctx context.Context, job queue.Job) error {
			var data ingestTextData
			if err := json.Unmarshal(job.Data, &data); err != nil {
				return err
			}
			return deps.IngestText(ctx, data.OrgID, data.SourceID, data.Text, data.Name)
		},
	}
}
