// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"chastedesk/internal/agent"
	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	mailemail "chastedesk/internal/email"
	"chastedesk/internal/id"
	"chastedesk/internal/model"
	"chastedesk/internal/ratelimit"
	"chastedesk/internal/web/views"
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type widgetTheme struct {
	DisplayName  string `json:"displayName"`
	Greeting     string `json:"greeting"`
	PrimaryColor string `json:"primaryColor"`
	Position     string `json:"position"`
}

func (s *Server) settingsSection(r *http.Request) string {
	switch r.URL.Query().Get("section") {
	case "agent", "widget", "api-keys", "workspace", "account":
		return r.URL.Query().Get("section")
	default:
		return "providers"
	}
}

func (s *Server) settingsView(r *http.Request, sess *auth.Session, section, createdKey string) (views.SettingsView, error) {
	ctx := db.WithOrg(r.Context(), sess.OrgID)
	view := views.SettingsView{
		Section: section, ProviderOptions: views.ProviderCatalog(),
		CurrentUserEmail: sess.Email, CurrentRole: string(sess.Role), AccountName: sess.Name,
		CreatedKey:     createdKey,
		WidgetPosition: "right", WidgetEnabled: true,
		WidgetGreeting: "Hi, how can we help?",
		WidgetPrimary:  "#d96c3f",
	}
	orgName, err := s.repo.OrgName(ctx)
	if err != nil {
		return view, err
	}
	view.OrgName = orgName
	view.WidgetDisplayName = orgName

	providers, err := s.repo.ListProviders(ctx)
	if err != nil {
		return view, err
	}
	for _, provider := range providers {
		item := views.ProviderView{
			ID: provider.ID, ProviderID: provider.ProviderID, DisplayName: provider.DisplayName,
			Enabled: provider.Enabled, ModelCount: provider.ModelCount,
		}
		if provider.KeyLast4 != nil {
			item.KeyLast4 = *provider.KeyLast4
		}
		if provider.BaseURL != nil {
			item.BaseURL = *provider.BaseURL
		}
		view.Providers = append(view.Providers, item)
	}

	originsText, snippet, err := s.widgetSnippet(r, sess.OrgID)
	if err != nil {
		return view, err
	}
	view.OriginsText = originsText
	view.EmbedSnippet = snippet
	config, err := s.repo.GetWidgetConfig(ctx)
	if err != nil {
		return view, err
	}
	if config != nil {
		view.WidgetEnabled = config.Enabled
		var theme widgetTheme
		if len(config.Theme) > 0 {
			_ = json.Unmarshal(config.Theme, &theme)
		}
		if theme.DisplayName != "" {
			view.WidgetDisplayName = theme.DisplayName
		}
		if theme.Greeting != "" {
			view.WidgetGreeting = theme.Greeting
		}
		if theme.PrimaryColor != "" {
			view.WidgetPrimary = theme.PrimaryColor
		}
		if theme.Position == "left" {
			view.WidgetPosition = "left"
		}
	}

	keys, err := s.repo.ListAPIKeys(ctx)
	if err != nil {
		return view, err
	}
	for _, key := range keys {
		item := views.APIKeyView{ID: key.ID, Name: key.Name, Prefix: key.KeyPrefix, CreatedAt: views.RelativeTime(key.CreatedAt), LastUsed: "never"}
		if key.LastUsedAt != nil {
			item.LastUsed = views.RelativeTime(*key.LastUsedAt)
		}
		view.APIKeys = append(view.APIKeys, item)
	}

	members, err := s.repo.ListMembers(ctx)
	if err != nil {
		return view, err
	}
	for _, member := range members {
		view.Members = append(view.Members, views.MemberView{
			ID: member.UserID, Name: member.Name, Email: member.Email, Role: member.Role,
			Joined: member.JoinedAt.Format("Jan 2006"), Self: member.UserID == sess.UserID,
		})
	}

	view.CanManage = auth.Can(sess.Role, auth.CapSettingsWrite)
	invitations, err := s.repo.ListPendingInvitations(ctx)
	if err != nil {
		return view, err
	}
	for _, invite := range invitations {
		view.Invitations = append(view.Invitations, views.InvitationView{
			ID: invite.ID, Email: invite.Email, Role: invite.Role,
			Expires: invite.ExpiresAt.Format("Jan 2"), Link: s.cfg.AppURL + "/invite/" + invite.ID,
		})
	}
	view.Agent = s.agentFormView(r, sess, providers)
	return view, nil
}

func agentTools(policy agent.AgentPolicy) []views.ToolModeView {
	defs := []struct {
		ID          string
		Description string
	}{
		{"knowledge.search", "Search the indexed knowledge base and return evidence."},
		{"customers.lookup", "Read the current customer record. Never crosses organizations."},
		{"conversations.add_note", "Add an internal note the customer never sees."},
		{"escalate", "Stop the run and build a handoff for a human."},
	}
	out := make([]views.ToolModeView, 0, len(defs))
	for _, def := range defs {
		mode := policy.ToolMode(def.ID)
		if mode == "" {
			mode = agent.ModeDisabled
		}
		out = append(out, views.ToolModeView{ID: def.ID, Description: def.Description, Mode: string(mode)})
	}
	return out
}

func (s *Server) agentFormView(r *http.Request, sess *auth.Session, providers []db.ProviderRow) views.AgentFormView {
	ctx := db.WithOrg(r.Context(), sess.OrgID)
	form := views.AgentFormView{
		Threshold: "0.35", MaxSteps: "6", MaxToolCalls: "4", MaxInputTokens: "8000", MaxOutputTokens: "2000",
		PublicScope: true, ConfiguredProviders: len(providers),
	}
	policy := agent.AgentPolicy{}
	record, err := s.repo.ActiveAgent(ctx)
	if err == nil {
		_ = json.Unmarshal(record.Policy, &policy)
	} else {
		_ = json.Unmarshal([]byte(db.DefaultAgentPolicy), &policy)
	}
	form.Purpose = policy.Purpose
	form.Keywords = strings.Join(policy.Escalation.Keywords, ", ")
	if policy.Escalation.LowConfidenceThreshold > 0 {
		form.Threshold = strconv.FormatFloat(policy.Escalation.LowConfidenceThreshold, 'f', 2, 64)
	}
	for _, scope := range policy.KnowledgeScopes {
		switch scope {
		case "public":
			form.PublicScope = true
		case "internal":
			form.InternalScope = true
		}
	}
	form.Tools = agentTools(policy)
	form.ProviderID = policy.Model.ProviderID
	form.ModelID = policy.Model.ModelID
	if policy.Budgets.MaxSteps > 0 {
		form.MaxSteps = strconv.Itoa(policy.Budgets.MaxSteps)
	}
	if policy.Budgets.MaxToolCalls > 0 {
		form.MaxToolCalls = strconv.Itoa(policy.Budgets.MaxToolCalls)
	}
	if policy.Budgets.MaxInputTokens > 0 {
		form.MaxInputTokens = strconv.Itoa(policy.Budgets.MaxInputTokens)
	}
	if policy.Budgets.MaxOutputTokens > 0 {
		form.MaxOutputTokens = strconv.Itoa(policy.Budgets.MaxOutputTokens)
	}
	for _, provider := range providers {
		form.Providers = append(form.Providers, views.ProviderOption{ID: provider.ProviderID, Label: views.ProviderLabel(provider.ProviderID)})
	}
	groups, err := s.repo.ListProviderModels(ctx)
	if err == nil {
		for _, group := range groups {
			entry := views.ProviderModels{ProviderID: group.ProviderID, Label: group.DisplayName}
			for _, model := range group.Models {
				name := model.DisplayName
				if name == "" {
					name = model.ID
				}
				entry.Models = append(entry.Models, views.ModelOption{ID: model.ID, DisplayName: name})
			}
			form.ProviderModels = append(form.ProviderModels, entry)
		}
	}
	counts, err := s.repo.OverviewCounts(ctx)
	if err == nil {
		form.HasKnowledge = counts.ReadySources > 0
	}
	return form
}

func (s *Server) renderSettings(w http.ResponseWriter, r *http.Request, sess *auth.Session, section, createdKey string) {
	s.renderSettingsExtra(w, r, sess, section, createdKey, "", "")
}

func (s *Server) renderSettingsExtra(w http.ResponseWriter, r *http.Request, sess *auth.Session, section, createdKey, createdInvite, createdInviteURL string) {
	shell, err := s.shellFor(r, sess, "settings", "Settings", "")
	if section == "agent" {
		shell.Active = "agent"
		shell.PageTitle = "Support agent"
	}
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	view, err := s.settingsView(r, sess, section, createdKey)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	view.CreatedInvite = createdInvite
	view.CreatedInviteURL = createdInviteURL
	renderPage(w, views.SettingsPage(shell, view))
}

func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsRead)
	if !ok {
		return
	}
	s.renderSettings(w, r, sess, s.settingsSection(r), "")
}

func (s *Server) handleSettingsProviderSave(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	body := providerSaveBody{
		ProviderID: r.FormValue("providerId"), DisplayName: r.FormValue("displayName"),
		APIKey: r.FormValue("apiKey"), BaseURL: r.FormValue("baseUrl"),
	}
	if err := s.saveProviderConfig(r.Context(), sess, body); err != nil {
		s.respondError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard/settings?msg=Provider+saved.+The+key+is+stored+encrypted.", http.StatusSeeOther)
}

func (s *Server) handleSettingsProviderTest(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	configID := r.FormValue("configId")
	row, cfg, err := s.providerConfigForID(r, sess.OrgID, configID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	models, listErr := model.For(model.ProviderID(row.ProviderID)).ListModels(r.Context(), cfg)
	cfgID := configID
	if listErr != nil {
		_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
			ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "settings.provider.tested",
			TargetType: "provider_config", TargetID: &cfgID, Payload: map[string]any{"ok": false, "error": listErr.Error()},
		})
		http.Redirect(w, r, "/dashboard/settings?msg=Connection+failed.+Check+the+key+and+base+URL.&tone=danger", http.StatusSeeOther)
		return
	}
	if encoded, err := json.Marshal(models); err == nil {
		_ = s.repo.UpdateModelsCache(db.WithOrg(r.Context(), sess.OrgID), configID, encoded)
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "settings.provider.tested",
		TargetType: "provider_config", TargetID: &cfgID, Payload: map[string]any{"ok": true, "models": len(models)},
	})
	message := url.QueryEscape(fmt.Sprintf("Connected. %d models available.", len(models)))
	http.Redirect(w, r, "/dashboard/settings?msg="+message, http.StatusSeeOther)
}

func (s *Server) handleSettingsProviderRemove(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	configID := r.FormValue("configId")
	found, err := s.repo.DeleteProviderConfig(db.WithOrg(r.Context(), sess.OrgID), configID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if found {
		_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
			ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "settings.provider.saved",
			TargetType: "provider_config", TargetID: &configID, Payload: map[string]any{"deleted": true},
		})
	}
	http.Redirect(w, r, "/dashboard/settings?msg=Provider+removed.", http.StatusSeeOther)
}

func (s *Server) handleSettingsWidgetSave(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	origins := []string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(r.FormValue("origins"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parsed, err := url.Parse(trimmed)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			s.renderPageError(w, r, http.StatusBadRequest, "Invalid origin", "Origins must be full URLs such as https://example.com. Paths are ignored.")
			return
		}
		origin := parsed.Scheme + "://" + parsed.Host
		if seen[origin] {
			continue
		}
		seen[origin] = true
		origins = append(origins, origin)
		if len(origins) > 20 {
			s.renderPageError(w, r, http.StatusBadRequest, "Too many origins", "Allow at most twenty origins for one widget.")
			return
		}
	}
	displayName := strings.TrimSpace(r.FormValue("displayName"))
	if len(displayName) > 60 {
		displayName = displayName[:60]
	}
	greeting := strings.TrimSpace(r.FormValue("greeting"))
	if len(greeting) > 160 {
		greeting = greeting[:160]
	}
	primary := strings.TrimSpace(r.FormValue("primaryColor"))
	if primary == "" {
		primary = "#d96c3f"
	}
	if !hexColor.MatchString(primary) {
		s.renderPageError(w, r, http.StatusBadRequest, "Invalid colour", "Use a six-digit hex colour such as #d96c3f.")
		return
	}
	position := "right"
	if r.FormValue("position") == "left" {
		position = "left"
	}
	enabled := r.FormValue("enabled") == "on" || r.FormValue("enabled") == "1" || r.FormValue("enabled") == "true"
	if displayName == "" {
		if orgName, err := s.repo.OrgName(db.WithOrg(r.Context(), sess.OrgID)); err == nil {
			displayName = orgName
		}
	}
	theme, err := json.Marshal(widgetTheme{DisplayName: displayName, Greeting: greeting, PrimaryColor: primary, Position: position})
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if err := s.repo.UpsertWidgetConfigWithEnabled(db.WithOrg(r.Context(), sess.OrgID), origins, theme, enabled); err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "settings.widget.updated",
		TargetType: "widget_config", Payload: map[string]any{"origins": origins, "enabled": enabled},
	})
	http.Redirect(w, r, "/dashboard/settings?section=widget&msg=Widget+settings+saved.", http.StatusSeeOther)
}

func (s *Server) handleSettingsAgentSave(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ctx := db.WithOrg(r.Context(), sess.OrgID)
	record, err := s.repo.ActiveAgent(ctx)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	var policy agent.AgentPolicy
	if err := json.Unmarshal(record.Policy, &policy); err != nil {
		policy = agent.AgentPolicy{}
	}
	purpose := strings.TrimSpace(r.FormValue("purpose"))
	if purpose == "" || len(purpose) > 4000 {
		s.renderPageError(w, r, http.StatusBadRequest, "Purpose required", "Describe what the agent is for in 1–4000 characters.")
		return
	}
	scopes := []string{}
	if r.FormValue("publicScope") != "" {
		scopes = append(scopes, "public")
	}
	if r.FormValue("internalScope") != "" {
		scopes = append(scopes, "internal")
	}
	if len(scopes) == 0 {
		s.renderPageError(w, r, http.StatusBadRequest, "Choose a knowledge scope", "Enable at least one scope. Public-only is the safe default.")
		return
	}
	threshold, err := strconv.ParseFloat(r.FormValue("threshold"), 64)
	if err != nil || threshold < 0 || threshold > 1 {
		s.renderPageError(w, r, http.StatusBadRequest, "Invalid threshold", "The low-confidence threshold must be between 0 and 1.")
		return
	}
	keywords := []string{}
	seen := map[string]bool{}
	for _, raw := range strings.Split(r.FormValue("keywords"), ",") {
		keyword := strings.TrimSpace(raw)
		if keyword == "" || len(keyword) > 60 || seen[strings.ToLower(keyword)] {
			continue
		}
		seen[strings.ToLower(keyword)] = true
		keywords = append(keywords, keyword)
		if len(keywords) >= 20 {
			break
		}
	}
	providerID := strings.TrimSpace(r.FormValue("providerId"))
	if providerID != "" {
		known := false
		for _, provider := range views.ProviderCatalog() {
			if provider.ID == providerID {
				known = true
			}
		}
		if !known {
			s.renderPageError(w, r, http.StatusBadRequest, "Unknown provider", "Choose a provider from the list, or leave it on auto.")
			return
		}
	}
	toolModes := []agent.PolicyTool{}
	for _, toolID := range []string{"knowledge.search", "customers.lookup", "conversations.add_note", "escalate"} {
		mode := r.FormValue("tool:" + toolID)
		switch mode {
		case "autonomous", "approval_required", "disabled":
		default:
			mode = "disabled"
		}
		toolModes = append(toolModes, agent.PolicyTool{ToolID: toolID, Mode: mode})
	}
	maxSteps, ok := boundedInt(r.FormValue("maxSteps"), 1, 12, 6)
	if !ok {
		s.renderPageError(w, r, http.StatusBadRequest, "Invalid budget", "Max steps must be between 1 and 12.")
		return
	}
	maxToolCalls, ok := boundedInt(r.FormValue("maxToolCalls"), 0, 12, 4)
	if !ok {
		s.renderPageError(w, r, http.StatusBadRequest, "Invalid budget", "Max tool calls must be between 0 and 12.")
		return
	}
	maxInputTokens, ok := boundedInt(r.FormValue("maxInputTokens"), 500, 64000, 8000)
	if !ok {
		s.renderPageError(w, r, http.StatusBadRequest, "Invalid budget", "Max input tokens must be between 500 and 64000.")
		return
	}
	maxOutputTokens, ok := boundedInt(r.FormValue("maxOutputTokens"), 100, 16000, 2000)
	if !ok {
		s.renderPageError(w, r, http.StatusBadRequest, "Invalid budget", "Max output tokens must be between 100 and 16000.")
		return
	}
	policy.Purpose = purpose
	policy.KnowledgeScopes = scopes
	policy.Tools = toolModes
	policy.Escalation.LowConfidenceThreshold = threshold
	policy.Escalation.Keywords = keywords
	policy.Model.ProviderID = providerID
	policy.Model.ModelID = strings.TrimSpace(r.FormValue("modelId"))
	if len(policy.Model.ModelID) > 200 {
		policy.Model.ModelID = policy.Model.ModelID[:200]
	}
	policy.Budgets = agent.BudgetPolicy{
		MaxSteps: maxSteps, MaxToolCalls: maxToolCalls,
		MaxInputTokens: maxInputTokens, MaxOutputTokens: maxOutputTokens,
	}
	encoded, err := json.Marshal(policy)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if err := s.repo.UpdateAgentPolicy(ctx, record.ID, encoded); err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(ctx, db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "agent.updated",
		TargetType: "agent", TargetID: &record.ID,
		Payload: map[string]any{"scopes": scopes, "tools": len(toolModes), "model": policy.Model.ModelID},
	})
	http.Redirect(w, r, "/dashboard/settings?section=agent&msg=Agent+policy+saved.", http.StatusSeeOther)
}

func boundedInt(raw string, min, max, fallback int) (int, bool) {
	if strings.TrimSpace(raw) == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	if value < min || value > max {
		return 0, false
	}
	return value, true
}

func (s *Server) handleSettingsAPIKeyCreate(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" || len(name) > 80 {
		s.renderPageError(w, r, http.StatusBadRequest, "Name required", "Give the key a name between 1 and 80 characters.")
		return
	}
	secret := "cd_" + id.Token(32)
	sum := sha256.Sum256([]byte(secret))
	keyID, err := s.repo.CreateAPIKey(db.WithOrg(r.Context(), sess.OrgID), name, secret[:8], hex.EncodeToString(sum[:]))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "apikey.created",
		TargetType: "api_key", TargetID: &keyID, Payload: map[string]any{"name": name, "prefix": secret[:8]},
	})
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	s.renderSettings(w, r, sess, "api-keys", secret)
}

func (s *Server) handleSettingsAPIKeyRevoke(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	keyID := r.FormValue("keyId")
	found, err := s.repo.DeleteAPIKey(db.WithOrg(r.Context(), sess.OrgID), keyID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if found {
		_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
			ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "apikey.revoked",
			TargetType: "api_key", TargetID: &keyID, Payload: map[string]any{},
		})
	}
	http.Redirect(w, r, "/dashboard/settings?section=api-keys&msg=API+key+revoked.", http.StatusSeeOther)
}

func (s *Server) handleSettingsWorkspaceSave(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapOrgAdmin)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("orgName"))
	if name == "" || len(name) > 100 {
		s.renderPageError(w, r, http.StatusBadRequest, "Workspace name required", "Use between 1 and 100 characters.")
		return
	}
	if err := s.repo.UpdateOrgName(db.WithOrg(r.Context(), sess.OrgID), name); err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "org.updated",
		TargetType: "organization", TargetID: &sess.OrgID, Payload: map[string]any{"name": name},
	})
	http.Redirect(w, r, "/dashboard/settings?section=workspace&msg=Workspace+name+saved.", http.StatusSeeOther)
}

func (s *Server) handleSettingsAccountSave(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" || len(name) > 100 {
		s.renderPageError(w, r, http.StatusBadRequest, "Name required", "Use between 1 and 100 characters.")
		return
	}
	currentPassword := r.FormValue("currentPassword")
	newPassword := r.FormValue("newPassword")
	if currentPassword != "" || newPassword != "" {
		if len(newPassword) < 8 || len(newPassword) > 200 {
			s.renderPageError(w, r, http.StatusBadRequest, "Password too short", "The new password must be between 8 and 200 characters.")
			return
		}
		cred, err := s.repo.CredentialByEmail(r.Context(), sess.Email)
		if err != nil || !auth.VerifyPassword(currentPassword, cred.PasswordHash) {
			s.renderPageError(w, r, http.StatusBadRequest, "Current password did not match", "Enter your current password to set a new one.")
			return
		}
		hash, err := auth.HashPassword(newPassword)
		if err != nil {
			s.respondError(w, r, err)
			return
		}
		if err := s.repo.UpdatePassword(r.Context(), sess.UserID, hash); err != nil {
			s.respondError(w, r, err)
			return
		}
		if token := s.sessionToken(r); token != "" {
			_ = s.repo.DeleteOtherSessions(r.Context(), sess.UserID, tokenHash(token))
		}
		_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
			ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "auth.password_changed",
			TargetType: "user", TargetID: &sess.UserID, Payload: map[string]any{},
		})
	}
	if err := s.repo.UpdateUserName(r.Context(), sess.UserID, name); err != nil {
		s.respondError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard/account?msg=Account+updated.", http.StatusSeeOther)
}

func (s *Server) handleSettingsInviteCreate(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	role := r.FormValue("role")
	if !validEmail(email) {
		s.renderPageError(w, r, http.StatusBadRequest, "Invalid email", "Enter the address of the teammate you want to invite.")
		return
	}
	if role != "admin" && role != "agent" {
		s.renderPageError(w, r, http.StatusBadRequest, "Invalid role", "Invite someone as an admin or an agent.")
		return
	}
	ctx := db.WithOrg(r.Context(), sess.OrgID)
	var exists bool
	if err := s.pool.QueryRow(r.Context(), `select exists(select 1 from "user" u join account a on a.user_id = u.id and a.provider_id = 'credential' where lower(u.email) = lower($1))`, email).Scan(&exists); err != nil {
		s.respondError(w, r, err)
		return
	}
	if exists {
		s.renderPageError(w, r, http.StatusConflict, "Already registered", "That email already has a sign-in account on this installation. Members share this single organization automatically.")
		return
	}
	invitationID, err := s.repo.CreateInvitation(ctx, email, role, sess.UserID, time.Now().Add(7*24*time.Hour))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(ctx, db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "user.invited",
		TargetType: "invitation", TargetID: &invitationID, Payload: map[string]any{"email": email, "role": role},
	})
	link := s.cfg.AppURL + "/invite/" + invitationID
	if s.cfg.SMTPHost != "" {
		if orgName, nameErr := s.repo.OrgName(ctx); nameErr == nil {
			if _, mailErr := mailemail.EnqueueInvitation(ctx, s.repo, orgName, email, link); mailErr != nil {
				s.logger.Warn("invitation email enqueue failed", "error", mailErr)
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	s.renderSettingsExtra(w, r, sess, "workspace", "", email, link)
}

func (s *Server) handleSettingsInviteRevoke(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	invitationID := r.FormValue("invitationId")
	found, err := s.repo.RevokeInvitation(db.WithOrg(r.Context(), sess.OrgID), invitationID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if found {
		_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
			ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "user.invited",
			TargetType: "invitation", TargetID: &invitationID, Payload: map[string]any{"revoked": true},
		})
	}
	http.Redirect(w, r, "/dashboard/settings?section=workspace&msg=Invitation+revoked.", http.StatusSeeOther)
}

func (s *Server) handleInvitePage(w http.ResponseWriter, r *http.Request) {
	if sess, err := s.sessionFromRequest(r); err == nil && sess != nil && sess.OrgID != "" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	invitation, orgName, err := s.repo.PendingInvitation(r.Context(), r.PathValue("id"))
	if err != nil {
		s.renderPageError(w, r, http.StatusNotFound, "Invitation not valid", "This link has expired, was revoked, or was already used. Ask an owner for a new one.")
		return
	}
	renderPage(w, views.InvitePage("", invitation.Email, invitation.Role, orgName))
}

func (s *Server) handleInviteSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	password := r.FormValue("password")
	if _, err := ratelimit.Enforce(r.Context(), s.pool, "invite:"+clientIP(r), 20, 3600); err != nil {
		s.respondError(w, r, err)
		return
	}
	invitation, orgName, lookupErr := s.repo.PendingInvitation(r.Context(), r.PathValue("id"))
	if lookupErr != nil {
		s.renderPageError(w, r, http.StatusNotFound, "Invitation not valid", "This link has expired, was revoked, or was already used. Ask an owner for a new one.")
		return
	}
	errorMessage := ""
	switch {
	case name == "" || len(name) > 100:
		errorMessage = "Your name must be between 1 and 100 characters."
	case len(password) < 8 || len(password) > 200:
		errorMessage = "Password must be between 8 and 200 characters."
	}
	if errorMessage != "" {
		w.WriteHeader(http.StatusBadRequest)
		renderPage(w, views.InvitePage(errorMessage, invitation.Email, invitation.Role, orgName))
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	userID, orgID, err := s.repo.AcceptInvitation(r.Context(), r.PathValue("id"), name, hash)
	if err != nil {
		if errors.Is(err, db.ErrEmailExists) {
			s.renderPageError(w, r, http.StatusConflict, "Already registered", "That email already has an account. Sign in instead.")
			return
		}
		if errors.Is(err, db.ErrNotFound) {
			s.renderPageError(w, r, http.StatusNotFound, "Invitation not valid", "This link has expired, was revoked, or was already used.")
			return
		}
		s.respondError(w, r, err)
		return
	}
	if err := s.startSession(w, r, userID); err != nil {
		s.respondError(w, r, err)
		return
	}
	s.auditAuthEvent(r, userID, "auth.login", "session")
	_ = s.repo.WriteAuditNoOrg(r.Context(), &orgID, db.AuditInput{
		ActorType: db.ActorUser, ActorID: &userID, Type: "user.accepted_invite",
		TargetType: "user", TargetID: &userID, Payload: map[string]any{},
	})
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleAccountPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	shell, err := s.shellFor(r, sess, "account", "Account", "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	renderPage(w, views.AccountPage(shell, views.SettingsView{
		AccountName: sess.Name, CurrentUserEmail: sess.Email, CurrentRole: string(sess.Role),
	}))
}
