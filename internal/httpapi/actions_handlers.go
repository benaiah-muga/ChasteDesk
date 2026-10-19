// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
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
	status, err := s.decideAction(r.Context(), sess.OrgID, sess.UserID, body.ActionID, body.Decision)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"status": status})
}

func (s *Server) decideAction(ctx context.Context, orgID, userID, actionID, decision string) (string, error) {
	ctxOrg := db.WithOrg(ctx, orgID)
	action, err := s.repo.GetAction(ctxOrg, actionID)
	if err != nil {
		return "", err
	}
	if action.Status != "pending_approval" {
		return "", apperr.Conflict("action is " + action.Status + ", not pending")
	}

	if decision == "reject" {
		if _, err := s.repo.DecideAction(ctxOrg, action.ID, "rejected_before_exec", userID); err != nil {
			return "", err
		}
		if err := s.repo.WriteAudit(ctxOrg, db.AuditInput{
			ActorType: db.ActorUser, ActorID: &userID, Type: "action.rejected",
			TargetType: "action", TargetID: &action.ID, Payload: map[string]any{},
		}); err != nil {
			return "", err
		}
		return "rejected_before_exec", nil
	}

	decided, err := s.repo.DecideAction(ctxOrg, action.ID, "approved", userID)
	if err != nil {
		return "", err
	}
	if !decided {
		return "", apperr.Conflict("action is no longer pending")
	}
	if err := s.repo.WriteAudit(ctxOrg, db.AuditInput{
		ActorType: db.ActorUser, ActorID: &userID, Type: "action.approved",
		TargetType: "action", TargetID: &action.ID, Payload: map[string]any{},
	}); err != nil {
		return "", err
	}
	if _, err := s.queue.Send(ctx, "execute-action", map[string]any{
		"orgId": orgID, "actionId": action.ID,
	}, queue.SendOptions{SingletonKey: "action:" + action.ID}); err != nil {
		return "", err
	}
	if _, err := s.repo.Pool().Exec(ctx,
		`insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'system', $3)`,
		orgID, action.ConversationID, "Action approved. Executing."); err != nil {
		return "", err
	}
	return "approved", nil
}
