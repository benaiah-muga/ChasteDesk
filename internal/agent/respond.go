// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/db"
	"chastedesk/internal/id"
	"chastedesk/internal/model"
	"chastedesk/internal/queue"
	"chastedesk/internal/retrieval"
)

type AiRespondData struct {
	OrgID          string `json:"orgId"`
	ConversationID string `json:"conversationId"`
}

type QueryEmbedder interface {
	Model() string
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}

func RespondHandler(pool *pgxpool.Pool, appSecret string, engine *retrieval.Engine, embedder QueryEmbedder) queue.Handler {
	return func(ctx context.Context, job queue.Job) error {
		var data AiRespondData
		if err := json.Unmarshal(job.Data, &data); err != nil {
			return err
		}
		return Respond(ctx, pool, appSecret, engine, embedder, data)
	}
}

func Respond(ctx context.Context, pool *pgxpool.Pool, appSecret string, engine *retrieval.Engine, embedder QueryEmbedder, data AiRespondData) error {
	var (
		customerID string
		channel    string
		status     string
	)
	err := pool.QueryRow(ctx,
		`select customer_id, channel::text, status::text from conversations where id = $1 and org_id = $2`,
		data.ConversationID, data.OrgID).Scan(&customerID, &channel, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("conversation %s not found", data.ConversationID)
	}
	if err != nil {
		return err
	}
	if status == "resolved" || status == "closed" {
		return nil
	}

	var (
		agentID    string
		policyJSON []byte
	)
	err = pool.QueryRow(ctx,
		`select id, policy from agents where org_id = $1 and is_active = true order by created_at limit 1`,
		data.OrgID).Scan(&agentID, &policyJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("no active agent configured")
	}
	if err != nil {
		return err
	}
	var policy AgentPolicy
	if err := json.Unmarshal(policyJSON, &policy); err != nil {
		return fmt.Errorf("agent policy: %w", err)
	}

	var (
		customerName *string
		customerMail *string
		attributes   []byte
	)
	if err := pool.QueryRow(ctx,
		`select name, email, attributes from customers where id = $1`, customerID,
	).Scan(&customerName, &customerMail, &attributes); err != nil {
		return fmt.Errorf("customer %s: %w", customerID, err)
	}
	var customerAttributes map[string]any
	if err := json.Unmarshal(attributes, &customerAttributes); err != nil {
		customerAttributes = map[string]any{}
	}
	if customerAttributes == nil {
		customerAttributes = map[string]any{}
	}

	history, customerMessage, err := loadHistory(ctx, pool, data.ConversationID)
	if err != nil {
		return err
	}
	if customerMessage == "" {
		return nil
	}

	recentConversations, err := loadRecentConversations(ctx, pool, data.OrgID, customerID, data.ConversationID)
	if err != nil {
		return err
	}

	var (
		queryEmbedding []float64
		embeddingModel string
	)
	if embedder != nil {
		vectors, embedErr := embedder.Embed(ctx, []string{customerMessage})
		if embedErr != nil {
			slog.Warn("query embedding failed; continuing with full-text retrieval", "org", data.OrgID, "conversation", data.ConversationID, "error", embedErr)
		} else if len(vectors) == 1 {
			queryEmbedding = vectors[0]
			embeddingModel = embedder.Model()
		}
	}

	tools := buildPolicyTools(pool, engine, policy)

	providerID := policy.Model.ProviderID
	if providerID == "" {
		err := pool.QueryRow(ctx,
			`select provider_id from provider_configs where org_id = $1 and enabled = true order by created_at limit 1`,
			data.OrgID).Scan(&providerID)
		if errors.Is(err, pgx.ErrNoRows) {
			providerID = ""
		} else if err != nil {
			return err
		}
	}
	if providerID == "" {
		return escalateWithoutProvider(ctx, pool, data.OrgID, data.ConversationID, agentID)
	}

	cfg, err := model.ResolveConfig(ctx, pool, appSecret, data.OrgID, model.ProviderID(providerID))
	if err != nil {
		return err
	}
	provider := model.For(model.ProviderID(providerID))

	var modelsCache []byte
	if err := pool.QueryRow(ctx,
		`select models_cache from provider_configs where org_id = $1 and provider_id = $2`,
		data.OrgID, providerID).Scan(&modelsCache); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	modelID := resolveModelID(policy, modelsCache)

	runID := id.UUID()
	rc := RunContext{
		OrgID: data.OrgID, ConversationID: data.ConversationID, AgentID: agentID,
		RunID: runID, CustomerID: customerID, Channel: channel,
	}
	deps := Deps{
		Chat: func(ctx context.Context, req model.ChatRequest) (model.ChatResult, error) {
			req.ModelID = modelID
			return provider.Chat(ctx, cfg, req)
		},
		Pool:      pool,
		Retrieval: engine,
		Tools:     tools,
		Notify: func(ctx context.Context, event Event) error {
			encoded, err := json.Marshal(event)
			if err != nil {
				return err
			}
			_, err = pool.Exec(ctx, "select pg_notify($1, $2)", "chastedesk", string(encoded))
			return err
		},
	}
	if _, err := RunTurn(ctx, rc, policy, TurnInput{
		CustomerMessage:     customerMessage,
		History:             history,
		Customer:            Customer{Name: customerName, Email: customerMail, Attributes: customerAttributes},
		RecentConversations: recentConversations,
		EmbeddingModel:      embeddingModel,
		QueryEmbedding:      queryEmbedding,
	}, deps); err != nil {
		return err
	}
	return nil
}

func loadHistory(ctx context.Context, pool *pgxpool.Pool, conversationID string) ([]HistoryMessage, string, error) {
	rows, err := pool.Query(ctx,
		`select role::text, content from messages where conversation_id = $1 and role <> 'internal_note' order by created_at asc limit 40`,
		conversationID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	type rawMessage struct{ role, content string }
	raw := make([]rawMessage, 0, 40)
	for rows.Next() {
		var message rawMessage
		if err := rows.Scan(&message.role, &message.content); err != nil {
			return nil, "", err
		}
		raw = append(raw, message)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	customerMessage := ""
	for i := len(raw) - 1; i >= 0; i-- {
		if raw[i].role == "customer" {
			customerMessage = truncate(raw[i].content, 4000)
			break
		}
	}
	start := 0
	if len(raw) > 10 {
		start = len(raw) - 10
	}
	history := make([]HistoryMessage, 0, len(raw)-start)
	for _, message := range raw[start:] {
		role := "agent_human"
		if message.role == "customer" {
			role = "customer"
		} else if message.role == "agent_ai" {
			role = "agent_ai"
		}
		history = append(history, HistoryMessage{Role: role, Content: truncate(message.content, 2000)})
	}
	return history, customerMessage, nil
}

func loadRecentConversations(ctx context.Context, pool *pgxpool.Pool, orgID, customerID, conversationID string) ([]string, error) {
	rows, err := pool.Query(ctx,
		`select to_char(created_at, 'YYYY-MM-DD') || ' - ' || coalesce(triage->>'summary', 'previous conversation') as line
		   from conversations where org_id = $1 and customer_id = $2 and id <> $3 order by created_at desc limit 5`,
		orgID, customerID, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := []string{}
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func buildPolicyTools(pool *pgxpool.Pool, engine *retrieval.Engine, policy AgentPolicy) []Tool {
	builtins := BuildBuiltinTools(pool, engine)
	tools := make([]Tool, 0, len(builtins))
	for _, tool := range builtins {
		mode := tool.Mode
		for _, configured := range policy.Tools {
			if configured.ToolID == tool.Declaration.ID {
				mode = ToolMode(configured.Mode)
				break
			}
		}
		if mode == ModeDisabled {
			continue
		}
		tool.Mode = mode
		tools = append(tools, tool)
	}
	return tools
}

func resolveModelID(policy AgentPolicy, modelsCache []byte) string {
	if policy.Model.ModelID != "" {
		return policy.Model.ModelID
	}
	var cached []struct {
		ID string `json:"id"`
	}
	if len(modelsCache) > 0 {
		if err := json.Unmarshal(modelsCache, &cached); err != nil {
			cached = nil
		}
	}
	if len(cached) > 0 && cached[0].ID != "" {
		return cached[0].ID
	}
	return "gpt-4o-mini"
}

func escalateWithoutProvider(ctx context.Context, pool *pgxpool.Pool, orgID, conversationID, agentID string) error {
	if _, err := pool.Exec(ctx,
		`insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'agent_ai', $3)`,
		orgID, conversationID, "Thanks for reaching out! Connecting you with a teammate who can help."); err != nil {
		return err
	}
	handoff, err := json.Marshal(map[string]any{
		"problem":             "No AI provider is configured yet. Routing to a teammate.",
		"recommendedNextStep": "Answer the customer. No AI provider is configured.",
		"customerContext":     map[string]any{},
		"investigated":        []string{},
		"evidence":            []string{},
		"actionsTaken":        []string{},
	})
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx,
		`update conversations set status = 'awaiting_human', handoff = $2::jsonb, updated_at = now() where id = $1`,
		conversationID, string(handoff)); err != nil {
		return err
	}
	target := conversationID
	actor := agentID
	if err := db.NewRepo(pool).WriteAuditNoOrg(ctx, &orgID, db.AuditInput{
		ActorType: db.ActorAgent, ActorID: &actor, Type: "run.escalated",
		TargetType: "conversation", TargetID: &target, Payload: map[string]any{"reason": "no_provider"},
	}); err != nil {
		return err
	}
	encoded, err := json.Marshal(Event{OrgID: orgID, ConversationID: conversationID, Type: "conversation.updated"})
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, "select pg_notify($1, $2)", "chastedesk", string(encoded))
	return err
}
