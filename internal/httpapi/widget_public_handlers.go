// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"chastedesk/internal/apperr"
	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/id"
	"chastedesk/internal/queue"
	"chastedesk/internal/ratelimit"
	"chastedesk/internal/widget"
)

type widgetAuth struct {
	OrgID  string
	Theme  json.RawMessage
	Origin string
}

func (s *Server) authenticateWidget(r *http.Request) (*widgetAuth, error) {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("x-widget-token")
	}
	payload, err := widget.VerifyAnyOrg(s.cfg.AppSecret, token)
	if err != nil {
		return nil, apperr.Unauthorized("invalid or expired widget token")
	}
	origin := r.Header.Get("Origin")
	if !widget.OriginAllowed(origin, payload.Origins) {
		return nil, apperr.Forbidden("origin not allowed for this widget")
	}
	config, err := s.repo.GetWidgetConfig(db.WithOrg(r.Context(), payload.OrgID))
	if err != nil {
		return nil, err
	}
	if config == nil || !config.Enabled {
		return nil, apperr.NotFound("widget disabled")
	}
	return &widgetAuth{OrgID: payload.OrgID, Theme: config.Theme, Origin: origin}, nil
}

func (s *Server) handleWidgetScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	_, _ = w.Write(widget.Bundle)
}

func themeDisplayName(theme json.RawMessage) string {
	var parsed struct {
		DisplayName string `json:"displayName"`
	}
	if len(theme) > 0 {
		_ = json.Unmarshal(theme, &parsed)
	}
	if parsed.DisplayName == "" {
		return "Support"
	}
	return parsed.DisplayName
}

func (s *Server) handleWidgetConfig(w http.ResponseWriter, r *http.Request) {
	auth, err := s.authenticateWidget(r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	theme := auth.Theme
	if len(theme) == 0 {
		theme = json.RawMessage(`{}`)
	}
	writeData(w, http.StatusOK, map[string]any{
		"displayName": themeDisplayName(theme),
		"theme":       json.RawMessage(theme),
	})
}

type widgetConversationBody struct {
	VisitorName  string `json:"visitorName"`
	VisitorEmail string `json:"visitorEmail"`
	FirstMessage string `json:"firstMessage"`
}

func (s *Server) handleWidgetConversationCreate(w http.ResponseWriter, r *http.Request) {
	auth, err := s.authenticateWidget(r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if _, err := ratelimit.Enforce(r.Context(), s.pool, "widget:new:"+auth.OrgID, 30, 3600); err != nil {
		s.respondError(w, r, err)
		return
	}
	body, err := decodeBody[widgetConversationBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	ctx := r.Context()
	customerID := id.UUID()
	name := "Website visitor"
	if strings.TrimSpace(body.VisitorName) != "" {
		name = strings.TrimSpace(body.VisitorName)
	}
	email := ""
	if validEmail(body.VisitorEmail) {
		email = body.VisitorEmail
	}
	if email != "" {
		var existing string
		err := s.pool.QueryRow(ctx,
			`select id from customers where org_id = $1 and lower(email) = lower($2) limit 1`, auth.OrgID, email).Scan(&existing)
		if err == nil {
			customerID = existing
		} else if !errors.Is(err, pgx.ErrNoRows) {
			s.respondError(w, r, err)
			return
		} else {
			if _, err := s.pool.Exec(ctx,
				`insert into customers (id, org_id, email, name, attributes) values ($1, $2, $3, $4, '{}'::jsonb)`,
				customerID, auth.OrgID, email, name); err != nil {
				s.respondError(w, r, err)
				return
			}
		}
	} else {
		if _, err := s.pool.Exec(ctx,
			`insert into customers (id, org_id, name, attributes) values ($1, $2, $3, '{}'::jsonb)`,
			customerID, auth.OrgID, name); err != nil {
			s.respondError(w, r, err)
			return
		}
	}
	conversationID := id.UUID()
	if _, err := s.pool.Exec(ctx,
		`insert into conversations (id, org_id, customer_id, channel, status) values ($1, $2, $3, 'widget', 'ai_active')`,
		conversationID, auth.OrgID, customerID); err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(ctx, auth.OrgID), db.AuditInput{
		ActorType:  db.ActorSystem,
		Type:       "conversation.created",
		TargetType: "conversation",
		TargetID:   &conversationID,
		Payload:    map[string]any{"channel": "widget"},
	})
	if first := strings.TrimSpace(body.FirstMessage); first != "" {
		if len(first) > 4000 {
			first = first[:4000]
		}
		if _, err := s.pool.Exec(ctx,
			`insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'customer', $3)`,
			auth.OrgID, conversationID, first); err != nil {
			s.respondError(w, r, err)
			return
		}
		if _, err := s.queue.Send(ctx, "ai-respond", map[string]any{
			"orgId": auth.OrgID, "conversationId": conversationID,
		}, queue.SendOptions{SingletonKey: "conv:" + conversationID}); err != nil {
			s.respondError(w, r, err)
			return
		}
	}
	token, err := widget.SignConversationToken(s.cfg.AppSecret, auth.OrgID, conversationID, time.Now())
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"conversationId": conversationID, "token": token})
}

type widgetMessageBody struct {
	Content string `json:"content"`
}

func (s *Server) handleWidgetMessageCreate(w http.ResponseWriter, r *http.Request) {
	auth, err := s.authenticateWidget(r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	ctx := r.Context()
	conversationID := r.PathValue("id")
	if !widget.VerifyConversationToken(s.cfg.AppSecret, auth.OrgID, conversationID, r.URL.Query().Get("convToken")) {
		s.respondError(w, r, apperr.Unauthorized("invalid conversation token"))
		return
	}
	var status string
	err = s.pool.QueryRow(ctx, `select status from conversations where id = $1 and org_id = $2`,
		conversationID, auth.OrgID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		s.respondError(w, r, apperr.NotFound("conversation not found"))
		return
	}
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if status == "resolved" || status == "closed" {
		s.respondError(w, r, apperr.Conflict("conversation is closed"))
		return
	}
	body, err := decodeBody[widgetMessageBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	content := strings.TrimSpace(body.Content)
	if content == "" {
		s.respondError(w, r, apperr.BadRequest("content is required"))
		return
	}
	if len(content) > 4000 {
		content = content[:4000]
	}
	if _, err := ratelimit.Enforce(ctx, s.pool, "widget:msg:"+conversationID, 20, 60); err != nil {
		s.respondError(w, r, err)
		return
	}
	forwarded := r.Header.Get("x-forwarded-for")
	if forwarded == "" {
		forwarded = "local"
	}
	if _, err := ratelimit.Enforce(ctx, s.pool, "widget:msg-ip:"+forwarded, 60, 3600); err != nil {
		s.respondError(w, r, err)
		return
	}
	if _, err := s.pool.Exec(ctx,
		`insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'customer', $3)`,
		auth.OrgID, conversationID, content); err != nil {
		s.respondError(w, r, err)
		return
	}
	if _, err := s.queue.Send(ctx, "ai-respond", map[string]any{
		"orgId": auth.OrgID, "conversationId": conversationID,
	}, queue.SendOptions{SingletonKey: "conv:" + conversationID, RetryLimit: 1, RetryDelaySeconds: 5}); err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusAccepted, map[string]any{"received": true})
}

type streamEvent struct {
	OrgID          string `json:"orgId"`
	ConversationID string `json:"conversationId"`
	Type           string `json:"type"`
}

type streamWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	closed  bool
}

func newStreamWriter(w http.ResponseWriter) *streamWriter {
	flusher, _ := w.(http.Flusher)
	return &streamWriter{w: w, flusher: flusher}
}

func (sw *streamWriter) send(event string, data any) {
	if sw.closed {
		return
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return
	}
	_, _ = sw.w.Write([]byte("event: " + event + "\ndata: " + string(encoded) + "\n\n"))
	if sw.flusher != nil {
		sw.flusher.Flush()
	}
}

func (sw *streamWriter) close() {
	sw.closed = true
}

func (s *Server) pushLatestAgentReply(ctx context.Context, sw *streamWriter, orgID, conversationID string) {
	var (
		messageID string
		role      string
		content   string
		citations []byte
	)
	err := s.pool.QueryRow(ctx,
		`select id, role::text, content, citations from messages
		  where conversation_id = $1 and org_id = $2 and role in ('agent_ai', 'agent_human')
		  order by created_at desc limit 1`, conversationID, orgID).Scan(&messageID, &role, &content, &citations)
	if err != nil {
		return
	}
	sw.send("message", map[string]any{
		"id": messageID, "role": role, "content": content, "citations": json.RawMessage(citations),
	})
}

func (s *Server) handleWidgetStream(w http.ResponseWriter, r *http.Request) {
	auth, err := s.authenticateWidget(r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	conversationID := r.PathValue("id")
	if !widget.VerifyConversationToken(s.cfg.AppSecret, auth.OrgID, conversationID, r.URL.Query().Get("convToken")) {
		s.respondError(w, r, apperr.Unauthorized("invalid conversation token"))
		return
	}
	var owner string
	if err := s.pool.QueryRow(r.Context(), `select org_id from conversations where id = $1`, conversationID).Scan(&owner); err != nil || owner != auth.OrgID {
		s.respondError(w, r, apperr.NotFound("conversation not found"))
		return
	}
	s.serveEventStream(w, r, func(ev streamEvent) bool {
		if ev.OrgID != auth.OrgID || ev.ConversationID != conversationID {
			return false
		}
		return ev.Type == "message.created" || ev.Type == "action.pending"
	}, func(ctx context.Context, sw *streamWriter) {
		s.pushLatestAgentReply(ctx, sw, auth.OrgID, conversationID)
	})
}

func (s *Server) handleConversationsStream(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	s.serveEventStream(w, r, func(ev streamEvent) bool {
		return ev.OrgID == sess.OrgID
	}, nil)
}

func (s *Server) serveEventStream(w http.ResponseWriter, r *http.Request, interesting func(streamEvent) bool, replay func(ctx context.Context, sw *streamWriter)) {
	conn, err := s.pool.Acquire(r.Context())
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	defer conn.Release()
	if _, err := conn.Exec(r.Context(), "listen chastedesk"); err != nil {
		s.respondError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	sw := newStreamWriter(w)
	defer sw.close()
	sw.send("ready", map[string]any{"ok": true})
	if replay != nil {
		replay(r.Context(), sw)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	events := make(chan streamEvent, 8)
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		for {
			notification, err := conn.Conn().WaitForNotification(ctx)
			if err != nil {
				close(events)
				return
			}
			var ev streamEvent
			if err := json.Unmarshal([]byte(notification.Payload), &ev); err != nil {
				continue
			}
			if !interesting(ev) {
				continue
			}
			select {
			case events <- ev:
			case <-ctx.Done():
				close(events)
				return
			}
		}
	}()
	defer func() {
		cancel()
		<-pumpDone
	}()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			sw.send("ping", map[string]any{"t": time.Now().UnixMilli()})
		case ev, open := <-events:
			if !open {
				return
			}
			if replay != nil {
				replay(ctx, sw)
			} else {
				sw.send("message", ev)
			}
		}
	}
}

func (s *Server) handleWidgetHistory(w http.ResponseWriter, r *http.Request) {
	auth, err := s.authenticateWidget(r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	conversationID := r.PathValue("id")
	if !widget.VerifyConversationToken(s.cfg.AppSecret, auth.OrgID, conversationID, r.URL.Query().Get("convToken")) {
		s.respondError(w, r, apperr.Unauthorized("invalid conversation token"))
		return
	}
	var status string
	if err := s.pool.QueryRow(r.Context(), `select status::text from conversations where id = $1 and org_id = $2`,
		conversationID, auth.OrgID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.respondError(w, r, apperr.NotFound("conversation not found"))
			return
		}
		s.respondError(w, r, err)
		return
	}
	rows, err := s.pool.Query(r.Context(),
		`select id::text, role::text, content, coalesce(citations, '[]'::jsonb), created_at
		   from messages where conversation_id = $1 and org_id = $2 and role <> 'internal_note'
		  order by created_at asc limit 200`, conversationID, auth.OrgID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	defer rows.Close()
	messages := []map[string]any{}
	for rows.Next() {
		var (
			messageID string
			role      string
			content   string
			citations []byte
			createdAt time.Time
		)
		if err := rows.Scan(&messageID, &role, &content, &citations, &createdAt); err != nil {
			s.respondError(w, r, err)
			return
		}
		messages = append(messages, map[string]any{
			"id": messageID, "role": role, "content": content,
			"citations": json.RawMessage(citations), "createdAt": createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"status": status, "messages": messages})
}
