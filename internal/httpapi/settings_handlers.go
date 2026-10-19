// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"chastedesk/internal/apperr"
	"chastedesk/internal/auth"
	"chastedesk/internal/crypto"
	"chastedesk/internal/db"
	"chastedesk/internal/model"
)

func (s *Server) handleAuditList(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapAuditRead)
	if !ok {
		return
	}
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	rows, err := s.repo.ListAuditEvents(db.WithOrg(r.Context(), sess.OrgID), r.URL.Query().Get("type"), r.URL.Query().Get("q"), limit)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, rows)
}

func (s *Server) handleProvidersList(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsRead)
	if !ok {
		return
	}
	rows, err := s.repo.ListProviders(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, rows)
}

type providerSaveBody struct {
	ProviderID  string `json:"providerId"`
	DisplayName string `json:"displayName"`
	APIKey      string `json:"apiKey"`
	BaseURL     string `json:"baseUrl"`
}

func (s *Server) handleProviderSave(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	body, err := decodeBody[providerSaveBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if err := s.saveProviderConfig(r.Context(), sess, body); err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"saved": true})
}

func (s *Server) saveProviderConfig(ctx context.Context, sess *auth.Session, body providerSaveBody) error {
	body.ProviderID = strings.TrimSpace(body.ProviderID)
	body.DisplayName = strings.TrimSpace(body.DisplayName)
	if !model.KnownProvider(body.ProviderID) {
		return apperr.BadRequest("unknown provider")
	}
	if body.DisplayName == "" {
		body.DisplayName = providerDefaultName(body.ProviderID)
	}
	if len(body.DisplayName) > 100 {
		return apperr.BadRequest("displayName must be between 1 and 100 characters")
	}
	if len(body.APIKey) > 500 {
		return apperr.BadRequest("apiKey is too long")
	}
	if !model.ValidBaseURL(body.BaseURL) {
		return apperr.BadRequest("baseUrl must be a valid URL")
	}
	in := db.ProviderUpsert{ProviderID: body.ProviderID, DisplayName: body.DisplayName}
	if body.BaseURL != "" {
		in.BaseURL = &body.BaseURL
	}
	if body.APIKey != "" {
		sealed, err := crypto.SealSecret(body.APIKey, s.cfg.AppSecret)
		if err != nil {
			return err
		}
		in.Sealed = &sealed
	}
	if err := s.repo.UpsertProviderConfig(db.WithOrg(ctx, sess.OrgID), in); err != nil {
		return err
	}
	providerID := body.ProviderID
	return s.repo.WriteAudit(db.WithOrg(ctx, sess.OrgID), db.AuditInput{
		ActorType:  db.ActorUser,
		ActorID:    &sess.UserID,
		Type:       "settings.provider.saved",
		TargetType: "provider_config",
		TargetID:   &providerID,
		Payload:    map[string]any{"providerId": providerID, "hasKey": in.Sealed != nil},
	})
}

func (s *Server) resolveProviderConfig(ctx context.Context, orgID string, row db.ProviderConfig) (model.Config, error) {
	cfg := model.Config{}
	if row.BaseURL != nil {
		cfg.BaseURL = *row.BaseURL
	}
	if row.KeyCipher != nil && row.KeyIV != nil && row.KeyTag != nil {
		version := 0
		if row.KeyVersion != nil {
			version = *row.KeyVersion
		}
		secret, err := crypto.OpenSecret(crypto.SealedSecret{
			Ciphertext: *row.KeyCipher,
			IV:         *row.KeyIV,
			AuthTag:    *row.KeyTag,
			KeyVersion: version,
		}, s.cfg.AppSecret)
		if err != nil {
			return cfg, err
		}
		cfg.APIKey = secret
	}
	return cfg, nil
}

func (s *Server) providerConfigForID(r *http.Request, orgID, configID string) (db.ProviderConfig, model.Config, error) {
	row, err := s.repo.GetProviderConfigByID(db.WithOrg(r.Context(), orgID), configID)
	if errors.Is(err, db.ErrNotFound) {
		return row, model.Config{}, apperr.NotFound("provider config not found")
	}
	if err != nil {
		return row, model.Config{}, err
	}
	if !row.Enabled {
		return row, model.Config{}, apperr.NotFound("provider config not found")
	}
	cfg, err := s.resolveProviderConfig(r.Context(), orgID, row)
	return row, cfg, err
}

func (s *Server) handleModelsList(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsRead)
	if !ok {
		return
	}
	configID := r.URL.Query().Get("configId")
	if configID == "" {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "configId is required")
		return
	}
	row, cfg, err := s.providerConfigForID(r, sess.OrgID, configID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	models, err := model.For(model.ProviderID(row.ProviderID)).ListModels(r.Context(), cfg)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if encoded, err := json.Marshal(models); err == nil {
		_ = s.repo.UpdateModelsCache(db.WithOrg(r.Context(), sess.OrgID), configID, encoded)
	}
	writeData(w, http.StatusOK, models)
}

type providerConnectBody struct {
	ConfigID string `json:"configId"`
}

func (s *Server) handleProviderConnect(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	body, err := decodeBody[providerConnectBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if body.ConfigID == "" {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "configId is required")
		return
	}
	s.providerConnectResult(w, r, sess, body.ConfigID)
}

func (s *Server) handleProviderConnectByID(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	s.providerConnectResult(w, r, sess, r.PathValue("id"))
}

func (s *Server) providerConnectResult(w http.ResponseWriter, r *http.Request, sess *auth.Session, configID string) {
	row, cfg, err := s.providerConfigForID(r, sess.OrgID, configID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	audit := func(payload map[string]any) {
		id := configID
		_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
			ActorType:  db.ActorUser,
			ActorID:    &sess.UserID,
			Type:       "settings.provider.tested",
			TargetType: "provider_config",
			TargetID:   &id,
			Payload:    payload,
		})
	}
	models, listErr := model.For(model.ProviderID(row.ProviderID)).ListModels(r.Context(), cfg)
	if listErr != nil {
		audit(map[string]any{"ok": false, "error": listErr.Error()})
		writeData(w, http.StatusOK, map[string]any{"ok": false, "error": listErr.Error()})
		return
	}
	if encoded, err := json.Marshal(models); err == nil {
		_ = s.repo.UpdateModelsCache(db.WithOrg(r.Context(), sess.OrgID), configID, encoded)
	}
	audit(map[string]any{"ok": true, "models": len(models)})
	writeData(w, http.StatusOK, map[string]any{"ok": true, "models": models})
}

func (s *Server) handleProviderDelete(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	id := r.PathValue("id")
	found, err := s.repo.DeleteProviderConfig(db.WithOrg(r.Context(), sess.OrgID), id)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !found {
		s.respondError(w, r, apperr.NotFound("provider config not found"))
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType:  db.ActorUser,
		ActorID:    &sess.UserID,
		Type:       "settings.provider.saved",
		TargetType: "provider_config",
		TargetID:   &id,
		Payload:    map[string]any{"deleted": true},
	})
	writeData(w, http.StatusOK, map[string]any{"deleted": true})
}

func providerDefaultName(providerID string) string {
	switch providerID {
	case "openai":
		return "OpenAI"
	case "anthropic":
		return "Anthropic"
	case "google":
		return "Google AI"
	case "ollama":
		return "Ollama (local)"
	case "openai-compatible":
		return "OpenAI-compatible endpoint"
	}
	return providerID
}
