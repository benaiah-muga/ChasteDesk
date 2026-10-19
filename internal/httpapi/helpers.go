// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strings"

	"chastedesk/internal/apperr"
)

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func (s *Server) respondError(w http.ResponseWriter, r *http.Request, err error) {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		writeErrorBody(w, ae.HTTPStatus, ae.Code, ae.Message)
		return
	}
	s.logger.Error("unhandled route error", "error", err, "path", r.URL.Path)
	writeErrorBody(w, http.StatusInternalServerError, "internal", "internal error")
}

func decodeBody[T any](r *http.Request) (T, error) {
	var out T
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err := dec.Decode(&out); err != nil {
		return out, apperr.BadRequest("invalid request body")
	}
	return out, nil
}

func validEmail(email string) bool {
	return len(email) <= 254 && emailRe.MatchString(email)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}
