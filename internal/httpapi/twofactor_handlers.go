// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"chastedesk/internal/auth"
	"chastedesk/internal/crypto"
	"chastedesk/internal/db"
	"chastedesk/internal/ratelimit"
)

type twoFactorVerifyBody struct {
	ChallengeToken string `json:"challengeToken"`
	Code           string `json:"code"`
}

func (s *Server) handleTwoFactorVerify(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody[twoFactorVerifyBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !uuidShape.MatchString(body.ChallengeToken) || strings.TrimSpace(body.Code) == "" {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "A challenge and authentication code are required.")
		return
	}
	if _, err := ratelimit.Enforce(r.Context(), s.pool, "auth2fa:"+clientIP(r)+":"+body.ChallengeToken, 10, 300); err != nil {
		s.respondError(w, r, err)
		return
	}
	ctx := db.RepoCtx{Ctx: r.Context()}
	challenge, err := s.repo.LoginChallenge(ctx, body.ChallengeToken)
	if errors.Is(err, db.ErrNotFound) || (err == nil && (challenge.UsedAt != nil || challenge.ExpiresAt.Before(time.Now()) || challenge.Attempts >= 5)) {
		writeErrorBody(w, http.StatusUnauthorized, "unauthorized", "This sign-in challenge has expired. Start again.")
		return
	}
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	record, err := s.repo.TwoFactor(ctx, challenge.UserID)
	if errors.Is(err, db.ErrNotFound) || (err == nil && !record.Enabled) {
		writeErrorBody(w, http.StatusUnauthorized, "unauthorized", "Two-factor authentication is not active for this account.")
		return
	}
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	secret, err := crypto.OpenSecret(record.Sealed, s.cfg.AppSecret)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	code := strings.TrimSpace(body.Code)
	valid := auth.ValidateTOTP(secret, code, time.Now())
	if !valid && len(record.RecoveryHashes) > 0 {
		consumed, consumeErr := s.repo.ConsumeRecoveryCode(ctx, challenge.UserID, auth.HashRecoveryCode(code))
		if consumeErr != nil && !errors.Is(consumeErr, db.ErrNotFound) {
			s.respondError(w, r, consumeErr)
			return
		}
		valid = consumed
	}
	if !valid {
		_ = s.repo.IncrementLoginChallenge(ctx, body.ChallengeToken)
		s.auditAuthEvent(r, challenge.UserID, "auth.2fa_failed", "user")
		writeErrorBody(w, http.StatusUnauthorized, "unauthorized", "That authentication code is not valid.")
		return
	}
	used, err := s.repo.UseLoginChallenge(ctx, body.ChallengeToken)
	if err != nil || !used {
		writeErrorBody(w, http.StatusUnauthorized, "unauthorized", "This sign-in challenge has expired. Start again.")
		return
	}
	if err := s.startSession(w, r, challenge.UserID); err != nil {
		s.respondError(w, r, err)
		return
	}
	s.auditAuthEvent(r, challenge.UserID, "auth.login", "session")
	s.auditAuthEvent(r, challenge.UserID, "auth.2fa", "user")
	user, err := s.accountUser(r.Context(), challenge.UserID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"user": user})
}

type twoFactorCodeBody struct {
	Code string `json:"code"`
}

type twoFactorDisableBody struct {
	CurrentPassword string `json:"currentPassword"`
	Code            string `json:"code"`
}

func (s *Server) handleConsoleTwoFactorSetup(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	ctx := db.RepoCtx{Ctx: r.Context()}
	if record, err := s.repo.TwoFactor(ctx, sess.UserID); err == nil && record.Enabled {
		writeErrorBody(w, http.StatusConflict, "conflict", "Two-factor authentication is already enabled.")
		return
	}
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	sealed, err := crypto.SealSecret(secret, s.cfg.AppSecret)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if err := s.repo.BeginTwoFactor(ctx, sess.UserID, sealed); err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"secret": secret, "otpauthUri": auth.TOTPURI(sess.Email, secret),
	})
}

func (s *Server) handleConsoleTwoFactorEnable(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	body, err := decodeBody[twoFactorCodeBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	ctx := db.RepoCtx{Ctx: r.Context()}
	record, err := s.repo.TwoFactor(ctx, sess.UserID)
	if errors.Is(err, db.ErrNotFound) {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Start the authenticator setup first.")
		return
	}
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if record.Enabled {
		writeErrorBody(w, http.StatusConflict, "conflict", "Two-factor authentication is already enabled.")
		return
	}
	secret, err := crypto.OpenSecret(record.Sealed, s.cfg.AppSecret)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !auth.ValidateTOTP(secret, body.Code, time.Now()) {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "That authentication code is not valid. Check your authenticator and try again.")
		return
	}
	codes, err := auth.GenerateRecoveryCodes()
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	hashes := make([]string, 0, len(codes))
	for _, code := range codes {
		hashes = append(hashes, auth.HashRecoveryCode(code))
	}
	enabled, err := s.repo.EnableTwoFactor(ctx, sess.UserID, hashes)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !enabled {
		writeErrorBody(w, http.StatusConflict, "conflict", "Two-factor authentication could not be enabled. Start again.")
		return
	}
	if token := s.sessionToken(r); token != "" {
		_ = s.repo.DeleteOtherSessions(r.Context(), sess.UserID, tokenHash(token))
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "auth.2fa_enabled",
		TargetType: "user", TargetID: &sess.UserID, Payload: map[string]any{},
	})
	writeData(w, http.StatusOK, map[string]any{"enabled": true, "recoveryCodes": codes})
}

func (s *Server) handleConsoleTwoFactorDisable(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	body, err := decodeBody[twoFactorDisableBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if body.CurrentPassword == "" || strings.TrimSpace(body.Code) == "" {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Your password and an authentication code are required.")
		return
	}
	cred, err := s.repo.CredentialByEmail(r.Context(), sess.Email)
	if err != nil || !auth.VerifyPassword(body.CurrentPassword, cred.PasswordHash) {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Your current password did not match.")
		return
	}
	ctx := db.RepoCtx{Ctx: r.Context()}
	record, err := s.repo.TwoFactor(ctx, sess.UserID)
	if errors.Is(err, db.ErrNotFound) || (err == nil && !record.Enabled) {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Two-factor authentication is not enabled.")
		return
	}
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	secret, err := crypto.OpenSecret(record.Sealed, s.cfg.AppSecret)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	valid := auth.ValidateTOTP(secret, body.Code, time.Now())
	if !valid && len(record.RecoveryHashes) > 0 {
		consumed, consumeErr := s.repo.ConsumeRecoveryCode(ctx, sess.UserID, auth.HashRecoveryCode(body.Code))
		if consumeErr != nil && !errors.Is(consumeErr, db.ErrNotFound) {
			s.respondError(w, r, consumeErr)
			return
		}
		valid = consumed
	}
	if !valid {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "That authentication code is not valid.")
		return
	}
	disabled, err := s.repo.DisableTwoFactor(ctx, sess.UserID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !disabled {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Two-factor authentication is not enabled.")
		return
	}
	if token := s.sessionToken(r); token != "" {
		_ = s.repo.DeleteOtherSessions(r.Context(), sess.UserID, tokenHash(token))
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "auth.2fa_disabled",
		TargetType: "user", TargetID: &sess.UserID, Payload: map[string]any{},
	})
	writeData(w, http.StatusOK, map[string]any{"enabled": false})
}

func (s *Server) accountUser(ctx context.Context, userID string) (map[string]any, error) {
	var name, email string
	if err := s.pool.QueryRow(ctx, `select name, email from "user" where id = $1`, userID).Scan(&name, &email); err != nil {
		return nil, err
	}
	return s.sessionPayloadForUser(ctx, userID, name, email), nil
}
