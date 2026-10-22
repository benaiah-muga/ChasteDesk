// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/id"
	"chastedesk/internal/ratelimit"
)

type signUpBody struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type signInBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID string) error {
	token := id.Token(32)
	sessionID := id.UUID()
	ip := clientIP(r)
	userAgent := r.UserAgent()
	expiresAt := time.Now().Add(sessionTTL)
	if err := s.repo.CreateSession(r.Context(), s.pool, sessionID, tokenHash(token), userID, &ip, &userAgent, expiresAt); err != nil {
		return err
	}
	s.setSessionCookie(w, token)
	return nil
}

func (s *Server) auditAuthEvent(r *http.Request, userID, eventType, targetType string) {
	orgID, _, err := s.repo.MembershipForUser(r.Context(), userID)
	var orgPtr *string
	if err == nil {
		orgPtr = &orgID
	}
	uid := userID
	_ = s.repo.WriteAuditNoOrg(r.Context(), orgPtr, db.AuditInput{
		ActorType:  db.ActorUser,
		ActorID:    &uid,
		Type:       eventType,
		TargetType: targetType,
		TargetID:   &uid,
		Payload:    map[string]any{},
	})
}

func (s *Server) handleSignUp(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody[signUpBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 100 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "name must be between 1 and 100 characters")
		return
	}
	if !validEmail(body.Email) {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "invalid email address")
		return
	}
	if len(body.Password) < 8 || len(body.Password) > 200 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "password must be between 8 and 200 characters")
		return
	}
	passwordHash, err := auth.HashPassword(body.Password)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	userID := id.UUID()
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.repo.CreateUser(ctx, tx, db.User{ID: userID, Name: body.Name, Email: body.Email}); err != nil {
		if db.IsUniqueViolation(err) {
			writeErrorBody(w, http.StatusConflict, "conflict", "email already registered")
			return
		}
		s.respondError(w, r, err)
		return
	}
	if err := s.repo.CreateCredentialAccount(ctx, tx, id.UUID(), userID, passwordHash); err != nil {
		s.respondError(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		s.respondError(w, r, err)
		return
	}
	if err := s.startSession(w, r, userID); err != nil {
		s.respondError(w, r, err)
		return
	}
	s.auditAuthEvent(r, userID, "auth.login", "session")
	writeJSON(w, http.StatusCreated, map[string]any{"user": s.sessionPayloadForUser(r.Context(), userID, body.Name, body.Email)})
}

func (s *Server) handleSignIn(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody[signInBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !validEmail(body.Email) || body.Password == "" {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "email and password are required")
		return
	}
	if _, err := ratelimit.Enforce(r.Context(), s.pool, "auth:"+clientIP(r)+":"+strings.ToLower(body.Email), 10, 900); err != nil {
		s.respondError(w, r, err)
		return
	}
	cred, credErr := s.repo.CredentialByEmail(r.Context(), body.Email)
	if credErr != nil && !errors.Is(credErr, db.ErrNotFound) {
		s.respondError(w, r, credErr)
		return
	}
	ok := credErr == nil && auth.VerifyPassword(body.Password, cred.PasswordHash)
	if !ok {
		if credErr == nil {
			s.auditAuthEvent(r, cred.User.ID, "auth.failed", "user")
		}
		writeErrorBody(w, http.StatusUnauthorized, "unauthorized", "invalid email or password")
		return
	}
	twoFactor, twoFactorErr := s.repo.TwoFactor(db.RepoCtx{Ctx: r.Context()}, cred.User.ID)
	if twoFactorErr != nil && !errors.Is(twoFactorErr, db.ErrNotFound) {
		s.respondError(w, r, twoFactorErr)
		return
	}
	if twoFactorErr == nil && twoFactor.Enabled {
		challengeID, err := s.repo.CreateLoginChallenge(db.RepoCtx{Ctx: r.Context()}, cred.User.ID, 5*time.Minute)
		if err != nil {
			s.respondError(w, r, err)
			return
		}
		s.auditAuthEvent(r, cred.User.ID, "auth.2fa_challenge", "user")
		writeData(w, http.StatusOK, map[string]any{"twoFactorRequired": true, "challengeToken": challengeID})
		return
	}
	if err := s.startSession(w, r, cred.User.ID); err != nil {
		s.respondError(w, r, err)
		return
	}
	s.auditAuthEvent(r, cred.User.ID, "auth.login", "session")
	writeData(w, http.StatusOK, map[string]any{"user": s.sessionPayloadForUser(r.Context(), cred.User.ID, cred.User.Name, cred.User.Email)})
}

func (s *Server) handleSignOut(w http.ResponseWriter, r *http.Request) {
	if token := s.sessionToken(r); token != "" {
		_ = s.repo.DeleteSession(r.Context(), tokenHash(token))
	}
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessionFromRequest(r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if sess == nil {
		writeJSON(w, http.StatusOK, map[string]any{"user": nil, "session": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":    sessionUserPayload(sess),
		"session": map[string]any{"id": sess.SessionID},
	})
}

func sessionUserPayload(sess *auth.Session) map[string]any {
	out := map[string]any{"id": sess.UserID, "name": sess.Name, "email": sess.Email}
	if sess.OrgID != "" {
		out["orgId"] = sess.OrgID
		out["role"] = sess.Role
	}
	return out
}

func (s *Server) sessionPayloadForUser(ctx context.Context, userID, name, email string) map[string]any {
	out := map[string]any{"id": userID, "name": name, "email": email}
	if orgID, role, err := s.repo.MembershipForUser(ctx, userID); err == nil {
		out["orgId"] = orgID
		out["role"] = role
	}
	return out
}
