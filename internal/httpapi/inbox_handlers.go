// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"chastedesk/internal/apperr"
	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/retrieval"
)

func (s *Server) publishConversationEvent(ctx context.Context, orgID, conversationID, eventType string) {
	payload, err := json.Marshal(map[string]any{"orgId": orgID, "conversationId": conversationID, "type": eventType})
	if err != nil {
		return
	}
	_, _ = s.pool.Exec(ctx, "select pg_notify($1, $2)", "chastedesk", string(payload))
}

type inboxListRow struct {
	ID              string
	Status          string
	Channel         string
	CreatedAt       time.Time
	FirstResponseAt *time.Time
	Handoff         []byte
	CustomerName    *string
	CustomerEmail   *string
	UpdatedAt       time.Time
}

func (s *Server) queryInboxList(ctx context.Context, orgID, status, q string) ([]inboxListRow, error) {
	args := []any{orgID}
	filter := ""
	if status != "" {
		args = append(args, status)
		filter = fmt.Sprintf(" and c.status = $%d", len(args))
	}
	rows, err := s.pool.Query(ctx,
		`select c.id, c.status::text, c.channel::text, c.created_at, c.first_response_at, c.handoff, c.updated_at, cu.name, cu.email
		   from conversations c join customers cu on cu.id = c.customer_id
		  where c.org_id = $1`+filter+`
		  order by c.updated_at desc limit 100`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []inboxListRow{}
	for rows.Next() {
		var row inboxListRow
		if err := rows.Scan(&row.ID, &row.Status, &row.Channel, &row.CreatedAt, &row.FirstResponseAt, &row.Handoff, &row.UpdatedAt, &row.CustomerName, &row.CustomerEmail); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if q != "" {
		needle := strings.ToLower(q)
		filtered := make([]inboxListRow, 0, len(out))
		for _, row := range out {
			name := ""
			if row.CustomerName != nil {
				name = *row.CustomerName
			}
			email := ""
			if row.CustomerEmail != nil {
				email = *row.CustomerEmail
			}
			if strings.Contains(strings.ToLower(name), needle) || strings.Contains(strings.ToLower(email), needle) {
				filtered = append(filtered, row)
			}
		}
		out = filtered
	}
	return out, nil
}

func (s *Server) handleInboxConversations(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	rows, err := s.queryInboxList(r.Context(), sess.OrgID, r.URL.Query().Get("status"), r.URL.Query().Get("q"))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"id": row.ID, "status": row.Status, "channel": row.Channel,
			"createdAt": row.CreatedAt, "updatedAt": row.UpdatedAt,
		}
		if row.FirstResponseAt != nil {
			item["firstResponseAt"] = row.FirstResponseAt
		}
		if len(row.Handoff) > 0 && string(row.Handoff) != "null" {
			item["handoff"] = json.RawMessage(row.Handoff)
		}
		if row.CustomerName != nil {
			item["customerName"] = *row.CustomerName
		}
		if row.CustomerEmail != nil {
			item["customerEmail"] = *row.CustomerEmail
		}
		items = append(items, item)
	}
	writeData(w, http.StatusOK, items)
}

type inboxConversationRow struct {
	ID              string
	CustomerID      string
	Status          string
	Channel         string
	Handoff         []byte
	FirstResponseAt *time.Time
	ResolvedAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func loadInboxConversation(ctx context.Context, queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, orgID, conversationID string) (inboxConversationRow, error) {
	var row inboxConversationRow
	err := queryer.QueryRow(ctx,
		`select id, customer_id, status::text, channel::text, handoff, first_response_at, resolved_at, created_at, updated_at
		   from conversations where id = $1 and org_id = $2`, conversationID, orgID).
		Scan(&row.ID, &row.CustomerID, &row.Status, &row.Channel, &row.Handoff, &row.FirstResponseAt, &row.ResolvedAt, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, apperr.NotFound("conversation not found")
	}
	return row, err
}

type inboxMessageRow struct {
	ID        string
	Role      string
	Content   string
	Citations []byte
	CreatedAt time.Time
}

func loadInboxMessages(ctx context.Context, queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, conversationID string) ([]inboxMessageRow, error) {
	rows, err := queryer.Query(ctx,
		`select id, role::text, content, citations, created_at from messages where conversation_id = $1 order by created_at asc`,
		conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []inboxMessageRow{}
	for rows.Next() {
		var message inboxMessageRow
		if err := rows.Scan(&message.ID, &message.Role, &message.Content, &message.Citations, &message.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

type inboxActionRow struct {
	ID        string
	ToolID    string
	Args      []byte
	Status    string
	CreatedAt time.Time
}

func loadInboxActions(ctx context.Context, queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, orgID, conversationID string) ([]inboxActionRow, error) {
	rows, err := queryer.Query(ctx,
		`select id, tool_id, args, status::text, created_at from actions
		  where org_id = $1 and conversation_id = $2 order by created_at desc limit 20`, orgID, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	actions := []inboxActionRow{}
	for rows.Next() {
		var action inboxActionRow
		if err := rows.Scan(&action.ID, &action.ToolID, &action.Args, &action.Status, &action.CreatedAt); err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	return actions, rows.Err()
}

type inboxRecentRow struct {
	ID        string
	Status    string
	CreatedAt time.Time
}

func loadInboxRecent(ctx context.Context, queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, orgID, customerID, conversationID string) ([]inboxRecentRow, error) {
	rows, err := queryer.Query(ctx,
		`select id, status::text, created_at from conversations
		  where org_id = $1 and customer_id = $2 and id <> $3 order by created_at desc limit 10`,
		orgID, customerID, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	recent := []inboxRecentRow{}
	for rows.Next() {
		var row inboxRecentRow
		if err := rows.Scan(&row.ID, &row.Status, &row.CreatedAt); err != nil {
			return nil, err
		}
		recent = append(recent, row)
	}
	return recent, rows.Err()
}

func (s *Server) handleInboxConversation(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	ctx := r.Context()
	conversation, err := loadInboxConversation(ctx, s.pool, sess.OrgID, r.PathValue("id"))
	if err != nil {
		s.respondError(w, r, err)
		return
	}

	var (
		customerName   *string
		customerEmail  *string
		customerAttrs  []byte
		customerResult map[string]any
	)
	if err := s.pool.QueryRow(ctx, `select name, email, attributes from customers where id = $1`, conversation.CustomerID).
		Scan(&customerName, &customerEmail, &customerAttrs); err != nil {
		return
	}
	if customerName != nil || customerEmail != nil {
		customerResult = map[string]any{"id": conversation.CustomerID, "name": customerName, "email": customerEmail}
		var attrs map[string]any
		if err := json.Unmarshal(customerAttrs, &attrs); err != nil {
			attrs = map[string]any{}
		}
		customerResult["attributes"] = attrs
	}

	messages, err := loadInboxMessages(ctx, s.pool, conversation.ID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	messageItems := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		item := map[string]any{"id": message.ID, "role": message.Role, "content": message.Content, "createdAt": message.CreatedAt}
		if len(message.Citations) > 0 && string(message.Citations) != "null" {
			item["citations"] = json.RawMessage(message.Citations)
		}
		messageItems = append(messageItems, item)
	}

	recent, err := loadInboxRecent(ctx, s.pool, sess.OrgID, conversation.CustomerID, conversation.ID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	recentItems := make([]map[string]any, 0, len(recent))
	openCount := 0
	for _, row := range recent {
		recentItems = append(recentItems, map[string]any{"id": row.ID, "status": row.Status, "createdAt": row.CreatedAt})
		if row.Status != "resolved" && row.Status != "closed" {
			openCount++
		}
	}

	actions, err := loadInboxActions(ctx, s.pool, sess.OrgID, conversation.ID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	actionItems := make([]map[string]any, 0, len(actions))
	for _, action := range actions {
		actionItems = append(actionItems, map[string]any{
			"id": action.ID, "toolId": action.ToolID, "args": json.RawMessage(action.Args),
			"status": action.Status, "createdAt": action.CreatedAt,
		})
	}

	conversationItem := map[string]any{
		"id": conversation.ID, "customerId": conversation.CustomerID, "status": conversation.Status,
		"channel": conversation.Channel, "createdAt": conversation.CreatedAt, "updatedAt": conversation.UpdatedAt,
	}
	if len(conversation.Handoff) > 0 && string(conversation.Handoff) != "null" {
		conversationItem["handoff"] = json.RawMessage(conversation.Handoff)
	}
	if conversation.FirstResponseAt != nil {
		conversationItem["firstResponseAt"] = conversation.FirstResponseAt
	}
	if conversation.ResolvedAt != nil {
		conversationItem["resolvedAt"] = conversation.ResolvedAt
	}

	writeData(w, http.StatusOK, map[string]any{
		"conversation":        conversationItem,
		"customer":            customerResult,
		"messages":            messageItems,
		"recentConversations": recentItems,
		"openConversations":   openCount,
		"actions":             actionItems,
	})
}

type inboxReplyBody struct {
	Content  string `json:"content"`
	Internal bool   `json:"internal"`
}

func (s *Server) inboxReply(ctx context.Context, sess *auth.Session, conversationID, content string, internal bool) (string, error) {
	conversation, err := loadInboxConversation(ctx, s.pool, sess.OrgID, conversationID)
	if err != nil {
		return "", err
	}
	role := "agent_human"
	if internal {
		role = "internal_note"
	}
	if _, err := s.pool.Exec(ctx,
		`insert into messages (org_id, conversation_id, role, content) values ($1, $2, $3, $4)`,
		sess.OrgID, conversationID, role, content); err != nil {
		return "", err
	}
	if !internal {
		status := conversation.Status
		if status == "awaiting_human" || status == "ai_active" {
			status = "human_active"
		}
		if _, err := s.pool.Exec(ctx,
			`update conversations set status = $2, first_response_at = coalesce(first_response_at, now()), updated_at = now() where id = $1`,
			conversationID, status); err != nil {
			return "", err
		}
		s.publishConversationEvent(ctx, sess.OrgID, conversationID, "message.created")
	}
	if err := s.repo.WriteAudit(db.WithOrg(ctx, sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "message.created",
		TargetType: "conversation", TargetID: &conversationID,
		Payload: map[string]any{"role": role, "length": len(content)},
	}); err != nil {
		return "", err
	}
	return role, nil
}

func (s *Server) handleInboxReply(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	body, err := decodeBody[inboxReplyBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	content := body.Content
	if strings.TrimSpace(content) == "" || len(content) > 8000 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "content must be between 1 and 8000 characters")
		return
	}
	role, err := s.inboxReply(r.Context(), sess, r.PathValue("id"), content, body.Internal)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"sent": true, "role": role})
}

type inboxResolveBody struct {
	Action string `json:"action"`
}

func (s *Server) inboxResolve(ctx context.Context, sess *auth.Session, conversationID, action string) error {
	var (
		status     string
		resolvedAt *time.Time
	)
	if action == "resolve" {
		status = "resolved"
		now := time.Now()
		resolvedAt = &now
	} else {
		status = "human_active"
	}
	tag, err := s.pool.Exec(ctx,
		`update conversations set status = $2, resolved_at = $3, updated_at = now() where id = $1 and org_id = $4`,
		conversationID, status, resolvedAt, sess.OrgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("conversation not found")
	}
	return s.repo.WriteAudit(db.WithOrg(ctx, sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "conversation.updated",
		TargetType: "conversation", TargetID: &conversationID, Payload: map[string]any{"action": action},
	})
}

func (s *Server) handleInboxResolve(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	body, err := decodeBody[inboxResolveBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if body.Action != "resolve" && body.Action != "reopen" {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "action must be resolve or reopen")
		return
	}
	if err := s.inboxResolve(r.Context(), sess, r.PathValue("id"), body.Action); err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"done": true})
}

type inboxAssignBody struct {
	AssignedUserID *string `json:"assignedUserId"`
}

func (s *Server) handleInboxAssign(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	body, err := decodeBody[inboxAssignBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	assigned := sess.UserID
	if body.AssignedUserID != nil {
		assigned = *body.AssignedUserID
	}
	tag, err := s.pool.Exec(r.Context(),
		`update conversations set assigned_user_id = $2, updated_at = now() where id = $1 and org_id = $3`,
		r.PathValue("id"), assigned, sess.OrgID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		s.respondError(w, r, apperr.NotFound("conversation not found"))
		return
	}
	writeData(w, http.StatusOK, map[string]any{"assigned": true})
}

type copilotDraftBody struct {
	ConversationID string `json:"conversationId"`
}

func (s *Server) copilotDraft(ctx context.Context, orgID, conversationID string) (string, []map[string]any, bool, error) {
	if _, err := loadInboxConversation(ctx, s.pool, orgID, conversationID); err != nil {
		return "", nil, false, err
	}
	var query string
	err := s.pool.QueryRow(ctx,
		`select content from messages where conversation_id = $1 and role = 'customer' order by created_at asc limit 1`,
		conversationID).Scan(&query)
	if errors.Is(err, pgx.ErrNoRows) {
		query = "customer question"
	} else if err != nil {
		return "", nil, false, err
	}
	engine := retrieval.NewEngine(s.pool)
	evidence, err := engine.Search(ctx, orgID, retrieval.SearchQuery{
		Text: query, Scopes: []string{"public", "internal"}, Limit: 3,
	})
	if err != nil {
		return "", nil, false, err
	}
	lines := make([]string, 0, len(evidence))
	citations := make([]map[string]any, 0, len(evidence))
	for i, item := range evidence {
		lines = append(lines, fmt.Sprintf("[E%d] %s (source: %s v%d)", i+1, truncateText(item.Content, 280), item.SourceName, item.Version))
		citations = append(citations, map[string]any{
			"chunkId": item.ChunkID, "sourceName": item.SourceName, "version": item.Version,
		})
	}
	paragraphs := []string{"Hi,"}
	if len(lines) > 0 {
		paragraphs = append(paragraphs, strings.Join(lines, "\n\n"), "These points come from our help docs — let me know if that solves it.")
	} else {
		paragraphs = append(paragraphs, "I checked our knowledge base but could not find a confident answer — a teammate will follow up shortly.")
	}
	return strings.Join(paragraphs, "\n\n"), citations, len(lines) > 0, nil
}

func truncateText(text string, limit int) string {
	if len(text) > limit {
		return text[:limit]
	}
	return text
}

func (s *Server) handleCopilotDraft(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	body, err := decodeBody[copilotDraftBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !uuidShape.MatchString(body.ConversationID) {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "conversationId must be a uuid")
		return
	}
	draft, citations, grounded, err := s.copilotDraft(r.Context(), sess.OrgID, body.ConversationID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"draft": draft, "citations": citations, "grounded": grounded})
}

type copilotSendBody struct {
	ConversationID string `json:"conversationId"`
	Content        string `json:"content"`
}

func (s *Server) copilotSend(ctx context.Context, orgID, conversationID, content string) error {
	if _, err := loadInboxConversation(ctx, s.pool, orgID, conversationID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx,
		`insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'agent_human', $3)`,
		orgID, conversationID, content)
	return err
}

func (s *Server) handleCopilotSend(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	body, err := decodeBody[copilotSendBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if strings.TrimSpace(body.Content) == "" || len(body.Content) > 8000 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "content must be between 1 and 8000 characters")
		return
	}
	if err := s.copilotSend(r.Context(), sess.OrgID, body.ConversationID, body.Content); err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"sent": true})
}
