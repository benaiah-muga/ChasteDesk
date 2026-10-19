// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/config"
	"chastedesk/internal/db"
)

type Server struct {
	cfg    config.Config
	logger *slog.Logger
	pool   *pgxpool.Pool
	repo   *db.Repo
}

func New(cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool) http.Handler {
	s := &Server{cfg: cfg, logger: logger, pool: pool}
	if pool != nil {
		s.repo = db.NewRepo(pool)
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
	mux.HandleFunc("GET /api/settings/providers", s.handleProvidersList)
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
