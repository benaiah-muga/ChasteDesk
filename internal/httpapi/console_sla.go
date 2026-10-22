// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"net/http"
	"strings"

	"chastedesk/internal/auth"
	"chastedesk/internal/db"
)

type consoleSLAPolicyBody struct {
	Priority             string `json:"priority"`
	FirstResponseMinutes int    `json:"firstResponseMinutes"`
	ResolutionMinutes    int    `json:"resolutionMinutes"`
	Enabled              bool   `json:"enabled"`
}

type consoleSLASaveBody struct {
	Policies []consoleSLAPolicyBody `json:"policies"`
}

func (s *Server) handleConsoleSLASave(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	body, err := decodeBody[consoleSLASaveBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if len(body.Policies) != 4 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Provide one policy for each priority.")
		return
	}
	seen := map[string]bool{}
	policies := make([]db.SLAPolicyInput, 0, 4)
	for _, item := range body.Policies {
		priority := strings.ToLower(strings.TrimSpace(item.Priority))
		if !ticketPriorities[priority] || seen[priority] {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "Each priority must appear exactly once.")
			return
		}
		if item.FirstResponseMinutes < 1 || item.FirstResponseMinutes > 525600 || item.ResolutionMinutes < 1 || item.ResolutionMinutes > 525600 {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "SLA targets must be between 1 minute and 365 days.")
			return
		}
		seen[priority] = true
		policies = append(policies, db.SLAPolicyInput{
			Priority: priority, FirstResponseMinutes: item.FirstResponseMinutes,
			ResolutionMinutes: item.ResolutionMinutes, Enabled: item.Enabled,
		})
	}
	ctx := db.WithOrg(r.Context(), sess.OrgID)
	saved, err := s.repo.SaveSLAPolicies(ctx, policies)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(ctx, db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "settings.sla.updated",
		TargetType: "sla_policies", Payload: map[string]any{"policies": len(saved)},
	})
	out := make([]map[string]any, 0, len(saved))
	for _, policy := range saved {
		out = append(out, slaPolicyPayload(policy))
	}
	writeData(w, http.StatusOK, map[string]any{"policies": out})
}

func slaPolicyPayload(policy db.SLAPolicy) map[string]any {
	return map[string]any{
		"id": policy.ID, "priority": policy.Priority,
		"firstResponseMinutes": policy.FirstResponseMinutes,
		"resolutionMinutes":    policy.ResolutionMinutes, "enabled": policy.Enabled,
	}
}
