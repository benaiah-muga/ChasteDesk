// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"regexp"
	"strings"

	"chastedesk/internal/retrieval"
)

type EscalationReason string

const (
	ReasonPolicyKeyword    EscalationReason = "policy_keyword"
	ReasonExplicitRequest  EscalationReason = "explicit_request"
	ReasonLowConfidence    EscalationReason = "low_confidence"
	ReasonDeniedTool       EscalationReason = "denied_tool"
	ReasonBudgetExhausted  EscalationReason = "budget_exhausted"
	ReasonMissingCitations EscalationReason = "missing_citations"
	ReasonToolFailure      EscalationReason = "tool_failure"
)

type ToolMode string

const (
	ModeAutonomous       ToolMode = "autonomous"
	ModeApprovalRequired ToolMode = "approval_required"
	ModeDisabled         ToolMode = "disabled"
)

type PolicyTool struct {
	ToolID string `json:"toolId"`
	Mode   string `json:"mode"`
}

type EscalationPolicy struct {
	LowConfidenceThreshold float64  `json:"lowConfidenceThreshold"`
	Keywords               []string `json:"keywords"`
	AlwaysEscalateTopics   []string `json:"alwaysEscalateTopics"`
}

type ModelPolicy struct {
	ProviderID string `json:"providerId"`
	ModelID    string `json:"modelId"`
}

type BudgetPolicy struct {
	MaxSteps        int `json:"maxSteps"`
	MaxToolCalls    int `json:"maxToolCalls"`
	MaxInputTokens  int `json:"maxInputTokens"`
	MaxOutputTokens int `json:"maxOutputTokens"`
}

type AgentPolicy struct {
	Purpose         string           `json:"purpose"`
	KnowledgeScopes []string         `json:"knowledgeScopes"`
	Tools           []PolicyTool     `json:"tools"`
	Escalation      EscalationPolicy `json:"escalation"`
	Model           ModelPolicy      `json:"model"`
	Budgets         BudgetPolicy     `json:"budgets"`
}

func (p AgentPolicy) ToolMode(toolID string) ToolMode {
	for _, tool := range p.Tools {
		if tool.ToolID == toolID {
			return ToolMode(tool.Mode)
		}
	}
	return ModeDisabled
}

var explicitRequestRe = regexp.MustCompile(`(talk to|speak to|speak with|want) a (human|real person|agent|person)`)

func EscalationCheck(customerMessage string, policy AgentPolicy, bestConfidence *float64) EscalationReason {
	text := strings.ToLower(customerMessage)
	for _, keyword := range policy.Escalation.Keywords {
		if keyword != "" && strings.Contains(text, strings.ToLower(keyword)) {
			return ReasonPolicyKeyword
		}
	}
	if explicitRequestRe.MatchString(text) || strings.Contains(text, "human agent") {
		return ReasonExplicitRequest
	}
	if bestConfidence != nil && *bestConfidence < policy.Escalation.LowConfidenceThreshold {
		return ReasonLowConfidence
	}
	return ""
}

func BestEvidenceConfidence(evidence []retrieval.Evidence) *float64 {
	if len(evidence) == 0 {
		return nil
	}
	best := evidence[0].Confidence
	for _, item := range evidence[1:] {
		if item.Confidence > best {
			best = item.Confidence
		}
	}
	return &best
}
