// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"net/http"
	"strconv"

	"chastedesk/internal/auth"
	"chastedesk/internal/db"
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
	rows, err := s.repo.ListAuditEvents(db.WithOrg(r.Context(), sess.OrgID), r.URL.Query().Get("type"), limit)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"events": rows})
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
	writeData(w, http.StatusOK, map[string]any{"providers": rows})
}
