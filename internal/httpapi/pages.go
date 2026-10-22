// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5"

	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/queue"
	"chastedesk/internal/ratelimit"
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
		s.renderPageError(w, r, http.StatusForbidden, "Not allowed", "Your role does not include permission for this page. Ask an owner or admin if you need access.")
		return nil, false
	}
	return sess, true
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	exists, err := s.repo.OrgExists(r.Context())
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !exists {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if sess, err := s.sessionFromRequest(r); err == nil && sess != nil && sess.OrgID != "" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleSetupPage(w http.ResponseWriter, r *http.Request) {
	exists, err := s.repo.OrgExists(r.Context())
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	renderPage(w, views.SetupPage("", "", "", ""))
}

func (s *Server) handleSetupSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	orgName := strings.TrimSpace(r.FormValue("orgName"))
	userName := strings.TrimSpace(r.FormValue("userName"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	errorMessage := ""
	switch {
	case orgName == "" || len(orgName) > 100:
		errorMessage = "Workspace name must be between 1 and 100 characters."
	case userName == "" || len(userName) > 100:
		errorMessage = "Your name must be between 1 and 100 characters."
	case !validEmail(email):
		errorMessage = "Enter a valid email address."
	case len(password) < 8 || len(password) > 200:
		errorMessage = "Password must be between 8 and 200 characters."
	}
	if errorMessage != "" {
		w.WriteHeader(http.StatusBadRequest)
		renderPage(w, views.SetupPage(errorMessage, orgName, userName, email))
		return
	}
	if _, err := ratelimit.Enforce(r.Context(), s.pool, "setup:"+clientIP(r), 10, 3600); err != nil {
		s.respondError(w, r, err)
		return
	}
	exists, err := s.repo.OrgExists(r.Context())
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	_, userID, err := s.repo.Bootstrap(r.Context(), db.BootstrapInput{
		OrgName: orgName, UserName: userName, Email: email, PasswordHash: hash,
	})
	if err != nil {
		if errors.Is(err, db.ErrOrgExists) || db.IsUniqueViolation(err) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		s.respondError(w, r, err)
		return
	}
	if err := s.startSession(w, r, userID); err != nil {
		s.respondError(w, r, err)
		return
	}
	http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	exists, err := s.repo.OrgExists(r.Context())
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !exists {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if sess, err := s.sessionFromRequest(r); err == nil && sess != nil && sess.OrgID != "" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
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
	if _, err := ratelimit.Enforce(r.Context(), s.pool, "auth:"+clientIP(r)+":"+strings.ToLower(email), 10, 900); err != nil {
		s.respondError(w, r, err)
		return
	}
	cred, credErr := s.repo.CredentialByEmail(r.Context(), email)
	if credErr != nil && !errors.Is(credErr, db.ErrNotFound) {
		s.respondError(w, r, credErr)
		return
	}
	if credErr == nil && auth.VerifyPassword(password, cred.PasswordHash) {
		if record, recordErr := s.repo.TwoFactor(db.RepoCtx{Ctx: r.Context()}, cred.User.ID); recordErr == nil && record.Enabled {
			renderPage(w, views.LoginPage("This account requires two-factor authentication. Use the console sign-in page."))
			return
		}
		if err := s.startSession(w, r, cred.User.ID); err != nil {
			s.respondError(w, r, err)
			return
		}
		s.auditAuthEvent(r, cred.User.ID, "auth.login", "session")
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	if credErr == nil {
		s.auditAuthEvent(r, cred.User.ID, "auth.failed", "user")
	}
	w.WriteHeader(http.StatusUnauthorized)
	renderPage(w, views.LoginPage("Email and password did not match an account on this desk."))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if token := s.sessionToken(r); token != "" {
		_ = s.repo.DeleteSession(r.Context(), tokenHash(token))
	}
	s.clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleOnboardingPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	shell, err := s.shellFor(r, sess, "onboarding", "Set up your desk", "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	ctx := db.WithOrg(r.Context(), sess.OrgID)
	state, err := s.repo.OnboardingState(ctx)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	counts, err := s.repo.OverviewCounts(ctx)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	renderPage(w, views.OnboardingPage(shell, views.OnboardingView{
		ProviderConfigured: state.ProviderConfigured,
		KnowledgeAdded:     state.KnowledgeAdded,
		Complete:           state.Complete,
		ProviderCount:      counts.EnabledProviders,
		SourceCount:        counts.Sources,
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
	http.Redirect(w, r, "/onboarding?msg=Provider+connected.", http.StatusSeeOther)
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
	http.Redirect(w, r, "/onboarding?msg=Knowledge+source+queued.", http.StatusSeeOther)
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
	http.Redirect(w, r, "/dashboard?msg=Setup+complete.", http.StatusSeeOther)
}

func sourceViews(rows []db.KnowledgeSourceRow) []views.SourceView {
	out := make([]views.SourceView, 0, len(rows))
	for _, row := range rows {
		view := views.SourceView{
			ID: row.ID, Name: row.Name, Type: row.Type, Scope: row.Scope, Status: row.Status,
			Version: row.Version, Chunks: row.Chunks, Vector: row.Vector,
			CreatedAt: views.RelativeTime(row.CreatedAt),
		}
		if row.Error != nil {
			view.Error = *row.Error
		}
		out = append(out, view)
	}
	return out
}

func (s *Server) knowledgeView(r *http.Request, sess *auth.Session) (views.KnowledgeView, error) {
	rows, err := s.repo.ListKnowledgeSources(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		return views.KnowledgeView{}, err
	}
	view := views.KnowledgeView{Sources: sourceViews(rows), CanWrite: auth.Can(sess.Role, auth.CapKnowledgeWrite)}
	for _, source := range view.Sources {
		view.TotalSources++
		view.TotalChunks += source.Chunks
		if source.Vector {
			view.VectorSearch = true
		}
		switch source.Status {
		case "ready":
			view.ReadySources++
		case "failed":
			view.FailedSources++
		default:
			view.ProcessingSources++
		}
	}
	view.AddMode = r.URL.Query().Get("add")
	switch view.AddMode {
	case "file", "text":
	default:
		view.AddMode = "url"
	}
	return view, nil
}

func (s *Server) handleKnowledgePage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapKnowledgeRead)
	if !ok {
		return
	}
	shell, err := s.shellFor(r, sess, "knowledge", "Knowledge", "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	view, err := s.knowledgeView(r, sess)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	view.Message = r.URL.Query().Get("msg")
	renderPage(w, views.KnowledgePage(shell, view))
}

func (s *Server) handleKnowledgeSourcesPartial(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapKnowledgeRead)
	if !ok {
		return
	}
	view, err := s.knowledgeView(r, sess)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.KnowledgeSourcesCard(view).Render(r.Context(), w)
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
			http.Redirect(w, r, "/dashboard/knowledge?msg=File+queued+for+indexing.", http.StatusSeeOther)
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
		http.Redirect(w, r, "/dashboard/knowledge?msg=Website+queued+for+crawling.", http.StatusSeeOther)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	text := r.FormValue("text")
	if name == "" || text == "" {
		http.Redirect(w, r, "/dashboard/knowledge?msg=Provide+a+URL%2C+file%2C+or+pasted+text&tone=warn", http.StatusSeeOther)
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
	http.Redirect(w, r, "/dashboard/knowledge?msg=Text+queued+for+indexing.", http.StatusSeeOther)
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
	http.Redirect(w, r, "/dashboard/knowledge?msg=Scope+updated.", http.StatusSeeOther)
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
	http.Redirect(w, r, "/dashboard/knowledge?msg=Source+deleted.", http.StatusSeeOther)
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
	http.Redirect(w, r, "/dashboard/knowledge?msg=Reindex+queued.", http.StatusSeeOther)
}

func (s *Server) widgetSnippet(r *http.Request, orgID string) (string, string, error) {
	config, err := s.repo.GetWidgetConfig(db.WithOrg(r.Context(), orgID))
	if err != nil {
		return "", "", err
	}
	origins := []string{}
	if config != nil {
		origins = config.Origins
	}
	token, err := widget.SignToken(s.cfg.AppSecret, orgID, origins, time.Now())
	if err != nil {
		return "", "", err
	}
	snippet := `<script src="` + s.cfg.AppURL + `/widget.js" data-org="` + orgID + `" data-token="` + token + `" defer></script>`
	return strings.Join(origins, "\n"), snippet, nil
}

func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return string(raw)
	}
	encoded, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(encoded)
}

func queryParamEscape(value string) string {
	return url.QueryEscape(value)
}

func pgxNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func (s *Server) renderPageError(w http.ResponseWriter, r *http.Request, status int, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = views.ErrorPage(status, title, body).Render(r.Context(), w)
}
