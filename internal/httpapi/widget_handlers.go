// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/widget"
)

func (s *Server) handleWidgetGet(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsRead)
	if !ok {
		return
	}
	config, err := s.repo.GetWidgetConfig(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if config == nil {
		writeData(w, http.StatusOK, map[string]any{"origins": []string{}, "enabled": false, "theme": map[string]any{}})
		return
	}
	writeData(w, http.StatusOK, config)
}

type widgetPutBody struct {
	Origins []string       `json:"origins"`
	Theme   map[string]any `json:"theme"`
}

func (s *Server) handleWidgetPut(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	body, err := decodeBody[widgetPutBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if body.Origins == nil {
		body.Origins = []string{}
	}
	if len(body.Origins) > 20 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "at most 20 origins are allowed")
		return
	}
	for _, origin := range body.Origins {
		if origin == "" || len(origin) > 200 {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "each origin must be between 1 and 200 characters")
			return
		}
	}
	theme := json.RawMessage(`{}`)
	if body.Theme != nil {
		encoded, err := json.Marshal(body.Theme)
		if err != nil {
			s.respondError(w, r, err)
			return
		}
		theme = encoded
	}
	if err := s.repo.UpsertWidgetConfig(db.WithOrg(r.Context(), sess.OrgID), body.Origins, theme); err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType:  db.ActorUser,
		ActorID:    &sess.UserID,
		Type:       "settings.widget.updated",
		TargetType: "widget_config",
		Payload:    map[string]any{"origins": body.Origins},
	})
	config, err := s.repo.GetWidgetConfig(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if config == nil {
		writeData(w, http.StatusOK, map[string]any{"origins": body.Origins, "enabled": true, "theme": body.Theme})
		return
	}
	writeData(w, http.StatusOK, config)
}

func (s *Server) handleWidgetToken(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsRead)
	if !ok {
		return
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
	writeData(w, http.StatusOK, map[string]any{"token": token, "embedSnippet": snippet})
}
