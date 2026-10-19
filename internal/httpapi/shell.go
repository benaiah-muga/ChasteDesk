// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"net/http"

	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/web/views"
)

func (s *Server) shellFor(r *http.Request, sess *auth.Session, active, title, sub string) (views.ShellView, error) {
	ctx := db.WithOrg(r.Context(), sess.OrgID)
	counts, err := s.repo.OverviewCounts(ctx)
	if err != nil {
		return views.ShellView{}, err
	}
	orgName, err := s.repo.OrgName(ctx)
	if err != nil {
		return views.ShellView{}, err
	}
	onboarding, err := s.repo.OnboardingState(ctx)
	if err != nil {
		return views.ShellView{}, err
	}
	flash := r.URL.Query().Get("msg")
	tone := r.URL.Query().Get("tone")
	if tone == "" {
		tone = "success"
	}
	shell := views.ShellView{
		Active:               active,
		PageTitle:            title,
		PageSub:              sub,
		UserName:             sess.Name,
		UserEmail:            sess.Email,
		UserInitial:          views.InitialOf(sess.Name),
		Role:                 string(sess.Role),
		OrgName:              orgName,
		OrgInitial:           views.InitialOf(orgName),
		OpenConversations:    counts.OpenConversations,
		PendingApprovals:     counts.PendingApprovals,
		OnboardingIncomplete: !onboarding.Complete,
		CanAudit:             auth.Can(sess.Role, auth.CapAuditRead),
		CanSettings:          auth.Can(sess.Role, auth.CapSettingsRead),
		CanKnowledgeWrite:    auth.Can(sess.Role, auth.CapKnowledgeWrite),
		Flash:                flash,
		FlashTone:            tone,
		InboxStream:          active == "inbox",
	}
	return shell, nil
}
