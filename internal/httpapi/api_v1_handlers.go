// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/apperr"
	"chastedesk/internal/db"
	"chastedesk/internal/id"
	"chastedesk/internal/queue"
	"chastedesk/internal/ratelimit"
)

type apiKeyCtx struct {
	OrgID string
	KeyID string
}

func (s *Server) resolveAPIKey(w http.ResponseWriter, r *http.Request) (*apiKeyCtx, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer cd_") {
		writeErrorBody(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
		return nil, false
	}
	key := strings.TrimPrefix(header, "Bearer ")
	sum := sha256.Sum256([]byte(key))
	hash := hex.EncodeToString(sum[:])
	var ctx apiKeyCtx
	err := s.pool.QueryRow(r.Context(), `select id, org_id from api_keys where key_hash = $1`, hash).Scan(&ctx.KeyID, &ctx.OrgID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorBody(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
		return nil, false
	}
	if err != nil {
		s.respondError(w, r, err)
		return nil, false
	}
	_, _ = s.pool.Exec(r.Context(), `update api_keys set last_used_at = now() where id = $1`, ctx.KeyID)
	if _, err := ratelimit.Enforce(r.Context(), s.pool, "api:"+ctx.KeyID, 600, 60); err != nil {
		s.respondError(w, r, err)
		return nil, false
	}
	return &ctx, true
}

type apiCustomerInput struct {
	Email      string         `json:"email"`
	Name       string         `json:"name"`
	Attributes map[string]any `json:"attributes"`
}

type apiConversationCreateBody struct {
	Customer apiCustomerInput `json:"customer"`
	Message  string           `json:"message"`
}

func ensureAPICustomer(ctx context.Context, pool *pgxpool.Pool, orgID string, input apiCustomerInput) (string, error) {
	var existing string
	err := pool.QueryRow(ctx,
		`select id from customers where org_id = $1 and lower(email) = lower($2) limit 1`, orgID, input.Email).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	name := input.Name
	if name == "" {
		name = input.Email
	}
	attributes := "{}"
	if input.Attributes != nil {
		encoded, err := json.Marshal(input.Attributes)
		if err != nil {
			return "", err
		}
		attributes = string(encoded)
	}
	customerID := id.UUID()
	if _, err := pool.Exec(ctx,
		`insert into customers (id, org_id, email, name, attributes) values ($1, $2, $3, $4, $5::jsonb)`,
		customerID, orgID, input.Email, name, attributes); err != nil {
		return "", err
	}
	return customerID, nil
}

func (s *Server) handleAPIConversationCreate(w http.ResponseWriter, r *http.Request) {
	api, ok := s.resolveAPIKey(w, r)
	if !ok {
		return
	}
	body, err := decodeBody[apiConversationCreateBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !validEmail(body.Customer.Email) {
		s.respondError(w, r, apperr.BadRequest("customer email is required"))
		return
	}
	if strings.TrimSpace(body.Message) == "" || len(body.Message) > 4000 {
		s.respondError(w, r, apperr.BadRequest("message must be between 1 and 4000 characters"))
		return
	}
	ctx := r.Context()
	customerID, err := ensureAPICustomer(ctx, s.pool, api.OrgID, body.Customer)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	conversationID := id.UUID()
	if _, err := s.pool.Exec(ctx,
		`insert into conversations (id, org_id, customer_id, channel, status) values ($1, $2, $3, 'api', 'ai_active')`,
		conversationID, api.OrgID, customerID); err != nil {
		s.respondError(w, r, err)
		return
	}
	if _, err := s.pool.Exec(ctx,
		`insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'customer', $3)`,
		api.OrgID, conversationID, body.Message); err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(ctx, api.OrgID), db.AuditInput{
		ActorType:  db.ActorSystem,
		Type:       "conversation.created",
		TargetType: "conversation",
		TargetID:   &conversationID,
		Payload:    map[string]any{"channel": "api"},
	})
	if _, err := s.queue.Send(ctx, "ai-respond", map[string]any{
		"orgId": api.OrgID, "conversationId": conversationID,
	}, queue.SendOptions{SingletonKey: "conv:" + conversationID}); err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"conversationId": conversationID})
}

func (s *Server) handleAPIConversationList(w http.ResponseWriter, r *http.Request) {
	api, ok := s.resolveAPIKey(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(),
		`select id, status::text, created_at from conversations where org_id = $1 order by created_at desc limit 50`,
		api.OrgID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var (
			conversationID string
			status         string
			createdAt      any
		)
		if err := rows.Scan(&conversationID, &status, &createdAt); err != nil {
			s.respondError(w, r, err)
			return
		}
		items = append(items, map[string]any{"id": conversationID, "status": status, "createdAt": createdAt})
	}
	if err := rows.Err(); err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, items)
}

type apiConversationPostBody struct {
	ConversationID string `json:"conversationId"`
	Content        string `json:"content"`
}

func (s *Server) handleAPIConversationPost(w http.ResponseWriter, r *http.Request) {
	api, ok := s.resolveAPIKey(w, r)
	if !ok {
		return
	}
	body, err := decodeBody[apiConversationPostBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if body.ConversationID == "" || strings.TrimSpace(body.Content) == "" || len(body.Content) > 4000 {
		s.respondError(w, r, apperr.BadRequest("conversationId and content are required"))
		return
	}
	if !s.apiConversationOwned(w, r, api.OrgID, body.ConversationID) {
		return
	}
	s.insertAPIMessage(w, r, api.OrgID, body.ConversationID, body.Content)
}

func (s *Server) handleAPIMessageCreate(w http.ResponseWriter, r *http.Request) {
	api, ok := s.resolveAPIKey(w, r)
	if !ok {
		return
	}
	conversationID := r.PathValue("id")
	body, err := decodeBody[apiConversationPostBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if strings.TrimSpace(body.Content) == "" || len(body.Content) > 4000 {
		s.respondError(w, r, apperr.BadRequest("content must be between 1 and 4000 characters"))
		return
	}
	if !s.apiConversationOwned(w, r, api.OrgID, conversationID) {
		return
	}
	s.insertAPIMessage(w, r, api.OrgID, conversationID, body.Content)
}

func (s *Server) apiConversationOwned(w http.ResponseWriter, r *http.Request, orgID, conversationID string) bool {
	var one int
	err := s.pool.QueryRow(r.Context(),
		`select 1 from conversations where id = $1 and org_id = $2`, conversationID, orgID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorBody(w, http.StatusNotFound, "not_found", "conversation not found")
		return false
	}
	if err != nil {
		s.respondError(w, r, err)
		return false
	}
	return true
}

func (s *Server) insertAPIMessage(w http.ResponseWriter, r *http.Request, orgID, conversationID, content string) {
	ctx := r.Context()
	if _, err := s.pool.Exec(ctx,
		`insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'customer', $3)`,
		orgID, conversationID, content); err != nil {
		s.respondError(w, r, err)
		return
	}
	if _, err := s.queue.Send(ctx, "ai-respond", map[string]any{
		"orgId": orgID, "conversationId": conversationID,
	}, queue.SendOptions{SingletonKey: "conv:" + conversationID}); err != nil {
		s.respondError(w, r, err)
		return
	}
	event, err := json.Marshal(map[string]any{"orgId": orgID, "conversationId": conversationID, "type": "message.created"})
	if err == nil {
		_, _ = s.pool.Exec(ctx, "select pg_notify($1, $2)", "chastedesk", string(event))
	}
	writeData(w, http.StatusAccepted, map[string]any{"received": true})
}
