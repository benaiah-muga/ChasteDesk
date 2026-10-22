// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"io/fs"
	"net/http"

	"chastedesk/internal/web"
)

func (s *Server) handleConsolePage(w http.ResponseWriter, r *http.Request) {
	index, err := fs.ReadFile(web.Static, "static/console/index.html")
	if err != nil {
		http.Error(w, "console build not found", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	_, _ = w.Write(index)
}

func (s *Server) legacyPages() http.Handler {
	legacy := http.NewServeMux()
	legacy.HandleFunc("GET /setup", s.handleSetupPage)
	legacy.HandleFunc("GET /login", s.handleLoginPage)
	legacy.HandleFunc("GET /onboarding", s.handleOnboardingPage)
	legacy.HandleFunc("GET /invite/{id}", s.handleInvitePage)
	legacy.HandleFunc("GET /dashboard", s.handleOverviewPage)
	legacy.HandleFunc("GET /dashboard/knowledge", s.handleKnowledgePage)
	legacy.HandleFunc("GET /dashboard/knowledge/sources", s.handleKnowledgeSourcesPartial)
	legacy.HandleFunc("GET /dashboard/settings", s.handleSettingsPage)
	legacy.HandleFunc("GET /dashboard/inbox", s.handleInboxPage)
	legacy.HandleFunc("GET /dashboard/inbox/list", s.handleInboxListPartial)
	legacy.HandleFunc("GET /dashboard/inbox/{id}", s.handleInboxDetailPage)
	legacy.HandleFunc("GET /dashboard/inbox/{id}/live", s.handleInboxLivePartial)
	legacy.HandleFunc("GET /dashboard/analytics", s.handleAnalyticsPage)
	legacy.HandleFunc("GET /dashboard/audit", s.handleAuditPage)
	legacy.HandleFunc("GET /dashboard/audit/export", s.handleAuditExport)
	legacy.HandleFunc("GET /dashboard/customers", s.handleCustomersPage)
	legacy.HandleFunc("GET /dashboard/customers/{id}", s.handleCustomerDetailPage)
	legacy.HandleFunc("GET /dashboard/approvals", s.handleApprovalsPage)
	legacy.HandleFunc("GET /dashboard/account", s.handleAccountPage)
	return legacy
}
