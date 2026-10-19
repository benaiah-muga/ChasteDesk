// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/config"
	"chastedesk/internal/db"
	"chastedesk/internal/queue"
	"chastedesk/internal/web"
)

type Server struct {
	cfg    config.Config
	logger *slog.Logger
	pool   *pgxpool.Pool
	repo   *db.Repo
	queue  *queue.Queue
}

func New(cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool) http.Handler {
	s := &Server{cfg: cfg, logger: logger, pool: pool}
	if pool != nil {
		s.repo = db.NewRepo(pool)
		s.queue = queue.New(pool)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("POST /api/bootstrap", s.handleBootstrap)
	mux.HandleFunc("POST /api/auth/sign-up/email", s.handleSignUp)
	mux.HandleFunc("POST /api/auth/sign-in/email", s.handleSignIn)
	mux.HandleFunc("POST /api/auth/sign-out", s.handleSignOut)
	mux.HandleFunc("GET /api/auth/get-session", s.handleGetSession)
	mux.HandleFunc("GET /api/onboarding/status", s.handleOnboardingStatus)
	mux.HandleFunc("POST /api/onboarding/complete", s.handleOnboardingComplete)
	mux.HandleFunc("GET /api/audit", s.handleAuditList)
	mux.HandleFunc("GET /api/analytics/summary", s.handleAnalyticsSummary)
	mux.HandleFunc("POST /api/analytics/rollup", s.handleAnalyticsRollup)
	mux.HandleFunc("GET /api/actions", s.handleActionsList)
	mux.HandleFunc("POST /api/actions/decide", s.handleActionDecide)
	mux.HandleFunc("GET /api/conversations/stream", s.handleConversationsStream)
	mux.HandleFunc("GET /api/widget/config", s.handleWidgetConfig)
	mux.HandleFunc("POST /api/widget/conversations", s.handleWidgetConversationCreate)
	mux.HandleFunc("POST /api/widget/conversations/{id}/messages", s.handleWidgetMessageCreate)
	mux.HandleFunc("GET /api/widget/conversations/{id}/stream", s.handleWidgetStream)
	mux.HandleFunc("POST /api/v1/conversations", s.handleAPIConversationCreate)
	mux.HandleFunc("GET /api/v1/conversations", s.handleAPIConversationList)
	mux.HandleFunc("PUT /api/v1/conversations", s.handleAPIConversationPost)
	mux.HandleFunc("POST /api/v1/conversations/{id}/messages", s.handleAPIMessageCreate)
	mux.HandleFunc("GET /api/inbox/conversations", s.handleInboxConversations)
	mux.HandleFunc("GET /api/inbox/conversations/{id}", s.handleInboxConversation)
	mux.HandleFunc("POST /api/inbox/conversations/{id}/reply", s.handleInboxReply)
	mux.HandleFunc("POST /api/inbox/conversations/{id}/resolve", s.handleInboxResolve)
	mux.HandleFunc("POST /api/inbox/conversations/{id}/assign", s.handleInboxAssign)
	mux.HandleFunc("POST /api/copilot/draft", s.handleCopilotDraft)
	mux.HandleFunc("POST /api/copilot/send", s.handleCopilotSend)
	mux.HandleFunc("GET /widget.js", s.handleWidgetScript)
	mux.HandleFunc("GET /api/settings/providers", s.handleProvidersList)
	mux.HandleFunc("POST /api/settings/providers", s.handleProviderSave)
	mux.HandleFunc("GET /api/settings/models", s.handleModelsList)
	mux.HandleFunc("POST /api/settings/providers-connect", s.handleProviderConnect)
	mux.HandleFunc("POST /api/settings/providers/{id}/connect", s.handleProviderConnectByID)
	mux.HandleFunc("DELETE /api/settings/providers/{id}", s.handleProviderDelete)
	mux.HandleFunc("GET /api/knowledge/sources", s.handleKnowledgeSourcesList)
	mux.HandleFunc("POST /api/knowledge/sources", s.handleKnowledgeSourceCreate)
	mux.HandleFunc("POST /api/knowledge/sources-update", s.handleKnowledgeSourceUpdate)
	mux.HandleFunc("DELETE /api/knowledge/sources-update", s.handleKnowledgeSourceDelete)
	mux.HandleFunc("POST /api/knowledge/sources-reindex", s.handleKnowledgeSourceReindex)
	mux.HandleFunc("GET /api/settings/widget", s.handleWidgetGet)
	mux.HandleFunc("PUT /api/settings/widget", s.handleWidgetPut)
	mux.HandleFunc("GET /api/settings/widget-token", s.handleWidgetToken)
	mux.Handle("GET /static/", http.FileServerFS(web.Static))
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLoginSubmit)
	mux.HandleFunc("POST /logout", s.handleLogout)
	mux.HandleFunc("GET /onboarding", s.handleOnboardingPage)
	mux.HandleFunc("POST /onboarding/provider", s.handleOnboardingProvider)
	mux.HandleFunc("POST /onboarding/add", s.handleOnboardingAdd)
	mux.HandleFunc("POST /onboarding/finish", s.handleOnboardingFinish)
	mux.HandleFunc("GET /dashboard/knowledge", s.handleKnowledgePage)
	mux.HandleFunc("POST /dashboard/knowledge/add", s.handleKnowledgeAdd)
	mux.HandleFunc("POST /dashboard/knowledge/scope", s.handleKnowledgeScope)
	mux.HandleFunc("POST /dashboard/knowledge/delete", s.handleKnowledgeDelete)
	mux.HandleFunc("POST /dashboard/knowledge/reindex", s.handleKnowledgeReindex)
	mux.HandleFunc("GET /dashboard/settings", s.handleSettingsPage)
	mux.HandleFunc("POST /dashboard/settings/providers", s.handleSettingsProviderSave)
	mux.HandleFunc("POST /dashboard/settings/providers/test", s.handleSettingsProviderTest)
	mux.HandleFunc("POST /dashboard/settings/providers/remove", s.handleSettingsProviderRemove)
	mux.HandleFunc("POST /dashboard/settings/widget", s.handleSettingsWidgetSave)
	mux.HandleFunc("GET /dashboard/inbox", s.handleInboxPage)
	mux.HandleFunc("GET /dashboard/inbox/{id}", s.handleInboxDetailPage)
	mux.HandleFunc("POST /dashboard/inbox/action", s.handleInboxActionForm)
	mux.HandleFunc("POST /dashboard/inbox/{id}/reply", s.handleInboxReplyForm)
	mux.HandleFunc("POST /dashboard/inbox/{id}/resolve", s.handleInboxResolveForm)
	mux.HandleFunc("POST /dashboard/inbox/{id}/copilot", s.handleInboxCopilotForm)
	mux.HandleFunc("POST /dashboard/inbox/{id}/copilot/send", s.handleInboxCopilotSendForm)
	mux.HandleFunc("GET /dashboard/analytics", s.handleAnalyticsPage)
	mux.HandleFunc("GET /dashboard/audit", s.handleAuditPage)
	mux.HandleFunc("/", s.handleNotFound)
	return s.withRequestLog(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeErrorBody(w, http.StatusNotFound, "not_found", "not found")
}

func (s *Server) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.logger.Debug("request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func writeData(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, map[string]any{"data": data})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErrorBody(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
