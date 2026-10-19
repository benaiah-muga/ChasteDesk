// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"chastedesk/internal/db"
	"chastedesk/internal/model"
	"chastedesk/internal/retrieval"

	"github.com/jackc/pgx/v5/pgxpool"
)

const HoldingMessage = "I am checking with a teammate on this one. You will hear from us shortly."

type RunContext struct {
	OrgID          string
	ConversationID string
	AgentID        string
	RunID          string
	CustomerID     string
	Channel        string
}

type HistoryMessage struct {
	Role    string
	Content string
}

type Customer struct {
	Name       *string
	Email      *string
	Attributes map[string]any
}

type TurnInput struct {
	CustomerMessage     string
	History             []HistoryMessage
	Customer            Customer
	RecentConversations []string
}

type Event struct {
	OrgID          string `json:"orgId"`
	ConversationID string `json:"conversationId"`
	Type           string `json:"type"`
}

type Deps struct {
	Chat      func(ctx context.Context, req model.ChatRequest) (model.ChatResult, error)
	Pool      *pgxpool.Pool
	Retrieval *retrieval.Engine
	Tools     []Tool
	Notify    func(ctx context.Context, event Event) error
}

type Usage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	Steps        int `json:"steps"`
}

type Citation struct {
	Marker  string `json:"marker"`
	ChunkID string `json:"chunkId"`
}

type TurnResult struct {
	Outcome          string
	EscalationReason EscalationReason
	Handoff          *HandoffPackage
	Citations        []Citation
	Evidence         []retrieval.Evidence
	Steps            []map[string]any
	Usage            Usage
	Error            string
}

type turn struct {
	ctx      context.Context
	rc       RunContext
	policy   AgentPolicy
	input    TurnInput
	deps     Deps
	steps    []map[string]any
	usage    Usage
	evidence []retrieval.Evidence
}

func (t *turn) appendAudit(auditType, targetType, targetID string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	actorID := t.rc.AgentID
	id := targetID
	repo := db.NewRepo(t.deps.Pool)
	return repo.WriteAuditNoOrg(t.ctx, &t.rc.OrgID, db.AuditInput{
		ActorType:  db.ActorAgent,
		ActorID:    &actorID,
		Type:       auditType,
		TargetType: targetType,
		TargetID:   &id,
		Payload:    payload,
	})
}

func (t *turn) startRun() error {
	if _, err := t.deps.Pool.Exec(t.ctx,
		`insert into ai_runs (id, org_id, conversation_id, agent_id, status, steps, evidence, usage)
		 values ($1, $2, $3, $4, 'running', '[]'::jsonb, '[]'::jsonb, '{}'::jsonb)`,
		t.rc.RunID, t.rc.OrgID, t.rc.ConversationID, t.rc.AgentID); err != nil {
		return err
	}
	if err := t.appendAudit("run.started", "ai_run", t.rc.RunID, nil); err != nil {
		return err
	}
	actorID := t.rc.AgentID
	_ = db.NewRepo(t.deps.Pool).WriteAuditNoOrg(t.ctx, &t.rc.OrgID, db.AuditInput{
		ActorType: db.ActorAgent, ActorID: &actorID, Type: "conversation.created",
		TargetType: "conversation", TargetID: &t.rc.ConversationID, Payload: map[string]any{},
	})
	return nil
}

func (t *turn) finishRun(status string, outcome *string, errMsg *string) error {
	steps, err := json.Marshal(t.steps)
	if err != nil {
		return err
	}
	evidence, err := json.Marshal(t.evidence)
	if err != nil {
		return err
	}
	usage, err := json.Marshal(t.usage)
	if err != nil {
		return err
	}
	_, err = t.deps.Pool.Exec(t.ctx,
		`update ai_runs set status = $2, outcome = $3, steps = $4::jsonb, evidence = $5::jsonb, usage = $6::jsonb, error = $7, updated_at = now()
		 where id = $1`,
		t.rc.RunID, status, outcome, string(steps), string(evidence), string(usage), errMsg)
	return err
}

func (t *turn) publish(eventType string) error {
	if t.deps.Notify == nil {
		return nil
	}
	return t.deps.Notify(t.ctx, Event{OrgID: t.rc.OrgID, ConversationID: t.rc.ConversationID, Type: eventType})
}

func (t *turn) escalate(reason EscalationReason, recommendedNextStep, errMsg string) (TurnResult, error) {
	investigated := []string{}
	actionsTaken := []string{}
	for _, step := range t.steps {
		if step["kind"] != "tool" {
			continue
		}
		toolID, _ := step["toolId"].(string)
		investigated = append(investigated, toolID)
		if executed, _ := step["executed"].(bool); executed {
			actionsTaken = append(actionsTaken, toolID)
		}
	}
	handoff := BuildHandoffPackage(t.input, investigated, t.evidence, actionsTaken, recommendedNextStep)
	encoded, err := json.Marshal(handoff)
	if err != nil {
		return TurnResult{}, err
	}
	if _, err := t.deps.Pool.Exec(t.ctx,
		`update conversations set status = 'awaiting_human', handoff = $2::jsonb, updated_at = now() where id = $1 and org_id = $3`,
		t.rc.ConversationID, string(encoded), t.rc.OrgID); err != nil {
		return TurnResult{}, err
	}
	status := "escalated"
	stringReason := string(reason)
	if err := t.finishRun(status, &stringReason, nil); err != nil {
		return TurnResult{}, err
	}
	if err := t.appendAudit("run.escalated", "conversation", t.rc.ConversationID, map[string]any{
		"reason": reason, "recommendedNextStep": recommendedNextStep,
	}); err != nil {
		return TurnResult{}, err
	}
	if err := t.publish("conversation.updated"); err != nil {
		return TurnResult{}, err
	}
	return TurnResult{
		Outcome:          "escalated",
		EscalationReason: reason,
		Handoff:          &handoff,
		Citations:        []Citation{},
		Evidence:         t.evidence,
		Steps:            t.steps,
		Usage:            t.usage,
		Error:            errMsg,
	}, nil
}

func (t *turn) findTool(toolID string) *Tool {
	for i := range t.deps.Tools {
		if t.deps.Tools[i].Declaration.ID == toolID {
			return &t.deps.Tools[i]
		}
	}
	return nil
}

type toolCallEnvelope struct {
	ID     string `json:"id"`
	ToolID string `json:"toolId"`
	Args   any    `json:"args"`
}

func (t *turn) requestApproval(call model.ToolCall) (TurnResult, error) {
	args, err := json.Marshal(call.Args)
	if err != nil {
		return TurnResult{}, err
	}
	runID, conversationID, agentID := t.rc.RunID, t.rc.ConversationID, t.rc.AgentID
	if _, err := t.deps.Pool.Exec(t.ctx,
		`insert into actions (org_id, run_id, conversation_id, agent_id, tool_id, args, status)
		 values ($1, $2, $3, $4, $5, $6::jsonb, 'pending_approval')`,
		t.rc.OrgID, runID, conversationID, agentID, call.ToolID, string(args)); err != nil {
		return TurnResult{}, err
	}
	t.steps = append(t.steps, map[string]any{"kind": "tool_pending_approval", "toolId": call.ToolID})
	if err := t.appendAudit("action.requested", "action", call.ToolID, map[string]any{"args": call.Args}); err != nil {
		return TurnResult{}, err
	}
	if _, err := t.deps.Pool.Exec(t.ctx,
		`insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'agent_ai', $3)`,
		t.rc.OrgID, t.rc.ConversationID, HoldingMessage); err != nil {
		return TurnResult{}, err
	}
	status := "awaiting_approval"
	outcome := "awaiting_approval"
	if err := t.finishRun(status, &outcome, nil); err != nil {
		return TurnResult{}, err
	}
	if err := t.publish("action.pending"); err != nil {
		return TurnResult{}, err
	}
	return TurnResult{
		Outcome:   "awaiting_approval",
		Citations: []Citation{},
		Evidence:  t.evidence,
		Steps:     t.steps,
		Usage:     t.usage,
	}, nil
}

func RunTurn(ctx context.Context, rc RunContext, policy AgentPolicy, input TurnInput, deps Deps) (TurnResult, error) {
	t := &turn{ctx: ctx, rc: rc, policy: policy, input: input, deps: deps, steps: []map[string]any{}}

	if err := t.startRun(); err != nil {
		return TurnResult{}, err
	}

	preflight := EscalationCheck(input.CustomerMessage, policy, nil)
	if preflight == ReasonPolicyKeyword || preflight == ReasonExplicitRequest {
		return t.escalate(preflight, "A human should review this request directly.", "")
	}

	if deps.Retrieval != nil {
		evidence, err := deps.Retrieval.Search(ctx, rc.OrgID, retrieval.SearchQuery{
			Text: input.CustomerMessage, Scopes: policy.KnowledgeScopes, Limit: 8,
		})
		if err != nil {
			return TurnResult{}, err
		}
		t.evidence = evidence
	}
	best := BestEvidenceConfidence(t.evidence)
	retrievalStep := map[string]any{"kind": "retrieval", "evidence": len(t.evidence), "bestConfidence": nil}
	if best != nil {
		retrievalStep["bestConfidence"] = *best
	}
	t.steps = append(t.steps, retrievalStep)

	lowConfidence := best != nil && *best < policy.Escalation.LowConfidenceThreshold
	noEvidence := deps.Retrieval != nil && len(t.evidence) == 0
	if lowConfidence || noEvidence {
		return t.escalate(ReasonLowConfidence, "The knowledge base does not confidently cover this question; a human should answer it.", "")
	}

	messages := []model.Message{{Role: "system", Content: BuildSystemPrompt(policy, t.evidence)}}
	for _, history := range input.History {
		role := "assistant"
		if history.Role == "customer" {
			role = "user"
		}
		messages = append(messages, model.Message{Role: role, Content: history.Content})
	}
	messages = append(messages, model.Message{Role: "user", Content: input.CustomerMessage})

	declarations := make([]model.ToolDeclaration, 0, len(deps.Tools))
	for _, tool := range deps.Tools {
		declarations = append(declarations, tool.Declaration)
	}

	var answerText *string
	toolCallCount := 0

	for step := 1; step <= policy.Budgets.MaxSteps; step++ {
		if t.usage.InputTokens >= policy.Budgets.MaxInputTokens || t.usage.OutputTokens >= policy.Budgets.MaxOutputTokens {
			return t.escalate(ReasonBudgetExhausted, "The AI ran out of its processing budget; a human should take over.", "")
		}
		result, err := deps.Chat(ctx, model.ChatRequest{
			ModelID: "agent", Messages: messages, Tools: declarations, MaxOutputTokens: policy.Budgets.MaxOutputTokens,
		})
		if err != nil {
			return t.escalate(ReasonToolFailure, "The AI provider failed; a human should handle this conversation.", err.Error())
		}
		t.usage.InputTokens += result.Usage.InputTokens
		t.usage.OutputTokens += result.Usage.OutputTokens
		t.usage.Steps++
		modelStep := map[string]any{"kind": "model", "finishReason": string(result.FinishReason), "text": nil, "toolCalls": []string{}}
		if result.Text != nil {
			modelStep["text"] = truncate(*result.Text, 300)
		}
		calledTools := make([]string, 0, len(result.ToolCalls))
		for _, call := range result.ToolCalls {
			calledTools = append(calledTools, call.ToolID)
		}
		modelStep["toolCalls"] = calledTools
		t.steps = append(t.steps, modelStep)

		if result.FinishReason == model.FinishToolCalls && len(result.ToolCalls) > 0 {
			envelopes := make([]toolCallEnvelope, 0, len(result.ToolCalls))
			for _, call := range result.ToolCalls {
				envelopes = append(envelopes, toolCallEnvelope{ID: call.ID, ToolID: call.ToolID, Args: call.Args})
			}
			encodedCalls, err := json.Marshal(envelopes)
			if err != nil {
				return TurnResult{}, err
			}
			messages = append(messages, model.Message{Role: "assistant", Content: string(encodedCalls)})
			for _, call := range result.ToolCalls {
				toolCallCount++
				if toolCallCount > policy.Budgets.MaxToolCalls {
					return t.escalate(ReasonBudgetExhausted, "Too many tool attempts; a human should take over.", "")
				}
				tool := t.findTool(call.ToolID)
				if tool == nil {
					t.steps = append(t.steps, map[string]any{"kind": "tool_refused", "toolId": call.ToolID, "reason": "not_configured"})
					if err := t.appendAudit("run.step", "tool", call.ToolID, map[string]any{"refused": "not_configured"}); err != nil {
						return TurnResult{}, err
					}
					messages = append(messages, model.Message{
						Role: "user", Content: fmt.Sprintf("Tool %s is not available. Continue without it or escalate.", call.ToolID),
					})
					continue
				}
				if tool.Mode == ModeDisabled {
					t.steps = append(t.steps, map[string]any{"kind": "tool_denied", "toolId": call.ToolID})
					if err := t.appendAudit("run.step", "tool", call.ToolID, map[string]any{"denied": true}); err != nil {
						return TurnResult{}, err
					}
					return t.escalate(ReasonDeniedTool, "The requested action is not permitted for the AI; a human should handle it.", "")
				}
				if tool.Mode == ModeApprovalRequired {
					return t.requestApproval(call)
				}
				outcome, execErr := tool.Execute(ToolCtx{RunContext: t.rc}, call.Args)
				if execErr != nil {
					outcome = ToolResult{OK: false, Error: execErr.Error()}
				}
				t.steps = append(t.steps, map[string]any{"kind": "tool", "toolId": call.ToolID, "executed": true, "ok": outcome.OK})
				if err := t.appendAudit("run.step", "tool", call.ToolID, map[string]any{"ok": outcome.OK}); err != nil {
					return TurnResult{}, err
				}
				var content any = outcome.Data
				if content == nil {
					failure := outcome.Error
					if failure == "" {
						failure = "failed"
					}
					content = map[string]any{"error": failure}
				}
				encodedContent, err := json.Marshal(content)
				if err != nil {
					return TurnResult{}, err
				}
				messages = append(messages, model.Message{
					Role: "tool", Content: string(encodedContent), ToolCallID: call.ID, ToolName: call.ToolID,
				})
			}
			continue
		}

		answerText = result.Text
		break
	}

	if answerText == nil || strings.TrimSpace(*answerText) == "" {
		return t.escalate(ReasonBudgetExhausted, "The AI could not produce an answer; a human should take over.", "")
	}

	cited := ExtractCitations(*answerText, len(t.evidence))
	hallucinated := HasHallucinatedCitation(*answerText, len(t.evidence))
	if (len(cited) == 0 && len(t.evidence) > 0) || hallucinated {
		messages = append(messages, model.Message{
			Role:    "user",
			Content: "Your answer was missing valid [E#] citations or used invalid ones. Answer again, citing the evidence you actually used. If you cannot answer from the evidence, say so.",
		})
		retry, err := deps.Chat(ctx, model.ChatRequest{
			ModelID: "agent", Messages: messages, MaxOutputTokens: policy.Budgets.MaxOutputTokens,
		})
		if err != nil {
			return TurnResult{}, err
		}
		t.usage.InputTokens += retry.Usage.InputTokens
		t.usage.OutputTokens += retry.Usage.OutputTokens
		answerText = retry.Text
		var retriedCitations []int
		retriedHallucinated := true
		if answerText != nil {
			retriedCitations = ExtractCitations(*answerText, len(t.evidence))
			retriedHallucinated = HasHallucinatedCitation(*answerText, len(t.evidence))
		}
		if answerText == nil || (len(retriedCitations) == 0 && len(t.evidence) > 0) || retriedHallucinated {
			return t.escalate(ReasonMissingCitations, "The AI could not ground its answer in the knowledge base; a human should answer.", "")
		}
	}

	finalCitations := ExtractCitations(*answerText, len(t.evidence))
	citations := make([]Citation, 0, len(finalCitations))
	citationsJSON := make([]map[string]any, 0, len(finalCitations))
	for _, n := range finalCitations {
		item := t.evidence[n-1]
		citations = append(citations, Citation{Marker: fmt.Sprintf("E%d", n), ChunkID: item.ChunkID})
		citationsJSON = append(citationsJSON, map[string]any{
			"chunkId":    item.ChunkID,
			"sourceId":   item.SourceID,
			"sourceName": item.SourceName,
			"version":    item.Version,
			"locator":    item.Locator,
			"quote":      truncate(item.Content, 200),
		})
	}
	encodedCitations, err := json.Marshal(citationsJSON)
	if err != nil {
		return TurnResult{}, err
	}
	if _, err := t.deps.Pool.Exec(t.ctx,
		`insert into messages (org_id, conversation_id, role, content, citations) values ($1, $2, 'agent_ai', $3, $4::jsonb)`,
		t.rc.OrgID, t.rc.ConversationID, *answerText, string(encodedCitations)); err != nil {
		return TurnResult{}, err
	}
	if _, err := t.deps.Pool.Exec(t.ctx,
		`update conversations set first_response_at = coalesce(first_response_at, now()), updated_at = now() where id = $1`,
		t.rc.ConversationID); err != nil {
		return TurnResult{}, err
	}
	status := "completed"
	outcome := "answered"
	if err := t.finishRun(status, &outcome, nil); err != nil {
		return TurnResult{}, err
	}
	if err := t.appendAudit("run.completed", "ai_run", t.rc.RunID, map[string]any{"citations": len(citations)}); err != nil {
		return TurnResult{}, err
	}
	if err := t.publish("message.created"); err != nil {
		return TurnResult{}, err
	}

	return TurnResult{
		Outcome:   "answered",
		Citations: citations,
		Evidence:  t.evidence,
		Steps:     t.steps,
		Usage:     t.usage,
	}, nil
}
