// SPDX-License-Identifier: AGPL-3.0-only
package apperr

type Error struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *Error) Error() string {
	return e.Message
}

func New(code, message string, status int) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: status}
}

func BadRequest(message string) *Error   { return New("bad_request", message, 400) }
func Unauthorized(message string) *Error { return New("unauthorized", message, 401) }
func Forbidden(message string) *Error    { return New("forbidden", message, 403) }
func NotFound(message string) *Error     { return New("not_found", message, 404) }
func Conflict(message string) *Error     { return New("conflict", message, 409) }
func RateLimited(message string) *Error  { return New("rate_limited", message, 429) }
func Internal(message string) *Error     { return New("internal", message, 500) }
