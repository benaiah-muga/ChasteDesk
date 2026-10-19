// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"chastedesk/internal/apperr"
	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/queue"
)

func (s *Server) handleKnowledgeSourcesList(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapKnowledgeRead)
	if !ok {
		return
	}
	rows, err := s.repo.ListKnowledgeSources(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, rows)
}

type knowledgeCreateBody struct {
	Kind  string `json:"kind"`
	URL   string `json:"url"`
	Name  string `json:"name"`
	Text  string `json:"text"`
	Scope string `json:"scope"`
}

func normalizeScope(raw string) string {
	if raw == "internal" {
		return "internal"
	}
	return "public"
}

func (s *Server) handleKnowledgeSourceCreate(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapKnowledgeWrite)
	if !ok {
		return
	}
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		s.createFileSource(w, r, sess)
		return
	}
	body, err := decodeBody[knowledgeCreateBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	scope := normalizeScope(body.Scope)
	switch body.Kind {
	case "url":
		rawURL := strings.TrimSpace(body.URL)
		parsed, err := url.Parse(rawURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "url must be a valid http(s) URL")
			return
		}
		if len(rawURL) > 2000 {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "url is too long")
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			name = rawURL
		}
		if len(name) > 200 {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "name is too long")
			return
		}
		sourceID, err := s.repo.CreateKnowledgeSource(db.WithOrg(r.Context(), sess.OrgID), "url", name, scope, map[string]any{"url": rawURL})
		if err != nil {
			s.respondError(w, r, err)
			return
		}
		if _, err := s.queue.Send(r.Context(), "ingest-url", map[string]any{"orgId": sess.OrgID, "sourceId": sourceID, "url": rawURL}, queue.SendOptions{SingletonKey: "src:" + sourceID}); err != nil {
			s.respondError(w, r, err)
			return
		}
		s.auditSourceCreated(r, sess, sourceID, map[string]any{"kind": "url", "url": rawURL, "scope": scope})
		writeData(w, http.StatusCreated, map[string]any{"id": sourceID})
	case "text":
		name := strings.TrimSpace(body.Name)
		if name == "" || len(name) > 200 {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "name must be between 1 and 200 characters")
			return
		}
		if body.Text == "" || len(body.Text) > 500_000 {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "text must be between 1 and 500000 characters")
			return
		}
		sourceID, err := s.repo.CreateKnowledgeSource(db.WithOrg(r.Context(), sess.OrgID), "text", name, scope, map[string]any{"bytes": len(body.Text)})
		if err != nil {
			s.respondError(w, r, err)
			return
		}
		if _, err := s.queue.Send(r.Context(), "ingest-text", map[string]any{"orgId": sess.OrgID, "sourceId": sourceID, "text": body.Text, "name": name}, queue.SendOptions{SingletonKey: "src:" + sourceID}); err != nil {
			s.respondError(w, r, err)
			return
		}
		s.auditSourceCreated(r, sess, sourceID, map[string]any{"kind": "text", "name": name, "scope": scope})
		writeData(w, http.StatusCreated, map[string]any{"id": sourceID})
	default:
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "kind must be url or text")
	}
}

func (s *Server) createFileSource(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		s.respondError(w, r, apperr.BadRequest("invalid multipart body"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "file is required")
		return
	}
	defer file.Close()
	scope := normalizeScope(r.FormValue("scope"))
	stored, err := s.saveUpload(header.Filename, file)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	sourceID, err := s.repo.CreateKnowledgeSource(db.WithOrg(r.Context(), sess.OrgID), "file", header.Filename, scope, map[string]any{
		"path": stored, "filename": header.Filename,
	})
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if _, err := s.queue.Send(r.Context(), "ingest-file", map[string]any{
		"orgId": sess.OrgID, "sourceId": sourceID, "filePath": stored, "filename": header.Filename,
	}, queue.SendOptions{SingletonKey: "src:" + sourceID}); err != nil {
		s.respondError(w, r, err)
		return
	}
	s.auditSourceCreated(r, sess, sourceID, map[string]any{"kind": "file", "name": header.Filename, "scope": scope})
	writeData(w, http.StatusCreated, map[string]any{"id": sourceID})
}

func (s *Server) auditSourceCreated(r *http.Request, sess *auth.Session, sourceID string, payload map[string]any) {
	id := sourceID
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType:  db.ActorUser,
		ActorID:    &sess.UserID,
		Type:       "knowledge.source.created",
		TargetType: "knowledge_source",
		TargetID:   &id,
		Payload:    payload,
	})
}

type sourceUpdateBody struct {
	SourceID string `json:"sourceId"`
	Scope    string `json:"scope"`
}

func (s *Server) handleKnowledgeSourceUpdate(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapKnowledgeWrite)
	if !ok {
		return
	}
	body, err := decodeBody[sourceUpdateBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if body.SourceID == "" {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "sourceId is required")
		return
	}
	scope := "unchanged"
	if body.Scope != "" {
		scope = normalizeScope(body.Scope)
	}
	found, err := s.repo.UpdateKnowledgeSourceScope(db.WithOrg(r.Context(), sess.OrgID), body.SourceID, scope)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !found {
		s.respondError(w, r, apperr.NotFound("source not found"))
		return
	}
	s.auditSourceUpdated(r, sess, body.SourceID, map[string]any{"scope": scope})
	writeData(w, http.StatusOK, map[string]any{"updated": true})
}

func (s *Server) handleKnowledgeSourceDelete(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapKnowledgeWrite)
	if !ok {
		return
	}
	body, err := decodeBody[sourceUpdateBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	found, err := s.repo.DeleteKnowledgeSource(db.WithOrg(r.Context(), sess.OrgID), body.SourceID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !found {
		s.respondError(w, r, apperr.NotFound("source not found"))
		return
	}
	id := body.SourceID
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType:  db.ActorUser,
		ActorID:    &sess.UserID,
		Type:       "knowledge.source.deleted",
		TargetType: "knowledge_source",
		TargetID:   &id,
		Payload:    map[string]any{},
	})
	writeData(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) auditSourceUpdated(r *http.Request, sess *auth.Session, sourceID string, payload map[string]any) {
	id := sourceID
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType:  db.ActorUser,
		ActorID:    &sess.UserID,
		Type:       "knowledge.source.updated",
		TargetType: "knowledge_source",
		TargetID:   &id,
		Payload:    payload,
	})
}

type sourceConfig struct {
	URL      string `json:"url"`
	Path     string `json:"path"`
	Filename string `json:"filename"`
}

func (s *Server) enqueueReindex(ctx context.Context, orgID, sourceID string, source db.KnowledgeSourceDetail) error {
	var config sourceConfig
	if len(source.SourceConfig) > 0 {
		if err := json.Unmarshal(source.SourceConfig, &config); err != nil {
			return apperr.BadRequest("source cannot be reindexed")
		}
	}
	opts := queue.SendOptions{SingletonKey: "src:" + sourceID}
	switch source.Type {
	case "url":
		if config.URL == "" {
			return apperr.BadRequest("source cannot be reindexed")
		}
		_, err := s.queue.Send(ctx, "ingest-url", map[string]any{"orgId": orgID, "sourceId": sourceID, "url": config.URL}, opts)
		return err
	case "file":
		if config.Path == "" {
			return apperr.BadRequest("source cannot be reindexed")
		}
		filename := config.Filename
		if filename == "" {
			filename = source.Name
		}
		_, err := s.queue.Send(ctx, "ingest-file", map[string]any{"orgId": orgID, "sourceId": sourceID, "filePath": config.Path, "filename": filename}, opts)
		return err
	default:
		return apperr.BadRequest("source cannot be reindexed")
	}
}

type reindexBody struct {
	SourceID string `json:"sourceId"`
}

func (s *Server) handleKnowledgeSourceReindex(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapKnowledgeWrite)
	if !ok {
		return
	}
	body, err := decodeBody[reindexBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if body.SourceID == "" {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "sourceId is required")
		return
	}
	source, err := s.repo.GetKnowledgeSource(db.WithOrg(r.Context(), sess.OrgID), body.SourceID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if err := s.enqueueReindex(r.Context(), sess.OrgID, body.SourceID, source); err != nil {
		s.respondError(w, r, err)
		return
	}
	if err := s.repo.SetKnowledgeSourcePending(db.WithOrg(r.Context(), sess.OrgID), body.SourceID); err != nil {
		s.respondError(w, r, err)
		return
	}
	s.auditSourceUpdated(r, sess, body.SourceID, map[string]any{"reindex": true})
	writeData(w, http.StatusOK, map[string]any{"reindexing": true})
}
