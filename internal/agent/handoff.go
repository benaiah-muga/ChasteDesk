// SPDX-License-Identifier: AGPL-3.0-only
package agent

import "chastedesk/internal/retrieval"

type HandoffEvidence struct {
	Source  string `json:"source"`
	Version int    `json:"version"`
	Quote   string `json:"quote"`
}

type HandoffCustomerContext struct {
	Name                *string        `json:"name"`
	Email               *string        `json:"email"`
	Attributes          map[string]any `json:"attributes"`
	RecentConversations []string       `json:"recentConversations"`
}

type HandoffPackage struct {
	Problem             string                 `json:"problem"`
	CustomerContext     HandoffCustomerContext `json:"customerContext"`
	Investigated        []string               `json:"investigated"`
	Evidence            []HandoffEvidence      `json:"evidence"`
	ActionsTaken        []string               `json:"actionsTaken"`
	RecommendedNextStep string                 `json:"recommendedNextStep"`
}

func trimList(values []string, limit int) []string {
	if len(values) > limit {
		return values[:limit]
	}
	return values
}

func truncate(text string, limit int) string {
	if len(text) > limit {
		return text[:limit]
	}
	return text
}

func BuildHandoffPackage(input TurnInput, investigated []string, evidence []retrieval.Evidence, actionsTaken []string, recommendedNextStep string) HandoffPackage {
	handoffEvidence := make([]HandoffEvidence, 0, len(evidence))
	for i, item := range evidence {
		if i == 5 {
			break
		}
		handoffEvidence = append(handoffEvidence, HandoffEvidence{
			Source:  item.SourceName,
			Version: item.Version,
			Quote:   truncate(item.Content, 200),
		})
	}
	return HandoffPackage{
		Problem: truncate(input.CustomerMessage, 500),
		CustomerContext: HandoffCustomerContext{
			Name:                input.Customer.Name,
			Email:               input.Customer.Email,
			Attributes:          input.Customer.Attributes,
			RecentConversations: trimList(input.RecentConversations, 5),
		},
		Investigated:        trimList(investigated, 10),
		Evidence:            handoffEvidence,
		ActionsTaken:        trimList(actionsTaken, 10),
		RecommendedNextStep: recommendedNextStep,
	}
}
