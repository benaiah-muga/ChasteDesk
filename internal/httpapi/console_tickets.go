// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"chastedesk/internal/apperr"
	"chastedesk/internal/auth"
	"chastedesk/internal/db"
)

var ticketPriorities = map[string]bool{"low": true, "normal": true, "high": true, "urgent": true}

func normalizeTicketTags(values []string) ([]string, error) {
	if len(values) > 8 {
		return nil, apperr.BadRequest("Use at most eight tags.")
	}
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range values {
		tag := strings.TrimSpace(strings.TrimPrefix(raw, "#"))
		if tag == "" {
			continue
		}
		if len(tag) > 24 {
			return nil, apperr.BadRequest("Each tag must be 24 characters or fewer.")
		}
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, tag)
	}
	return out, nil
}

func parseTicketSnooze(raw string) (*time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, apperr.BadRequest("Choose a valid snooze time.")
	}
	now := time.Now()
	if !parsed.After(now) {
		return nil, apperr.BadRequest("Snooze time must be in the future.")
	}
	if parsed.After(now.Add(366 * 24 * time.Hour)) {
		return nil, apperr.BadRequest("Snooze time must be within one year.")
	}
	return &parsed, nil
}

type consoleTicketUpdateBody struct {
	Priority     *string   `json:"priority"`
	Tags         *[]string `json:"tags"`
	SnoozedUntil *string   `json:"snoozedUntil"`
	ClearSnooze  bool      `json:"clearSnooze"`
}

func (s *Server) handleConsoleTicketUpdate(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	conversationID := r.PathValue("id")
	if !uuidShape.MatchString(conversationID) {
		s.respondError(w, r, apperr.NotFound("conversation not found"))
		return
	}
	body, err := decodeBody[consoleTicketUpdateBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if body.Priority == nil && body.Tags == nil && body.SnoozedUntil == nil && !body.ClearSnooze {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Send at least one ticket field.")
		return
	}
	update := db.TicketUpdate{}
	if body.Priority != nil {
		priority := strings.ToLower(strings.TrimSpace(*body.Priority))
		if !ticketPriorities[priority] {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "Priority must be low, normal, high, or urgent.")
			return
		}
		update.Priority = &priority
	}
	if body.Tags != nil {
		tags, err := normalizeTicketTags(*body.Tags)
		if err != nil {
			s.respondError(w, r, err)
			return
		}
		update.Tags = &tags
	}
	if body.ClearSnooze {
		update.ClearSnooze = true
	}
	if body.SnoozedUntil != nil {
		if body.ClearSnooze {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "Choose a snooze time or clear it, not both.")
			return
		}
		until, err := parseTicketSnooze(*body.SnoozedUntil)
		if err != nil {
			s.respondError(w, r, err)
			return
		}
		update.SnoozedUntil = until
		if until == nil {
			update.ClearSnooze = true
		}
	}
	fields, err := s.repo.UpdateTicketFields(db.WithOrg(r.Context(), sess.OrgID), conversationID, update)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	payload := map[string]any{}
	if body.Priority != nil {
		payload["priority"] = fields.Priority
	}
	if body.Tags != nil {
		payload["tags"] = fields.Tags
	}
	if body.SnoozedUntil != nil || body.ClearSnooze {
		payload["snoozedUntil"] = fields.SnoozedUntil
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "conversation.ticket_updated",
		TargetType: "conversation", TargetID: &conversationID, Payload: payload,
	})
	s.publishConversationEvent(r.Context(), sess.OrgID, conversationID, "ticket.updated")
	writeData(w, http.StatusOK, map[string]any{
		"ticketNumber": fields.Number, "priority": fields.Priority, "tags": fields.Tags, "snoozedUntil": fields.SnoozedUntil,
	})
}

type consoleCannedReplyBody struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func cannedReplyPayload(reply db.CannedReply) map[string]any {
	return map[string]any{
		"id": reply.ID, "title": reply.Title, "body": reply.Body,
		"createdAt": reply.CreatedAt, "updatedAt": reply.UpdatedAt,
	}
}

func validateCannedReply(body consoleCannedReplyBody) (string, string, error) {
	title := strings.TrimSpace(body.Title)
	text := strings.TrimSpace(body.Body)
	if title == "" || len(title) > 80 {
		return "", "", apperr.BadRequest("Quick reply name must be between 1 and 80 characters.")
	}
	if text == "" || len(text) > 4000 {
		return "", "", apperr.BadRequest("Quick reply body must be between 1 and 4000 characters.")
	}
	return title, text, nil
}

func (s *Server) handleConsoleCannedReplies(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	rows, err := s.repo.ListCannedReplies(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, reply := range rows {
		items = append(items, cannedReplyPayload(reply))
	}
	writeData(w, http.StatusOK, map[string]any{"replies": items})
}

func (s *Server) handleConsoleCannedReplyCreate(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	body, err := decodeBody[consoleCannedReplyBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	title, text, err := validateCannedReply(body)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	reply, err := s.repo.CreateCannedReply(db.WithOrg(r.Context(), sess.OrgID), title, text, sess.UserID)
	if err != nil {
		if db.IsUniqueViolation(err) {
			writeErrorBody(w, http.StatusConflict, "conflict", "A quick reply with that name already exists.")
			return
		}
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "canned_reply.created",
		TargetType: "canned_reply", TargetID: &reply.ID, Payload: map[string]any{"title": reply.Title},
	})
	writeData(w, http.StatusCreated, cannedReplyPayload(reply))
}

func (s *Server) handleConsoleCannedReplyUpdate(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	replyID := r.PathValue("id")
	if !uuidShape.MatchString(replyID) {
		s.respondError(w, r, apperr.NotFound("quick reply not found"))
		return
	}
	body, err := decodeBody[consoleCannedReplyBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	title, text, err := validateCannedReply(body)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	reply, err := s.repo.UpdateCannedReply(db.WithOrg(r.Context(), sess.OrgID), replyID, title, text)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			s.respondError(w, r, apperr.NotFound("quick reply not found"))
			return
		}
		if db.IsUniqueViolation(err) {
			writeErrorBody(w, http.StatusConflict, "conflict", "A quick reply with that name already exists.")
			return
		}
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "canned_reply.updated",
		TargetType: "canned_reply", TargetID: &reply.ID, Payload: map[string]any{"title": reply.Title},
	})
	writeData(w, http.StatusOK, cannedReplyPayload(reply))
}

func (s *Server) handleConsoleCannedReplyDelete(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	replyID := r.PathValue("id")
	if !uuidShape.MatchString(replyID) {
		s.respondError(w, r, apperr.NotFound("quick reply not found"))
		return
	}
	found, err := s.repo.DeleteCannedReply(db.WithOrg(r.Context(), sess.OrgID), replyID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !found {
		s.respondError(w, r, apperr.NotFound("quick reply not found"))
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "canned_reply.deleted",
		TargetType: "canned_reply", TargetID: &replyID, Payload: map[string]any{},
	})
	writeData(w, http.StatusOK, map[string]any{"deleted": true})
}
