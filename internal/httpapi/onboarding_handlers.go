// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"net/http"

	"chastedesk/internal/db"
)

func (s *Server) handleOnboardingStatus(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessionFromRequest(r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if sess == nil || sess.OrgID == "" {
		writeData(w, http.StatusOK, map[string]any{
			"orgCreated":         false,
			"providerConfigured": false,
			"knowledgeAdded":     false,
			"testSent":           false,
			"complete":           false,
		})
		return
	}
	state, err := s.repo.OnboardingState(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"orgCreated":         true,
		"providerConfigured": state.ProviderConfigured,
		"knowledgeAdded":     state.KnowledgeAdded,
		"testSent":           state.TestSent,
		"complete":           state.Complete,
	})
}

type onboardingCompleteBody struct {
	Complete *bool `json:"complete"`
}

func (s *Server) handleOnboardingComplete(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessionFromRequest(r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if sess == nil || sess.OrgID == "" {
		writeErrorBody(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	body := onboardingCompleteBody{}
	if r.ContentLength != 0 {
		decoded, err := decodeBody[onboardingCompleteBody](r)
		if err != nil {
			s.respondError(w, r, err)
			return
		}
		body = decoded
	}
	complete := body.Complete == nil || *body.Complete
	if err := s.repo.SetOnboardingComplete(db.WithOrg(r.Context(), sess.OrgID), complete); err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"complete": complete})
}
