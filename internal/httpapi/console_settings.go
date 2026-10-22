// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"chastedesk/internal/agent"
	"chastedesk/internal/apperr"
	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/id"
	"chastedesk/internal/model"
)

func consoleStringsFromLines(value string) []string {
	out := []string{}
	for _, line := range strings.Split(value, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func (s *Server) handleConsoleSettings(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsRead)
	if !ok {
		return
	}
	view, err := s.settingsView(r, sess, "providers", "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	providers := make([]map[string]any, 0, len(view.Providers))
	for _, provider := range view.Providers {
		providers = append(providers, map[string]any{
			"id": provider.ID, "providerId": provider.ProviderID, "displayName": provider.DisplayName,
			"keyLast4": provider.KeyLast4, "baseUrl": provider.BaseURL, "modelCount": provider.ModelCount,
			"enabled": provider.Enabled,
		})
	}
	providerOptions := make([]map[string]any, 0, len(view.ProviderOptions))
	for _, option := range view.ProviderOptions {
		providerOptions = append(providerOptions, map[string]any{
			"id": option.ID, "label": option.Label, "keyless": option.Keyless, "description": option.Description,
		})
	}
	apiKeys := make([]map[string]any, 0, len(view.APIKeys))
	for _, key := range view.APIKeys {
		apiKeys = append(apiKeys, map[string]any{
			"id": key.ID, "name": key.Name, "prefix": key.Prefix, "lastUsed": key.LastUsed, "createdAt": key.CreatedAt,
		})
	}
	members := make([]map[string]any, 0, len(view.Members))
	for _, member := range view.Members {
		members = append(members, map[string]any{
			"id": member.ID, "name": member.Name, "email": member.Email, "role": member.Role, "joined": member.Joined, "self": member.Self,
		})
	}
	invitations := make([]map[string]any, 0, len(view.Invitations))
	for _, invitation := range view.Invitations {
		invitations = append(invitations, map[string]any{
			"id": invitation.ID, "email": invitation.Email, "role": invitation.Role,
			"expires": invitation.Expires, "link": invitation.Link,
		})
	}
	tools := make([]map[string]any, 0, len(view.Agent.Tools))
	for _, tool := range view.Agent.Tools {
		tools = append(tools, map[string]any{"id": tool.ID, "description": tool.Description, "mode": tool.Mode})
	}
	agentProviders := make([]map[string]any, 0, len(view.Agent.Providers))
	for _, provider := range view.Agent.Providers {
		agentProviders = append(agentProviders, map[string]any{"id": provider.ID, "label": provider.Label})
	}
	providerModels := make([]map[string]any, 0, len(view.Agent.ProviderModels))
	for _, group := range view.Agent.ProviderModels {
		models := make([]map[string]any, 0, len(group.Models))
		for _, item := range group.Models {
			models = append(models, map[string]any{"id": item.ID, "displayName": item.DisplayName})
		}
		providerModels = append(providerModels, map[string]any{
			"providerId": group.ProviderID, "label": group.Label, "models": models,
		})
	}
	slaPolicies := []map[string]any{}
	if rows, err := s.repo.ListSLAPolicies(db.WithOrg(r.Context(), sess.OrgID)); err == nil {
		for _, policy := range rows {
			slaPolicies = append(slaPolicies, slaPolicyPayload(policy))
		}
	}
	scimTokens := []map[string]any{}
	canManageSCIM := auth.Can(sess.Role, auth.CapOrgAdmin)
	if canManageSCIM {
		if rows, err := s.repo.ListSCIMTokens(db.WithOrg(r.Context(), sess.OrgID)); err == nil {
			for _, token := range rows {
				scimTokens = append(scimTokens, map[string]any{
					"id": token.ID, "name": token.Name, "createdAt": token.CreatedAt, "lastUsedAt": token.LastUsedAt,
				})
			}
		}
	}
	writeData(w, http.StatusOK, map[string]any{
		"org":             map[string]any{"id": sess.OrgID, "name": view.OrgName},
		"user":            map[string]any{"id": sess.UserID, "name": view.AccountName, "email": view.CurrentUserEmail, "role": view.CurrentRole},
		"canManage":       view.CanManage,
		"canAdmin":        auth.Can(sess.Role, auth.CapOrgAdmin),
		"providers":       providers,
		"providerOptions": providerOptions,
		"agent": map[string]any{
			"purpose": view.Agent.Purpose, "publicScope": view.Agent.PublicScope, "internalScope": view.Agent.InternalScope,
			"tools": tools, "keywords": view.Agent.Keywords, "threshold": view.Agent.Threshold,
			"providerId": view.Agent.ProviderID, "modelId": view.Agent.ModelID,
			"maxSteps": view.Agent.MaxSteps, "maxToolCalls": view.Agent.MaxToolCalls,
			"maxInputTokens": view.Agent.MaxInputTokens, "maxOutputTokens": view.Agent.MaxOutputTokens,
			"providers": agentProviders, "providerModels": providerModels,
			"configuredProviders": view.Agent.ConfiguredProviders,
			"hasKnowledge":        view.Agent.HasKnowledge, "hasEmbedding": view.Agent.HasEmbedding,
		},
		"widget": map[string]any{
			"origins":      consoleStringsFromLines(view.OriginsText),
			"enabled":      view.WidgetEnabled,
			"displayName":  view.WidgetDisplayName,
			"greeting":     view.WidgetGreeting,
			"primaryColor": view.WidgetPrimary,
			"position":     view.WidgetPosition,
			"embedSnippet": view.EmbedSnippet,
		},
		"apiKeys":     apiKeys,
		"members":     members,
		"invitations": invitations,
		"slaPolicies": slaPolicies,
		"scim": map[string]any{
			"baseUrl": s.cfg.AppURL + "/scim/v2", "tokens": scimTokens, "canManage": canManageSCIM,
		},
	})
}

func (s *Server) handleConsoleProviderSave(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	body, err := decodeBody[providerSaveBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if err := s.saveProviderConfig(r.Context(), sess, body); err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"saved": true})
}

func (s *Server) handleConsoleProviderDelete(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	configID := r.PathValue("id")
	found, err := s.repo.DeleteProviderConfig(db.WithOrg(r.Context(), sess.OrgID), configID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !found {
		s.respondError(w, r, apperr.NotFound("provider config not found"))
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "settings.provider.saved",
		TargetType: "provider_config", TargetID: &configID, Payload: map[string]any{"deleted": true},
	})
	writeData(w, http.StatusOK, map[string]any{"deleted": true})
}

type consoleWidgetSaveBody struct {
	Origins      []string `json:"origins"`
	Enabled      bool     `json:"enabled"`
	DisplayName  string   `json:"displayName"`
	Greeting     string   `json:"greeting"`
	PrimaryColor string   `json:"primaryColor"`
	Position     string   `json:"position"`
}

func (s *Server) handleConsoleWidgetSave(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	body, err := decodeBody[consoleWidgetSaveBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if len(body.Origins) > 20 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Allow at most 20 origins.")
		return
	}
	origins := []string{}
	seen := map[string]bool{}
	for _, raw := range body.Origins {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		parsed, err := url.Parse(trimmed)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "Origins must be full URLs such as https://example.com.")
			return
		}
		origin := parsed.Scheme + "://" + parsed.Host
		if !seen[origin] {
			seen[origin] = true
			origins = append(origins, origin)
		}
	}
	body.DisplayName = strings.TrimSpace(body.DisplayName)
	body.Greeting = strings.TrimSpace(body.Greeting)
	if body.DisplayName == "" {
		if orgName, err := s.repo.OrgName(db.WithOrg(r.Context(), sess.OrgID)); err == nil {
			body.DisplayName = orgName
		}
	}
	if len(body.DisplayName) > 60 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Display name must be 60 characters or fewer.")
		return
	}
	if len(body.Greeting) > 160 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Greeting must be 160 characters or fewer.")
		return
	}
	if body.PrimaryColor == "" {
		body.PrimaryColor = "#d96c3f"
	}
	if !hexColor.MatchString(body.PrimaryColor) {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Use a six-digit hex colour such as #d96c3f.")
		return
	}
	position := "right"
	if body.Position == "left" {
		position = "left"
	}
	theme, err := json.Marshal(widgetTheme{
		DisplayName: body.DisplayName, Greeting: body.Greeting, PrimaryColor: body.PrimaryColor, Position: position,
	})
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if err := s.repo.UpsertWidgetConfigWithEnabled(db.WithOrg(r.Context(), sess.OrgID), origins, theme, body.Enabled); err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "settings.widget.updated",
		TargetType: "widget_config", Payload: map[string]any{"origins": origins, "enabled": body.Enabled},
	})
	writeData(w, http.StatusOK, map[string]any{"saved": true, "origins": origins, "enabled": body.Enabled})
}

type consoleAgentSaveBody struct {
	Purpose         string            `json:"purpose"`
	PublicScope     bool              `json:"publicScope"`
	InternalScope   bool              `json:"internalScope"`
	Tools           map[string]string `json:"tools"`
	Keywords        string            `json:"keywords"`
	Threshold       float64           `json:"threshold"`
	ProviderID      string            `json:"providerId"`
	ModelID         string            `json:"modelId"`
	MaxSteps        int               `json:"maxSteps"`
	MaxToolCalls    int               `json:"maxToolCalls"`
	MaxInputTokens  int               `json:"maxInputTokens"`
	MaxOutputTokens int               `json:"maxOutputTokens"`
}

func consoleBoundedInt(value, min, max int) (int, bool) {
	if value < min || value > max {
		return 0, false
	}
	return value, true
}

func (s *Server) handleConsoleAgentSave(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	body, err := decodeBody[consoleAgentSaveBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	body.Purpose = strings.TrimSpace(body.Purpose)
	if body.Purpose == "" || len(body.Purpose) > 4000 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Describe what the agent is for in 1 to 4000 characters.")
		return
	}
	scopes := []string{}
	if body.PublicScope {
		scopes = append(scopes, "public")
	}
	if body.InternalScope {
		scopes = append(scopes, "internal")
	}
	if len(scopes) == 0 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Enable at least one knowledge scope.")
		return
	}
	if body.Threshold < 0 || body.Threshold > 1 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "The low-confidence threshold must be between 0 and 1.")
		return
	}
	if body.ProviderID != "" && !model.KnownProvider(body.ProviderID) {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Choose a known provider or leave the model on auto.")
		return
	}
	maxSteps, stepOK := consoleBoundedInt(body.MaxSteps, 1, 12)
	maxToolCalls, toolOK := consoleBoundedInt(body.MaxToolCalls, 0, 12)
	maxInput, inputOK := consoleBoundedInt(body.MaxInputTokens, 500, 64000)
	maxOutput, outputOK := consoleBoundedInt(body.MaxOutputTokens, 100, 16000)
	if !stepOK || !toolOK || !inputOK || !outputOK {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Budgets are outside the allowed ranges.")
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
	keywords := []string{}
	seen := map[string]bool{}
	for _, raw := range strings.Split(body.Keywords, ",") {
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
	toolModes := []agent.PolicyTool{}
	for _, toolID := range []string{"knowledge.search", "customers.lookup", "conversations.add_note", "escalate"} {
		mode := body.Tools[toolID]
		switch mode {
		case "autonomous", "approval_required", "disabled":
		default:
			mode = "disabled"
		}
		toolModes = append(toolModes, agent.PolicyTool{ToolID: toolID, Mode: mode})
	}
	policy.Purpose = body.Purpose
	policy.KnowledgeScopes = scopes
	policy.Tools = toolModes
	policy.Escalation.LowConfidenceThreshold = body.Threshold
	policy.Escalation.Keywords = keywords
	policy.Model.ProviderID = body.ProviderID
	policy.Model.ModelID = strings.TrimSpace(body.ModelID)
	if len(policy.Model.ModelID) > 200 {
		policy.Model.ModelID = policy.Model.ModelID[:200]
	}
	policy.Budgets = agent.BudgetPolicy{
		MaxSteps: maxSteps, MaxToolCalls: maxToolCalls, MaxInputTokens: maxInput, MaxOutputTokens: maxOutput,
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
	writeData(w, http.StatusOK, map[string]any{"saved": true})
}

type consoleAPIKeyBody struct {
	Name string `json:"name"`
}

func (s *Server) handleConsoleAPIKeyCreate(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	body, err := decodeBody[consoleAPIKeyBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 80 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Give the key a name between 1 and 80 characters.")
		return
	}
	secret := "cd_" + id.Token(32)
	sum := sha256.Sum256([]byte(secret))
	keyID, err := s.repo.CreateAPIKey(db.WithOrg(r.Context(), sess.OrgID), body.Name, secret[:8], hex.EncodeToString(sum[:]))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "apikey.created",
		TargetType: "api_key", TargetID: &keyID, Payload: map[string]any{"name": body.Name, "prefix": secret[:8]},
	})
	w.Header().Set("Cache-Control", "no-store")
	writeData(w, http.StatusCreated, map[string]any{"secret": secret, "id": keyID})
}

func (s *Server) handleConsoleAPIKeyRevoke(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	keyID := r.PathValue("id")
	if !uuidShape.MatchString(keyID) {
		s.respondError(w, r, apperr.NotFound("API key not found"))
		return
	}
	found, err := s.repo.DeleteAPIKey(db.WithOrg(r.Context(), sess.OrgID), keyID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !found {
		s.respondError(w, r, apperr.NotFound("API key not found"))
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "apikey.revoked",
		TargetType: "api_key", TargetID: &keyID, Payload: map[string]any{},
	})
	writeData(w, http.StatusOK, map[string]any{"deleted": true})
}

type consoleWorkspaceBody struct {
	Name string `json:"name"`
}

func (s *Server) handleConsoleWorkspaceSave(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapOrgAdmin)
	if !ok {
		return
	}
	body, err := decodeBody[consoleWorkspaceBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 100 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Workspace name must be between 1 and 100 characters.")
		return
	}
	if err := s.repo.UpdateOrgName(db.WithOrg(r.Context(), sess.OrgID), body.Name); err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "org.updated",
		TargetType: "organization", TargetID: &sess.OrgID, Payload: map[string]any{"name": body.Name},
	})
	writeData(w, http.StatusOK, map[string]any{"saved": true})
}

type consoleAccountBody struct {
	Name            string `json:"name"`
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (s *Server) handleConsoleAccount(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	enabled := false
	if record, err := s.repo.TwoFactor(db.RepoCtx{Ctx: r.Context()}, sess.UserID); err == nil {
		enabled = record.Enabled
	}
	writeData(w, http.StatusOK, map[string]any{
		"id": sess.UserID, "name": sess.Name, "email": sess.Email, "role": string(sess.Role),
		"initial": consoleInitial(sess.Name), "twoFactorEnabled": enabled,
	})
}

func (s *Server) handleConsoleAccountSave(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	body, err := decodeBody[consoleAccountBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 100 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Your name must be between 1 and 100 characters.")
		return
	}
	if body.CurrentPassword != "" || body.NewPassword != "" {
		if len(body.NewPassword) < 8 || len(body.NewPassword) > 200 {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "The new password must be between 8 and 200 characters.")
			return
		}
		cred, err := s.repo.CredentialByEmail(r.Context(), sess.Email)
		if err != nil || !auth.VerifyPassword(body.CurrentPassword, cred.PasswordHash) {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "Current password did not match.")
			return
		}
		hash, err := auth.HashPassword(body.NewPassword)
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
	if err := s.repo.UpdateUserName(r.Context(), sess.UserID, body.Name); err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"saved": true})
}

type consoleInviteBody struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (s *Server) handleConsoleInviteCreate(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	body, err := decodeBody[consoleInviteBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	body.Email = strings.TrimSpace(body.Email)
	if !validEmail(body.Email) {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Enter the address of the teammate you want to invite.")
		return
	}
	if body.Role != "admin" && body.Role != "agent" {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Invite someone as an admin or an agent.")
		return
	}
	var exists bool
	if err := s.pool.QueryRow(r.Context(), `select exists(select 1 from "user" u join account a on a.user_id = u.id and a.provider_id = 'credential' where lower(u.email) = lower($1))`, body.Email).Scan(&exists); err != nil {
		s.respondError(w, r, err)
		return
	}
	if exists {
		writeErrorBody(w, http.StatusConflict, "conflict", "That email already has a sign-in account on this installation.")
		return
	}
	invitationID, err := s.repo.CreateInvitation(db.WithOrg(r.Context(), sess.OrgID), body.Email, body.Role, sess.UserID, time.Now().Add(7*24*time.Hour))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "user.invited",
		TargetType: "invitation", TargetID: &invitationID, Payload: map[string]any{"email": body.Email, "role": body.Role},
	})
	link := s.cfg.AppURL + "/invite/" + invitationID
	w.Header().Set("Cache-Control", "no-store")
	writeData(w, http.StatusCreated, map[string]any{
		"id": invitationID, "email": body.Email, "role": body.Role, "link": link,
	})
}

func (s *Server) handleConsoleInviteRevoke(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapSettingsWrite)
	if !ok {
		return
	}
	invitationID := r.PathValue("id")
	found, err := s.repo.RevokeInvitation(db.WithOrg(r.Context(), sess.OrgID), invitationID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !found {
		s.respondError(w, r, apperr.NotFound("invitation not found"))
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "user.invited",
		TargetType: "invitation", TargetID: &invitationID, Payload: map[string]any{"revoked": true},
	})
	writeData(w, http.StatusOK, map[string]any{"revoked": true})
}
