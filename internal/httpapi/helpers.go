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
	status := http.StatusInternalServerError
	code := "internal"
	message := "internal error"
	if errors.As(err, &ae) {
		status = ae.HTTPStatus
		code = ae.Code
		message = ae.Message
	} else {
		s.logger.Error("unhandled route error", "error", err, "path", r.URL.Path)
	}
	if isPagePath(r.URL.Path) {
		title := "Something went wrong"
		if status == http.StatusNotFound {
			title = "Off the desk"
		}
		s.renderPageError(w, r, status, title, message)
		return
	}
	writeErrorBody(w, status, code, message)
}

func isPagePath(path string) bool {
	return path == "/login" || path == "/setup" || path == "/onboarding" ||
		strings.HasPrefix(path, "/dashboard")
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
