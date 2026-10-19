// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/ratelimit"
)

type bootstrapBody struct {
	OrgName  string `json:"orgName"`
	UserName string `json:"userName"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody[bootstrapBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	body.OrgName = strings.TrimSpace(body.OrgName)
	body.UserName = strings.TrimSpace(body.UserName)
	switch {
	case body.OrgName == "" || len(body.OrgName) > 100:
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "orgName must be between 1 and 100 characters")
		return
	case body.UserName == "" || len(body.UserName) > 100:
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "userName must be between 1 and 100 characters")
		return
	case !validEmail(body.Email):
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "invalid email address")
		return
	case len(body.Password) < 8 || len(body.Password) > 200:
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "password must be between 8 and 200 characters")
		return
	}
	if _, err := ratelimit.Enforce(r.Context(), s.pool, "bootstrap", 20, 3600); err != nil {
		s.respondError(w, r, err)
		return
	}
	exists, err := s.repo.OrgExists(r.Context())
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if exists {
		writeErrorBody(w, http.StatusConflict, "conflict", "this installation already has an organization")
		return
	}
	passwordHash, err := auth.HashPassword(body.Password)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	orgID, userID, err := s.repo.Bootstrap(r.Context(), db.BootstrapInput{
		OrgName:      body.OrgName,
		UserName:     body.UserName,
		Email:        body.Email,
		PasswordHash: passwordHash,
	})
	if err != nil {
		if errors.Is(err, db.ErrOrgExists) || db.IsUniqueViolation(err) {
			writeErrorBody(w, http.StatusConflict, "conflict", "this installation already has an organization")
			return
		}
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"orgId": orgID, "userId": userID})
}
