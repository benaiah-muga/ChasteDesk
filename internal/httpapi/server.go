// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/config"
	"chastedesk/internal/db"
	"chastedesk/internal/observability"
	"chastedesk/internal/queue"
	"chastedesk/internal/web"
	"chastedesk/internal/widget"
)

type Server struct {
	cfg      config.Config
	logger   *slog.Logger
	pool     *pgxpool.Pool
	repo     *db.Repo
	queue    *queue.Queue
	metrics  *observability.Metrics
	notifier *observability.Notifier
}

func New(cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool) http.Handler {
	s := &Server{cfg: cfg, logger: logger, pool: pool, metrics: observability.NewMetrics()}
	s.notifier = observability.NewNotifier(cfg.AlertWebhookURL, cfg.AlertWebhookSecret, logger, s.metrics)
	if pool != nil {
		s.repo = db.NewRepo(pool)
		s.queue = queue.New(pool)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleRoot)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/ready", s.handleReady)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	mux.HandleFunc("POST /api/bootstrap", s.handleBootstrap)
	mux.HandleFunc("POST /api/auth/sign-up/email", s.handleSignUp)
	mux.HandleFunc("POST /api/auth/sign-in/email", s.handleSignIn)
	mux.HandleFunc("POST /api/auth/sign-out", s.handleSignOut)
	mux.HandleFunc("POST /api/auth/two-factor/verify", s.handleTwoFactorVerify)
	mux.HandleFunc("POST /api/email/inbound/postmark", s.handleEmailInbound)
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
	mux.HandleFunc("GET /api/widget/conversations/{id}/history", s.handleWidgetHistory)
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
	mux.HandleFunc("GET /api/app/session", s.handleConsoleSession)
	mux.HandleFunc("POST /api/app/setup", s.handleConsoleSetup)
	mux.HandleFunc("GET /api/app/overview", s.handleConsoleOverview)
	mux.HandleFunc("GET /api/app/inbox", s.handleConsoleInbox)
	mux.HandleFunc("POST /api/app/inbox/bulk", s.handleConsoleInboxBulk)
	mux.HandleFunc("PATCH /api/app/inbox/{id}/ticket", s.handleConsoleTicketUpdate)
	mux.HandleFunc("GET /api/app/canned-replies", s.handleConsoleCannedReplies)
	mux.HandleFunc("POST /api/app/canned-replies", s.handleConsoleCannedReplyCreate)
	mux.HandleFunc("PATCH /api/app/canned-replies/{id}", s.handleConsoleCannedReplyUpdate)
	mux.HandleFunc("DELETE /api/app/canned-replies/{id}", s.handleConsoleCannedReplyDelete)
	mux.HandleFunc("GET /api/app/customers", s.handleConsoleCustomers)
	mux.HandleFunc("GET /api/app/customers/{id}", s.handleConsoleCustomer)
	mux.HandleFunc("GET /api/app/approvals", s.handleConsoleApprovals)
	mux.HandleFunc("POST /api/app/approvals/bulk", s.handleConsoleApprovalsBulk)
	mux.HandleFunc("GET /api/app/knowledge", s.handleConsoleKnowledge)
	mux.HandleFunc("POST /api/app/knowledge", s.handleKnowledgeSourceCreate)
	mux.HandleFunc("PATCH /api/app/knowledge/{id}", s.handleConsoleKnowledgeUpdate)
	mux.HandleFunc("POST /api/app/knowledge/{id}/reindex", s.handleConsoleKnowledgeReindex)
	mux.HandleFunc("DELETE /api/app/knowledge/{id}", s.handleConsoleKnowledgeDelete)
	mux.HandleFunc("GET /api/app/analytics", s.handleConsoleAnalytics)
	mux.HandleFunc("GET /api/app/audit", s.handleConsoleAudit)
	mux.HandleFunc("GET /api/app/audit/export", s.handleAuditExport)
	mux.HandleFunc("GET /api/app/search", s.handleConsoleSearch)
	mux.HandleFunc("GET /api/app/settings", s.handleConsoleSettings)
	mux.HandleFunc("POST /api/app/settings/providers", s.handleConsoleProviderSave)
	mux.HandleFunc("DELETE /api/app/settings/providers/{id}", s.handleConsoleProviderDelete)
	mux.HandleFunc("POST /api/app/settings/widget", s.handleConsoleWidgetSave)
	mux.HandleFunc("POST /api/app/settings/agent", s.handleConsoleAgentSave)
	mux.HandleFunc("POST /api/app/settings/api-keys", s.handleConsoleAPIKeyCreate)
	mux.HandleFunc("DELETE /api/app/settings/api-keys/{id}", s.handleConsoleAPIKeyRevoke)
	mux.HandleFunc("POST /api/app/settings/workspace", s.handleConsoleWorkspaceSave)
	mux.HandleFunc("POST /api/app/settings/scim/tokens", s.handleConsoleSCIMTokenCreate)
	mux.HandleFunc("POST /api/app/settings/email/test", s.handleConsoleEmailTest)
	mux.HandleFunc("DELETE /api/app/settings/scim/tokens/{id}", s.handleConsoleSCIMTokenRevoke)

	mux.HandleFunc("GET /scim/v2/ServiceProviderConfig", s.requireSCIM(s.handleSCIMServiceProviderConfig))
	mux.HandleFunc("GET /scim/v2/ResourceTypes", s.requireSCIM(s.handleSCIMResourceTypes))
	mux.HandleFunc("GET /scim/v2/Users", s.requireSCIM(s.handleSCIMListUsers))
	mux.HandleFunc("POST /scim/v2/Users", s.requireSCIM(s.handleSCIMCreateUser))
	mux.HandleFunc("GET /scim/v2/Users/{id}", s.requireSCIM(s.handleSCIMGetUser))
	mux.HandleFunc("PUT /scim/v2/Users/{id}", s.requireSCIM(s.handleSCIMReplaceUser))
	mux.HandleFunc("PATCH /scim/v2/Users/{id}", s.requireSCIM(s.handleSCIMPatchUser))
	mux.HandleFunc("DELETE /scim/v2/Users/{id}", s.requireSCIM(s.handleSCIMDeleteUser))
	mux.HandleFunc("POST /api/app/settings/sla", s.handleConsoleSLASave)
	mux.HandleFunc("PATCH /api/app/settings/members/{id}", s.handleConsoleMemberRole)
	mux.HandleFunc("DELETE /api/app/settings/members/{id}", s.handleConsoleMemberRemove)
	mux.HandleFunc("POST /api/app/settings/members/{id}/revoke-sessions", s.handleConsoleMemberRevokeSessions)
	mux.HandleFunc("POST /api/app/settings/account", s.handleConsoleAccountSave)
	mux.HandleFunc("POST /api/app/account/two-factor/setup", s.handleConsoleTwoFactorSetup)
	mux.HandleFunc("POST /api/app/account/two-factor/enable", s.handleConsoleTwoFactorEnable)
	mux.HandleFunc("POST /api/app/account/two-factor/disable", s.handleConsoleTwoFactorDisable)
	mux.HandleFunc("POST /api/app/settings/invitations", s.handleConsoleInviteCreate)
	mux.HandleFunc("DELETE /api/app/settings/invitations/{id}", s.handleConsoleInviteRevoke)
	mux.HandleFunc("GET /api/app/account", s.handleConsoleAccount)
	mux.HandleFunc("POST /api/app/account", s.handleConsoleAccountSave)
	mux.HandleFunc("GET /api/app/invitations/{id}", s.handleConsoleInvitation)
	mux.HandleFunc("POST /api/app/invitations/{id}/accept", s.handleConsoleInvitationAccept)
	mux.HandleFunc("GET /static/", s.handleStatic)
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/favicon-32.png", http.StatusSeeOther)
	})
	mux.HandleFunc("POST /setup", s.handleSetupSubmit)
	mux.HandleFunc("POST /login", s.handleLoginSubmit)
	mux.HandleFunc("POST /logout", s.handleLogout)
	mux.HandleFunc("POST /onboarding/provider", s.handleOnboardingProvider)
	mux.HandleFunc("POST /onboarding/add", s.handleOnboardingAdd)
	mux.HandleFunc("POST /onboarding/finish", s.handleOnboardingFinish)
	mux.HandleFunc("POST /dashboard/knowledge/add", s.handleKnowledgeAdd)
	mux.HandleFunc("POST /dashboard/knowledge/scope", s.handleKnowledgeScope)
	mux.HandleFunc("POST /dashboard/knowledge/delete", s.handleKnowledgeDelete)
	mux.HandleFunc("POST /dashboard/knowledge/reindex", s.handleKnowledgeReindex)
	mux.HandleFunc("POST /dashboard/settings/providers", s.handleSettingsProviderSave)
	mux.HandleFunc("POST /dashboard/settings/providers/test", s.handleSettingsProviderTest)
	mux.HandleFunc("POST /dashboard/settings/providers/remove", s.handleSettingsProviderRemove)
	mux.HandleFunc("POST /dashboard/settings/widget", s.handleSettingsWidgetSave)
	mux.HandleFunc("POST /dashboard/settings/agent", s.handleSettingsAgentSave)
	mux.HandleFunc("POST /dashboard/settings/api-keys", s.handleSettingsAPIKeyCreate)
	mux.HandleFunc("POST /dashboard/settings/api-keys/revoke", s.handleSettingsAPIKeyRevoke)
	mux.HandleFunc("POST /dashboard/settings/workspace", s.handleSettingsWorkspaceSave)
	mux.HandleFunc("POST /dashboard/settings/account", s.handleSettingsAccountSave)
	mux.HandleFunc("POST /dashboard/settings/invite", s.handleSettingsInviteCreate)
	mux.HandleFunc("POST /dashboard/settings/invite/revoke", s.handleSettingsInviteRevoke)
	mux.HandleFunc("POST /invite/{id}", s.handleInviteSubmit)
	mux.HandleFunc("POST /dashboard/inbox/action", s.handleInboxActionForm)
	mux.HandleFunc("POST /dashboard/inbox/{id}/reply", s.handleInboxReplyForm)
	mux.HandleFunc("POST /dashboard/inbox/{id}/resolve", s.handleInboxResolveForm)
	mux.HandleFunc("POST /dashboard/inbox/{id}/assign", s.handleInboxAssignForm)
	mux.HandleFunc("POST /dashboard/inbox/{id}/copilot", s.handleInboxCopilotForm)
	mux.HandleFunc("POST /dashboard/inbox/{id}/copilot/send", s.handleInboxCopilotSendForm)
	mux.HandleFunc("POST /dashboard/analytics/rollup", s.handleAnalyticsRollupPage)
	mux.HandleFunc("GET /login", s.handleConsolePage)
	mux.HandleFunc("GET /setup", s.handleConsolePage)
	mux.HandleFunc("GET /onboarding", s.handleConsolePage)
	mux.HandleFunc("GET /invite/{id}", s.handleConsolePage)
	mux.HandleFunc("GET /dashboard", s.handleConsolePage)
	mux.HandleFunc("GET /dashboard/{rest...}", s.handleConsolePage)
	mux.Handle("GET /legacy/", http.StripPrefix("/legacy", s.legacyPages()))
	mux.HandleFunc("GET /api/", s.handleNotFound)
	mux.HandleFunc("/", s.handleNotFound)
	var handler http.Handler = mux
	handler = s.withWidgetCORS(handler)
	handler = s.withOriginCheck(handler)
	handler = s.withSecurityHeaders(handler)
	handler = (&observability.Middleware{Metrics: s.metrics, Notifier: s.notifier, Logger: s.logger}).Wrap(handler)
	return handler
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if s.cfg.MetricsToken == "" && s.cfg.AppEnv == "production" {
		s.handleNotFound(w, r)
		return
	}
	if s.cfg.MetricsToken != "" {
		provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.MetricsToken)) != 1 {
			writeErrorBody(w, http.StatusUnauthorized, "unauthorized", "metrics token required")
			return
		}
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, s.metrics.Render(r.Context(), s.pool))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.pool.Ping(ctx); err != nil {
		writeErrorBody(w, http.StatusServiceUnavailable, "not_ready", "database unavailable")
		return
	}
	writeData(w, http.StatusOK, map[string]any{"ok": true, "database": "ok"})
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.Header.Get("Accept"), "application/json") {
		writeErrorBody(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	s.renderPageError(w, r, http.StatusNotFound, "Off the desk", "That page does not exist. It may have moved, or the link may be old. The inbox is still where you left it.")
}

func (s *Server) withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("X-Frame-Options", "DENY")
		headers.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		headers.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		headers.Set("Cross-Origin-Opener-Policy", "same-origin")
		if strings.HasPrefix(s.cfg.AppURL, "https://") {
			headers.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		headers.Set("Content-Security-Policy",
			"default-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'; "+
				"img-src 'self' data:; font-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"script-src 'self'; connect-src 'self'; object-src 'none'; manifest-src 'self'")
		path := r.URL.Path
		if strings.HasPrefix(path, "/dashboard") || strings.HasPrefix(path, "/api/app") || path == "/onboarding" || path == "/login" || path == "/setup" {
			headers.Set("Cache-Control", "no-store, max-age=0")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withOriginCheck(next http.Handler) http.Handler {
	appOrigin := ""
	if parsed, err := url.Parse(s.cfg.AppURL); err == nil {
		appOrigin = parsed.Scheme + "://" + parsed.Host
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/widget/") || strings.HasPrefix(r.URL.Path, "/api/v1/") {
			next.ServeHTTP(w, r)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != appOrigin {
			writeErrorBody(w, http.StatusForbidden, "forbidden", "cross-origin request refused")
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			writeErrorBody(w, http.StatusForbidden, "forbidden", "cross-site request refused")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if !strings.HasPrefix(path, "static/") {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(web.Static, path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case strings.HasSuffix(path, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(path, ".js"):
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case strings.HasSuffix(path, ".svg"):
		w.Header().Set("Content-Type", "image/svg+xml")
	case strings.HasSuffix(path, ".woff2"):
		w.Header().Set("Content-Type", "font/woff2")
	case strings.HasSuffix(path, ".png"):
		w.Header().Set("Content-Type", "image/png")
	case strings.HasSuffix(path, ".txt"):
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	if strings.Contains(path, "/fonts/") || strings.HasPrefix(path, "static/console/assets/") || strings.HasSuffix(path, "htmx.min.js") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	_, _ = w.Write(data)
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

func (s *Server) withWidgetCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/widget/") {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		allowed := false
		if origin != "" && s.repo != nil {
			token := r.URL.Query().Get("token")
			if token == "" {
				token = r.Header.Get("x-widget-token")
			}
			if payload, err := widget.VerifyAnyOrg(s.cfg.AppSecret, token); err == nil {
				if config, err := s.repo.GetWidgetConfig(db.WithOrg(r.Context(), payload.OrgID)); err == nil && config != nil {
					allowed = widget.OriginAllowed(origin, config.Origins)
				}
			}
		}
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Widget-Token")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
