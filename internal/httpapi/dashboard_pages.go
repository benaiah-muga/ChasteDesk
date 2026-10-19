// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"chastedesk/internal/agent"
	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/web/views"
)

func greeting(name string) string {
	first := strings.Fields(name)
	label := "there"
	if len(first) > 0 && first[0] != "" {
		label = first[0]
	}
	hour := time.Now().Hour()
	switch {
	case hour < 12:
		return "Good morning, " + label + "."
	case hour < 18:
		return "Good afternoon, " + label + "."
	default:
		return "Good evening, " + label + "."
	}
}

func (s *Server) handleOverviewPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	shell, err := s.shellFor(r, sess, "overview", "Overview", "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	ctx := db.WithOrg(r.Context(), sess.OrgID)
	counts, err := s.repo.OverviewCounts(ctx)
	if err != nil {
		s.respondError(w, r, err)
		return
	}

	metrics, _, metricsErr := s.analyticsPageData(r, sess.OrgID, 14)
	if metricsErr != nil {
		metrics = nil
	}
	aiResolution := "-"
	medianFirst := "-"
	if metrics != nil {
		aiResolution = rateLabel(metrics.AIResolutionRate)
		medianFirst = secondsLabel(metrics.P50FirstResponseSeconds)
	}
	stats := []views.StatView{
		{Label: "Open conversations", Value: fmt.Sprintf("%d", counts.OpenConversations), Hint: fmt.Sprintf("%d awaiting a human", counts.AwaitingHuman), Icon: "inbox"},
		{Label: "Awaiting a human", Value: fmt.Sprintf("%d", counts.AwaitingHuman), Hint: "Escalated with a complete handoff", Icon: "alert"},
		{Label: "AI resolution rate", Value: aiResolution, Hint: "No human reply and no approved action", Icon: "bot"},
		{Label: "Median first response", Value: medianFirst, Hint: "Latest rolled-up UTC day", Icon: "clock"},
	}

	statusText := "AI is answering from ready knowledge."
	statusOK := true
	switch {
	case counts.EnabledProviders == 0:
		statusText = "AI is paused. Every conversation routes to a human until a provider is connected."
		statusOK = false
	case counts.ReadySources == 0:
		statusText = "The AI has no ready knowledge. It will escalate rather than guess."
		statusOK = false
	default:
		statusText = fmt.Sprintf("AI is answering from %d ready %s · %d open %s.", counts.ReadySources, pluralWord(counts.ReadySources, "source", "sources"), counts.OpenConversations, pluralWord(counts.OpenConversations, "conversation", "conversations"))
	}

	rows, err := s.queryInboxList(r.Context(), sess.OrgID, "", "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	recentRows := rows
	if len(recentRows) > 6 {
		recentRows = recentRows[:6]
	}
	recent := s.inboxListView(recentRows, "")

	policySummary := "Auto model · default budgets"
	policyDone := counts.EnabledProviders > 0
	if record, err := s.repo.ActiveAgent(ctx); err == nil {
		var policy agent.AgentPolicy
		if err := json.Unmarshal(record.Policy, &policy); err == nil {
			model := policy.Model.ModelID
			if model == "" {
				model = "Auto model"
			}
			policySummary = fmt.Sprintf("%s · %d steps · %d tool calls", model, policy.Budgets.MaxSteps, policy.Budgets.MaxToolCalls)
			policyDone = policyDone && policy.Budgets.MaxSteps > 0
		}
	}
	readiness := []views.ReadinessView{
		{Title: "AI provider", Body: providerReadiness(counts.EnabledProviders), Done: counts.EnabledProviders > 0, Href: "/dashboard/settings", Action: actionWord(counts.EnabledProviders > 0, "Review", "Connect")},
		{Title: "Knowledge", Body: knowledgeReadiness(counts.ReadySources, counts.Sources), Done: counts.ReadySources > 0, Href: "/dashboard/knowledge?add=url", Action: actionWord(counts.ReadySources > 0, "Manage", "Add")},
		{Title: "Agent policy", Body: policySummary, Done: policyDone, Href: "/dashboard/settings?section=agent", Action: "Review"},
		{Title: "Widget origin", Body: originsReadiness(counts.WidgetOrigins), Done: counts.WidgetOrigins > 0, Href: "/dashboard/settings?section=widget", Action: actionWord(counts.WidgetOrigins > 0, "Manage", "Allow")},
	}

	needs := []views.NeedView{}
	if counts.PendingApprovals > 0 {
		needs = append(needs, views.NeedView{
			Title: fmt.Sprintf("%d %s waiting", counts.PendingApprovals, pluralWord(counts.PendingApprovals, "approval", "approvals")),
			Body:  "Risky actions stopped before execution",
			Href:  "/dashboard/approvals",
			Icon:  "approvals",
		})
	}
	if counts.AwaitingHuman > 0 {
		needs = append(needs, views.NeedView{
			Title: awaitingLabel(counts.AwaitingHuman),
			Body:  "The AI escalated and left a handoff",
			Href:  "/dashboard/inbox?status=awaiting_human",
			Icon:  "alert",
		})
	}
	if counts.FailedSources > 0 {
		needs = append(needs, views.NeedView{
			Title: fmt.Sprintf("%d source %s failed", counts.FailedSources, pluralWord(counts.FailedSources, "ingest", "ingests")),
			Body:  "Inspect the error and reindex",
			Href:  "/dashboard/knowledge",
			Icon:  "book",
		})
	}
	if counts.EnabledProviders == 0 {
		needs = append(needs, views.NeedView{
			Title: "Connect an AI provider",
			Body:  "Without one, every conversation routes to a human",
			Href:  "/dashboard/settings",
			Icon:  "key",
		})
	}
	if counts.ReadySources == 0 {
		needs = append(needs, views.NeedView{
			Title: "Add a knowledge source",
			Body:  "The AI will not answer from thin air",
			Href:  "/dashboard/knowledge?add=url",
			Icon:  "globe",
		})
	}

	renderPage(w, views.DashboardPage(shell, views.OverviewView{
		Greeting:    greeting(sess.Name) + " Here is the desk right now.",
		StatusText:  statusText,
		StatusOK:    statusOK,
		Stats:       stats,
		Recent:      recent,
		Readiness:   readiness,
		Needs:       needs,
		Pending:     counts.PendingApprovals,
		SourceCount: counts.Sources,
	}))
}

func pluralWord(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func actionWord(done bool, doneAction, missingAction string) string {
	if done {
		return doneAction
	}
	return missingAction
}

func providerReadiness(count int) string {
	if count == 0 {
		return "No provider connected"
	}
	return fmt.Sprintf("%d %s connected", count, pluralWord(count, "provider", "providers"))
}

func knowledgeReadiness(ready, total int) string {
	if total == 0 {
		return "No sources yet"
	}
	return fmt.Sprintf("%d of %d ready", ready, total)
}

func originsReadiness(count int) string {
	if count == 0 {
		return "No origin allowed"
	}
	return fmt.Sprintf("%d %s allowed", count, pluralWord(count, "origin", "origins"))
}

func awaitingLabel(count int) string {
	if count == 1 {
		return "1 conversation needs a human"
	}
	return fmt.Sprintf("%d conversations need a human", count)
}
