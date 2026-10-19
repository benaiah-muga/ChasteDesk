// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"net/http"
	"regexp"

	"chastedesk/internal/apperr"
	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/queue"
)

var uuidShape = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (s *Server) handleActionsList(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	rows, err := s.repo.ListActions(db.WithOrg(r.Context(), sess.OrgID), r.URL.Query().Get("status"))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"actions": rows})
}

type actionDecideBody struct {
	ActionID string `json:"actionId"`
	Decision string `json:"decision"`
}

func (s *Server) handleActionDecide(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	body, err := decodeBody[actionDecideBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !uuidShape.MatchString(body.ActionID) {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "actionId must be a uuid")
		return
	}
	if body.Decision != "approve" && body.Decision != "reject" {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "decision must be approve or reject")
		return
	}
	ctx := db.WithOrg(r.Context(), sess.OrgID)
	action, err := s.repo.GetAction(ctx, body.ActionID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if action.Status != "pending_approval" {
		s.respondError(w, r, apperr.Conflict("action is "+action.Status+", not pending"))
		return
	}

	if body.Decision == "reject" {
		if _, err := s.repo.DecideAction(ctx, action.ID, "rejected_before_exec", sess.UserID); err != nil {
			s.respondError(w, r, err)
			return
		}
		if err := s.repo.WriteAudit(ctx, db.AuditInput{
			ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "action.rejected",
			TargetType: "action", TargetID: &action.ID, Payload: map[string]any{},
		}); err != nil {
			s.respondError(w, r, err)
			return
		}
		writeData(w, http.StatusOK, map[string]any{"status": "rejected_before_exec"})
		return
	}

	decided, err := s.repo.DecideAction(ctx, action.ID, "approved", sess.UserID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !decided {
		s.respondError(w, r, apperr.Conflict("action is no longer pending"))
		return
	}
	if err := s.repo.WriteAudit(ctx, db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "action.approved",
		TargetType: "action", TargetID: &action.ID, Payload: map[string]any{},
	}); err != nil {
		s.respondError(w, r, err)
		return
	}
	if _, err := s.queue.Send(r.Context(), "execute-action", map[string]any{
		"orgId": sess.OrgID, "actionId": action.ID,
	}, queue.SendOptions{SingletonKey: "action:" + action.ID}); err != nil {
		s.respondError(w, r, err)
		return
	}
	if _, err := s.repo.Pool().Exec(r.Context(),
		`insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'system', $3)`,
		sess.OrgID, action.ConversationID, "Action approved — executing."); err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"status": "approved"})
}
