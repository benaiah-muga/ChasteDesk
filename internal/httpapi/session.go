// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"chastedesk/internal/auth"
	"chastedesk/internal/db"
)

const plainCookieName = "chastedesk.session_token"
const secureCookieName = "__Secure-chastedesk.session_token"
const sessionTTL = 7 * 24 * time.Hour

func (s *Server) secureCookies() bool {
	return strings.HasPrefix(s.cfg.AppURL, "https://")
}

func (s *Server) sessionCookieName() string {
	if s.secureCookies() {
		return secureCookieName
	}
	return plainCookieName
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.sessionCookieName(),
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.secureCookies(),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.sessionCookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.secureCookies(),
	})
}

func (s *Server) sessionToken(r *http.Request) string {
	for _, name := range []string{s.sessionCookieName(), plainCookieName, secureCookieName} {
		if c, err := r.Cookie(name); err == nil && c.Value != "" {
			return c.Value
		}
	}
	return ""
}

func (s *Server) sessionFromRequest(r *http.Request) (*auth.Session, error) {
	token := s.sessionToken(r)
	if token == "" {
		return nil, nil
	}
	info, err := s.repo.SessionByTokenHash(r.Context(), tokenHash(token))
	if errors.Is(err, db.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sess := &auth.Session{
		SessionID: info.SessionID,
		UserID:    info.UserID,
		Name:      info.Name,
		Email:     info.Email,
	}
	orgID, role, err := s.repo.MembershipForUser(r.Context(), info.UserID)
	if err == nil {
		sess.OrgID = orgID
		sess.Role = auth.Role(role)
	} else if !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}
	return sess, nil
}

func (s *Server) requireCapability(w http.ResponseWriter, r *http.Request, capability auth.Capability) (*auth.Session, bool) {
	sess, err := s.sessionFromRequest(r)
	if err != nil {
		s.respondError(w, r, err)
		return nil, false
	}
	if sess == nil {
		writeErrorBody(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return nil, false
	}
	if sess.Role == "" {
		writeErrorBody(w, http.StatusUnauthorized, "unauthorized", "no organization membership")
		return nil, false
	}
	if !auth.Can(sess.Role, capability) {
		writeErrorBody(w, http.StatusForbidden, "forbidden", fmt.Sprintf("role %s lacks %s", sess.Role, capability))
		return nil, false
	}
	return sess, true
}
