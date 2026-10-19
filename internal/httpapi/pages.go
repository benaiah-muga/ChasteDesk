// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/model"
	"chastedesk/internal/queue"
	"chastedesk/internal/web/views"
	"chastedesk/internal/widget"
)

func renderPage(w http.ResponseWriter, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = component.Render(context.Background(), w)
}

func (s *Server) pageSession(w http.ResponseWriter, r *http.Request, capability auth.Capability) (*auth.Session, bool) {
	sess, err := s.sessionFromRequest(r)
	if err != nil {
		s.respondError(w, r, err)
		return nil, false
	}
	if sess == nil || sess.OrgID == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return nil, false
	}
	if capability != "" && !auth.Can(sess.Role, capability) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	return sess, true
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if sess, err := s.sessionFromRequest(r); err == nil && sess != nil {
		http.Redirect(w, r, "/dashboard/knowledge", http.StatusSeeOther)
		return
	}
	renderPage(w, views.LoginPage(""))
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	cred, credErr := s.repo.CredentialByEmail(r.Context(), email)
	if credErr != nil && !errors.Is(credErr, db.ErrNotFound) {
		s.respondError(w, r, credErr)
		return
	}
	if credErr == nil && auth.VerifyPassword(password, cred.PasswordHash) {
		if err := s.startSession(w, r, cred.User.ID); err != nil {
			s.respondError(w, r, err)
			return
		}
		s.auditAuthEvent(r, cred.User.ID, "auth.login", "session")
		http.Redirect(w, r, "/dashboard/knowledge", http.StatusSeeOther)
		return
	}
	w.WriteHeader(http.StatusUnauthorized)
	renderPage(w, views.LoginPage("Invalid email or password."))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if token := s.sessionToken(r); token != "" {
		_ = s.repo.DeleteSession(r.Context(), tokenHash(token))
	}
	s.clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleOnboardingPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, "")
	if !ok {
		return
	}
	state, err := s.repo.OnboardingState(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	renderPage(w, views.OnboardingPage(views.OnboardingView{
		ProviderConfigured: state.ProviderConfigured,
		KnowledgeAdded:     state.KnowledgeAdded,
		Complete:           state.Complete,
	}))
}

func (s *Server) handleOnboardingProvider(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	body := providerSaveBody{
		ProviderID:  r.FormValue("providerId"),
		DisplayName: r.FormValue("displayName"),
		APIKey:      r.FormValue("apiKey"),
		BaseURL:     r.FormValue("baseUrl"),
	}
	if err := s.saveProviderConfig(r.Context(), sess, body); err != nil {
		s.respondError(w, r, err)
		return
	}
	http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
}

func (s *Server) handleOnboardingAdd(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapKnowledgeWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	rawURL := strings.TrimSpace(r.FormValue("url"))
	if rawURL == "" {
		rawURL = "https://example.com/help"
	}
	sourceID, err := s.repo.CreateKnowledgeSource(db.WithOrg(r.Context(), sess.OrgID), "url", rawURL, "public", map[string]any{"url": rawURL})
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if _, err := s.queue.Send(r.Context(), "ingest-url", map[string]any{"orgId": sess.OrgID, "sourceId": sourceID, "url": rawURL}, queue.SendOptions{SingletonKey: "src:" + sourceID}); err != nil {
		s.respondError(w, r, err)
		return
	}
	s.auditSourceCreated(r, sess, sourceID, map[string]any{"kind": "url", "url": rawURL, "scope": "public"})
	http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
}

func (s *Server) handleOnboardingFinish(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapOrgAdmin)
	if !ok {
		return
	}
	if err := s.repo.SetOnboardingComplete(db.WithOrg(r.Context(), sess.OrgID), true); err != nil {
		s.respondError(w, r, err)
		return
	}
	http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
}

func (s *Server) handleKnowledgePage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapKnowledgeRead)
	if !ok {
		return
	}
	rows, err := s.repo.ListKnowledgeSources(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	sources := make([]views.SourceView, 0, len(rows))
	for _, row := range rows {
		view := views.SourceView{ID: row.ID, Name: row.Name, Type: row.Type, Scope: row.Scope, Status: row.Status, Version: row.Version}
		if row.Error != nil {
			view.Error = *row.Error
		}
		sources = append(sources, view)
	}
	renderPage(w, views.KnowledgePage(sources, r.URL.Query().Get("msg")))
}

func (s *Server) handleKnowledgeAdd(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapKnowledgeWrite)
	if !ok {
		return
	}
	multipart := strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data")
	if multipart {
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	} else if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	scope := normalizeScope(r.FormValue("scope"))
	if multipart {
		file, header, fileErr := r.FormFile("file")
		if fileErr == nil && header != nil && header.Filename != "" {
			defer file.Close()
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
			http.Redirect(w, r, "/dashboard/knowledge", http.StatusSeeOther)
			return
		}
	}
	if rawURL := strings.TrimSpace(r.FormValue("url")); rawURL != "" {
		sourceID, err := s.repo.CreateKnowledgeSource(db.WithOrg(r.Context(), sess.OrgID), "url", rawURL, scope, map[string]any{"url": rawURL})
		if err != nil {
			s.respondError(w, r, err)
			return
		}
		if _, err := s.queue.Send(r.Context(), "ingest-url", map[string]any{"orgId": sess.OrgID, "sourceId": sourceID, "url": rawURL}, queue.SendOptions{SingletonKey: "src:" + sourceID}); err != nil {
			s.respondError(w, r, err)
			return
		}
		s.auditSourceCreated(r, sess, sourceID, map[string]any{"kind": "url", "url": rawURL, "scope": scope})
		http.Redirect(w, r, "/dashboard/knowledge", http.StatusSeeOther)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	text := r.FormValue("text")
	if name == "" || text == "" {
		http.Redirect(w, r, "/dashboard/knowledge?msg=Provide+a+URL%2C+file%2C+or+pasted+text", http.StatusSeeOther)
		return
	}
	sourceID, err := s.repo.CreateKnowledgeSource(db.WithOrg(r.Context(), sess.OrgID), "text", name, scope, map[string]any{"bytes": len(text)})
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if _, err := s.queue.Send(r.Context(), "ingest-text", map[string]any{"orgId": sess.OrgID, "sourceId": sourceID, "text": text, "name": name}, queue.SendOptions{SingletonKey: "src:" + sourceID}); err != nil {
		s.respondError(w, r, err)
		return
	}
	s.auditSourceCreated(r, sess, sourceID, map[string]any{"kind": "text", "name": name, "scope": scope})
	http.Redirect(w, r, "/dashboard/knowledge", http.StatusSeeOther)
}

func (s *Server) handleKnowledgeScope(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapKnowledgeWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	sourceID := r.FormValue("sourceId")
	scope := normalizeScope(r.FormValue("scope"))
	found, err := s.repo.UpdateKnowledgeSourceScope(db.WithOrg(r.Context(), sess.OrgID), sourceID, scope)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if found {
		s.auditSourceUpdated(r, sess, sourceID, map[string]any{"scope": scope})
	}
	http.Redirect(w, r, "/dashboard/knowledge", http.StatusSeeOther)
}

func (s *Server) handleKnowledgeDelete(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapKnowledgeWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	sourceID := r.FormValue("sourceId")
	found, err := s.repo.DeleteKnowledgeSource(db.WithOrg(r.Context(), sess.OrgID), sourceID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if found {
		id := sourceID
		_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
			ActorType:  db.ActorUser,
			ActorID:    &sess.UserID,
			Type:       "knowledge.source.deleted",
			TargetType: "knowledge_source",
			TargetID:   &id,
			Payload:    map[string]any{},
		})
	}
	http.Redirect(w, r, "/dashboard/knowledge", http.StatusSeeOther)
}

func (s *Server) handleKnowledgeReindex(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapKnowledgeWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	sourceID := r.FormValue("sourceId")
	source, err := s.repo.GetKnowledgeSource(db.WithOrg(r.Context(), sess.OrgID), sourceID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Redirect(w, r, "/dashboard/knowledge", http.StatusSeeOther)
			return
		}
		s.respondError(w, r, err)
		return
	}
	if err := s.enqueueReindex(r.Context(), sess.OrgID, sourceID, source); err == nil {
		_ = s.repo.SetKnowledgeSourcePending(db.WithOrg(r.Context(), sess.OrgID), sourceID)
		s.auditSourceUpdated(r, sess, sourceID, map[string]any{"reindex": true})
	}
	http.Redirect(w, r, "/dashboard/knowledge", http.StatusSeeOther)
}

func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsRead)
	if !ok {
		return
	}
	rows, err := s.repo.ListProviders(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	providers := make([]views.ProviderView, 0, len(rows))
	for _, row := range rows {
		view := views.ProviderView{ID: row.ID, ProviderID: row.ProviderID, DisplayName: row.DisplayName}
		if row.KeyLast4 != nil {
			view.KeyLast4 = *row.KeyLast4
		}
		if row.BaseURL != nil {
			view.BaseURL = *row.BaseURL
		}
		providers = append(providers, view)
	}
	config, err := s.repo.GetWidgetConfig(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	origins := []string{}
	if config != nil {
		origins = config.Origins
	}
	token, err := widget.SignToken(s.cfg.AppSecret, sess.OrgID, origins, time.Now())
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	snippet := `<script src="` + s.cfg.AppURL + `/widget.js" data-org="` + sess.OrgID + `" data-token="` + token + `" defer></script>`
	renderPage(w, views.SettingsPage(providers, strings.Join(origins, "\n"), snippet, r.URL.Query().Get("msg")))
}

func (s *Server) handleSettingsProviderSave(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	body := providerSaveBody{
		ProviderID:  r.FormValue("providerId"),
		DisplayName: r.FormValue("displayName"),
		APIKey:      r.FormValue("apiKey"),
		BaseURL:     r.FormValue("baseUrl"),
	}
	if err := s.saveProviderConfig(r.Context(), sess, body); err != nil {
		s.respondError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard/settings?msg=Provider+saved.+The+key+is+stored+encrypted.", http.StatusSeeOther)
}

func (s *Server) handleSettingsProviderTest(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	configID := r.FormValue("configId")
	row, cfg, err := s.providerConfigForID(r, sess.OrgID, configID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	models, listErr := model.For(model.ProviderID(row.ProviderID)).ListModels(r.Context(), cfg)
	id := configID
	if listErr != nil {
		_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
			ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "settings.provider.tested",
			TargetType: "provider_config", TargetID: &id, Payload: map[string]any{"ok": false, "error": listErr.Error()},
		})
		http.Redirect(w, r, "/dashboard/settings?msg=Connection+failed", http.StatusSeeOther)
		return
	}
	if encoded, err := json.Marshal(models); err == nil {
		_ = s.repo.UpdateModelsCache(db.WithOrg(r.Context(), sess.OrgID), configID, encoded)
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "settings.provider.tested",
		TargetType: "provider_config", TargetID: &id, Payload: map[string]any{"ok": true, "models": len(models)},
	})
	message := url.QueryEscape("Connected. " + strconv.Itoa(len(models)) + " models available.")
	http.Redirect(w, r, "/dashboard/settings?msg="+message, http.StatusSeeOther)
}

func (s *Server) handleSettingsProviderRemove(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	configID := r.FormValue("configId")
	found, err := s.repo.DeleteProviderConfig(db.WithOrg(r.Context(), sess.OrgID), configID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if found {
		id := configID
		_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
			ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "settings.provider.saved",
			TargetType: "provider_config", TargetID: &id, Payload: map[string]any{"deleted": true},
		})
	}
	http.Redirect(w, r, "/dashboard/settings", http.StatusSeeOther)
}

func (s *Server) handleSettingsWidgetSave(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	origins := []string{}
	for _, line := range strings.Split(r.FormValue("origins"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	config, err := s.repo.GetWidgetConfig(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	theme := []byte(`{}`)
	if config != nil && config.Theme != nil {
		theme = config.Theme
	}
	if err := s.repo.UpsertWidgetConfig(db.WithOrg(r.Context(), sess.OrgID), origins, theme); err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "settings.widget.updated",
		TargetType: "widget_config", Payload: map[string]any{"origins": origins},
	})
	http.Redirect(w, r, "/dashboard/settings?msg=Widget+settings+saved.", http.StatusSeeOther)
}
