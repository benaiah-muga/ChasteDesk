// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"chastedesk/internal/apperr"
	"chastedesk/internal/auth"
	"chastedesk/internal/db"
)

type consoleMemberRoleBody struct {
	Role string `json:"role"`
}

func validMemberRole(role string) bool {
	return role == "owner" || role == "admin" || role == "agent"
}

func (s *Server) memberTarget(w http.ResponseWriter, r *http.Request, sess *auth.Session) (string, string, bool) {
	userID := r.PathValue("id")
	if !uuidShape.MatchString(userID) {
		s.respondError(w, r, apperr.NotFound("member not found"))
		return "", "", false
	}
	if userID == sess.UserID {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "You cannot change your own membership here.")
		return "", "", false
	}
	role, err := s.repo.MemberRole(db.WithOrg(r.Context(), sess.OrgID), userID)
	if errors.Is(err, db.ErrNotFound) {
		s.respondError(w, r, apperr.NotFound("member not found"))
		return "", "", false
	}
	if err != nil {
		s.respondError(w, r, err)
		return "", "", false
	}
	return userID, role, true
}

func (s *Server) handleConsoleMemberRole(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapOrgAdmin)
	if !ok {
		return
	}
	userID, currentRole, ok := s.memberTarget(w, r, sess)
	if !ok {
		return
	}
	body, err := decodeBody[consoleMemberRoleBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	role := strings.ToLower(strings.TrimSpace(body.Role))
	if !validMemberRole(role) {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "role must be owner, admin, or agent")
		return
	}
	if currentRole == role {
		writeData(w, http.StatusOK, map[string]any{"updated": false, "role": role})
		return
	}
	ctx := db.WithOrg(r.Context(), sess.OrgID)
	if currentRole == "owner" && role != "owner" {
		owners, err := s.repo.CountOwners(ctx)
		if err != nil {
			s.respondError(w, r, err)
			return
		}
		if owners <= 1 {
			writeErrorBody(w, http.StatusConflict, "conflict", "The workspace must keep at least one owner.")
			return
		}
	}
	found, err := s.repo.SetMemberRole(ctx, userID, role)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !found {
		s.respondError(w, r, apperr.NotFound("member not found"))
		return
	}
	revoked, err := s.repo.DeleteSessionsForUser(ctx, userID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if currentRole == "owner" && role != "owner" {
		if err := s.repo.RevokeSCIMTokensForCreator(ctx, userID); err != nil {
			s.respondError(w, r, err)
			return
		}
	}
	if err := s.repo.SyncSCIMRole(ctx, userID, role); err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(ctx, db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "member.role_changed",
		TargetType: "member", TargetID: &userID, Payload: map[string]any{"from": currentRole, "to": role, "sessionsRevoked": revoked},
	})
	writeData(w, http.StatusOK, map[string]any{"updated": true, "role": role, "sessionsRevoked": revoked})
}

func (s *Server) handleConsoleMemberRemove(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapOrgAdmin)
	if !ok {
		return
	}
	userID, role, ok := s.memberTarget(w, r, sess)
	if !ok {
		return
	}
	ctx := db.WithOrg(r.Context(), sess.OrgID)
	if role == "owner" {
		owners, err := s.repo.CountOwners(ctx)
		if err != nil {
			s.respondError(w, r, err)
			return
		}
		if owners <= 1 {
			writeErrorBody(w, http.StatusConflict, "conflict", "The workspace must keep at least one owner.")
			return
		}
	}
	found, err := s.repo.RemoveMember(ctx, userID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !found {
		s.respondError(w, r, apperr.NotFound("member not found"))
		return
	}
	revoked, err := s.repo.DeleteSessionsForUser(ctx, userID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if err := s.repo.RevokeSCIMTokensForCreator(ctx, userID); err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(ctx, db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "member.removed",
		TargetType: "member", TargetID: &userID, Payload: map[string]any{"role": role, "sessionsRevoked": revoked},
	})
	writeData(w, http.StatusOK, map[string]any{"removed": true, "sessionsRevoked": revoked})
}

func (s *Server) handleConsoleMemberRevokeSessions(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapOrgAdmin)
	if !ok {
		return
	}
	userID, role, ok := s.memberTarget(w, r, sess)
	if !ok {
		return
	}
	ctx := db.WithOrg(r.Context(), sess.OrgID)
	revoked, err := s.repo.DeleteSessionsForUser(ctx, userID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(ctx, db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "member.sessions_revoked",
		TargetType: "member", TargetID: &userID, Payload: map[string]any{"role": role, "sessionsRevoked": revoked},
	})
	writeData(w, http.StatusOK, map[string]any{"revoked": revoked})
}
