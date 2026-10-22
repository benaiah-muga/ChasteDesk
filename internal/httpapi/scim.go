// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"chastedesk/internal/apperr"
	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/email"
	"chastedesk/internal/id"
)

type scimPrincipal struct {
	OrgID     string
	TokenID   string
	CreatedBy string
}

type scimPrincipalKey struct{}

func scimFromContext(ctx context.Context) scimPrincipal {
	principal, _ := ctx.Value(scimPrincipalKey{}).(scimPrincipal)
	return principal
}

func scimTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Server) requireSCIM(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="ChasteDesk SCIM"`)
			writeSCIMError(w, http.StatusUnauthorized, "unauthorized", "A SCIM bearer token is required.")
			return
		}
		orgID, tokenID, createdBy, err := s.repo.SCIMOrgForToken(r.Context(), scimTokenHash(parts[1]))
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="ChasteDesk SCIM"`)
			writeSCIMError(w, http.StatusUnauthorized, "unauthorized", "The SCIM bearer token is invalid or revoked.")
			return
		}
		s.repo.TouchSCIMToken(r.Context(), tokenID)
		next(w, r.WithContext(context.WithValue(r.Context(), scimPrincipalKey{}, scimPrincipal{OrgID: orgID, TokenID: tokenID, CreatedBy: createdBy})))
	}
}

func writeSCIM(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeSCIMError(w http.ResponseWriter, status int, scimType, detail string) {
	writeSCIM(w, status, map[string]any{
		"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
		"status":  strconv.Itoa(status), "scimType": scimType, "detail": detail,
	})
}

type scimName struct {
	Formatted  string `json:"formatted,omitempty"`
	GivenName  string `json:"givenName,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
}

type scimEmail struct {
	Value   string `json:"value"`
	Type    string `json:"type,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

type scimRole struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

type scimUserRequest struct {
	Schemas     []string    `json:"schemas"`
	ExternalID  string      `json:"externalId"`
	UserName    string      `json:"userName"`
	Name        scimName    `json:"name"`
	DisplayName string      `json:"displayName"`
	Emails      []scimEmail `json:"emails"`
	Active      *bool       `json:"active"`
	Roles       []scimRole  `json:"roles"`
}

func scimDisplayName(body scimUserRequest) string {
	if name := strings.TrimSpace(body.Name.Formatted); name != "" {
		return truncateRunes(name, 100)
	}
	if combined := strings.TrimSpace(strings.TrimSpace(body.Name.GivenName) + " " + strings.TrimSpace(body.Name.FamilyName)); combined != "" {
		return truncateRunes(combined, 100)
	}
	if name := strings.TrimSpace(body.DisplayName); name != "" {
		return truncateRunes(name, 100)
	}
	userName := strings.TrimSpace(body.UserName)
	if at := strings.IndexByte(userName, '@'); at > 0 {
		return userName[:at]
	}
	return "SCIM user"
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func scimRoleValue(roles []scimRole) (string, bool) {
	if len(roles) == 0 {
		return "agent", true
	}
	value := strings.ToLower(strings.TrimSpace(roles[0].Value))
	switch value {
	case "owner", "admin", "agent":
		return value, true
	default:
		return "", false
	}
}

func scimUserResource(baseURL string, user db.SCIMUserRow) map[string]any {
	resource := map[string]any{
		"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"id":          user.ID,
		"userName":    user.UserName,
		"displayName": user.DisplayName,
		"active":      user.Active,
		"name":        map[string]any{"formatted": user.DisplayName},
		"emails":      []map[string]any{{"value": user.UserName, "type": "work", "primary": true}},
		"roles":       []map[string]any{{"value": user.Role, "display": roleDisplay(user.Role), "primary": true}},
		"meta": map[string]any{
			"resourceType": "User", "location": baseURL + "/scim/v2/Users/" + user.ID,
			"created": user.CreatedAt.UTC().Format(time.RFC3339), "lastModified": user.UpdatedAt.UTC().Format(time.RFC3339),
		},
	}
	if user.ExternalID != nil && *user.ExternalID != "" {
		resource["externalId"] = *user.ExternalID
	}
	return resource
}

func roleDisplay(role string) string {
	switch role {
	case "owner":
		return "Owner"
	case "admin":
		return "Admin"
	default:
		return "Agent"
	}
}

func (s *Server) handleSCIMServiceProviderConfig(w http.ResponseWriter, r *http.Request) {
	writeSCIM(w, http.StatusOK, map[string]any{
		"schemas":          []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"documentationUri": s.cfg.AppURL + "/docs/API.md",
		"patch":            map[string]any{"supported": true},
		"bulk":             map[string]any{"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
		"filter":           map[string]any{"supported": true, "maxResults": 200},
		"changePassword":   map[string]any{"supported": false},
		"sort":             map[string]any{"supported": false},
		"etag":             map[string]any{"supported": false},
		"authenticationSchemes": []map[string]any{{
			"type": "oauthbearertoken", "name": "Bearer", "description": "Organization-scoped SCIM bearer token",
			"specUri": "https://www.rfc-editor.org/info/rfc6750", "primary": true,
		}},
	})
}

func (s *Server) handleSCIMResourceTypes(w http.ResponseWriter, r *http.Request) {
	base := s.cfg.AppURL
	writeSCIM(w, http.StatusOK, map[string]any{
		"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"totalResults": 1, "startIndex": 1, "itemsPerPage": 1,
		"Resources": []map[string]any{{
			"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ResourceType"},
			"id":      "User", "name": "User", "endpoint": "/Users",
			"schema": "urn:ietf:params:scim:schemas:core:2.0:User",
			"meta":   map[string]any{"location": base + "/scim/v2/ResourceTypes/User", "resourceType": "ResourceType"},
		}},
	})
}

var scimFilterPattern = regexp.MustCompile(`(?i)^\s*(username|externalid)\s+eq\s+"([^"]+)"\s*$`)

func (s *Server) handleSCIMListUsers(w http.ResponseWriter, r *http.Request) {
	principal := scimFromContext(r.Context())
	ctx := db.WithOrg(r.Context(), principal.OrgID)
	start, count := 1, 100
	if raw := r.URL.Query().Get("startIndex"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeSCIMError(w, http.StatusBadRequest, "invalidValue", "startIndex must be a positive integer.")
			return
		}
		start = parsed
	}
	if raw := r.URL.Query().Get("count"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeSCIMError(w, http.StatusBadRequest, "invalidValue", "count must be a positive integer.")
			return
		}
		count = parsed
	}
	if count > 200 {
		count = 200
	}
	var users []db.SCIMUserRow
	total := 0
	var err error
	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	if filter != "" {
		match := scimFilterPattern.FindStringSubmatch(filter)
		if len(match) != 3 {
			writeSCIMError(w, http.StatusBadRequest, "invalidFilter", "Only userName eq and externalId eq filters are supported.")
			return
		}
		var row db.SCIMUserRow
		var err error
		if strings.EqualFold(match[1], "userName") {
			row, err = s.repo.SCIMUserByUserName(ctx, match[2])
		} else {
			row, err = s.repo.SCIMUserByExternalID(ctx, match[2])
		}
		if err == nil {
			users = append(users, row)
			total = 1
		} else if !errors.Is(err, db.ErrNotFound) {
			s.respondError(w, r, err)
			return
		}
	} else {
		users, total, err = s.repo.ListSCIMUsers(ctx, start-1, count)
		if err != nil {
			s.respondError(w, r, err)
			return
		}
	}
	resources := make([]map[string]any, 0, len(users))
	for _, user := range users {
		resources = append(resources, scimUserResource(s.cfg.AppURL, user))
	}
	if filter != "" {
		if start > 1 {
			resources = []map[string]any{}
		}
	}
	writeSCIM(w, http.StatusOK, map[string]any{
		"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"totalResults": total, "startIndex": start, "itemsPerPage": len(resources), "Resources": resources,
	})
}

func (s *Server) handleSCIMGetUser(w http.ResponseWriter, r *http.Request) {
	principal := scimFromContext(r.Context())
	user, err := s.repo.SCIMUserByID(db.WithOrg(r.Context(), principal.OrgID), r.PathValue("id"))
	if errors.Is(err, db.ErrNotFound) {
		writeSCIMError(w, http.StatusNotFound, "notFound", "SCIM user not found.")
		return
	}
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	writeSCIM(w, http.StatusOK, scimUserResource(s.cfg.AppURL, user))
}

func (s *Server) handleSCIMCreateUser(w http.ResponseWriter, r *http.Request) {
	principal := scimFromContext(r.Context())
	body, err := scimDecode[scimUserRequest](r)
	if err != nil {
		writeSCIMError(w, http.StatusBadRequest, "invalidSyntax", "The SCIM user body is not valid JSON.")
		return
	}
	userName := strings.ToLower(strings.TrimSpace(body.UserName))
	if !validEmail(userName) {
		writeSCIMError(w, http.StatusBadRequest, "invalidValue", "userName must be a valid email address.")
		return
	}
	role, ok := scimRoleValue(body.Roles)
	if !ok {
		writeSCIMError(w, http.StatusBadRequest, "invalidValue", "roles must contain owner, admin, or agent.")
		return
	}
	displayName := scimDisplayName(body)
	user, err := s.repo.CreateSCIMUser(db.WithOrg(r.Context(), principal.OrgID), id.UUID(), userName, displayName, role, strings.TrimSpace(body.ExternalID), principal.CreatedBy)
	if errors.Is(err, db.ErrEmailExists) {
		writeSCIMError(w, http.StatusConflict, "uniqueness", "A user with that userName already exists.")
		return
	}
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	active := true
	if body.Active != nil {
		active = *body.Active
	}
	if !active {
		user, err = s.repo.UpdateSCIMUser(db.WithOrg(r.Context(), principal.OrgID), user.ID, db.SCIMUserUpdate{
			UserName: userName, DisplayName: displayName, Role: role, Active: false,
			ExternalID: strings.TrimSpace(body.ExternalID), InviterID: principal.CreatedBy,
		})
		if err != nil {
			s.respondError(w, r, err)
			return
		}
	}
	if user.Active && user.InvitationID != nil && s.cfg.SMTPHost != "" {
		if orgName, orgErr := s.repo.OrgName(db.WithOrg(r.Context(), principal.OrgID)); orgErr == nil {
			if _, mailErr := email.EnqueueInvitation(db.WithOrg(r.Context(), principal.OrgID), s.repo, orgName, userName, s.cfg.AppURL+"/invite/"+*user.InvitationID); mailErr != nil {
				s.logger.Warn("SCIM activation email enqueue failed", "error", mailErr)
			}
		}
	}
	s.scimAudit(r, principal.OrgID, "scim.user.created", user.ID, map[string]any{"role": role, "userName": userName})
	w.Header().Set("Location", s.cfg.AppURL+"/scim/v2/Users/"+user.ID)
	writeSCIM(w, http.StatusCreated, scimUserResource(s.cfg.AppURL, user))
}

func (s *Server) handleSCIMReplaceUser(w http.ResponseWriter, r *http.Request) {
	principal := scimFromContext(r.Context())
	ctx := db.WithOrg(r.Context(), principal.OrgID)
	existing, err := s.repo.SCIMUserByID(ctx, r.PathValue("id"))
	if errors.Is(err, db.ErrNotFound) {
		writeSCIMError(w, http.StatusNotFound, "notFound", "SCIM user not found.")
		return
	}
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	body, err := scimDecode[scimUserRequest](r)
	if err != nil {
		writeSCIMError(w, http.StatusBadRequest, "invalidSyntax", "The SCIM user body is not valid JSON.")
		return
	}
	userName := strings.ToLower(strings.TrimSpace(body.UserName))
	if !validEmail(userName) {
		writeSCIMError(w, http.StatusBadRequest, "invalidValue", "userName must be a valid email address.")
		return
	}
	role := existing.Role
	if len(body.Roles) > 0 {
		role, _ = scimRoleValue(body.Roles)
		if role == "" {
			writeSCIMError(w, http.StatusBadRequest, "invalidValue", "roles must contain owner, admin, or agent.")
			return
		}
	}
	active := existing.Active
	if body.Active != nil {
		active = *body.Active
	}
	if err := s.scimGuardLastOwner(ctx, existing, role, active); err != nil {
		writeSCIMError(w, http.StatusConflict, "mutability", err.Error())
		return
	}
	updated, err := s.repo.UpdateSCIMUser(ctx, existing.ID, db.SCIMUserUpdate{
		UserName: userName, DisplayName: scimDisplayName(body), Role: role, Active: active,
		ExternalID: strings.TrimSpace(body.ExternalID), InviterID: principal.CreatedBy,
	})
	if errors.Is(err, db.ErrEmailExists) {
		writeSCIMError(w, http.StatusConflict, "uniqueness", "A user with that userName already exists.")
		return
	}
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	s.scimAudit(r, principal.OrgID, "scim.user.updated", updated.ID, map[string]any{"role": updated.Role, "active": updated.Active})
	writeSCIM(w, http.StatusOK, scimUserResource(s.cfg.AppURL, updated))
}

type scimPatchBody struct {
	Schemas    []string             `json:"schemas"`
	Operations []scimPatchOperation `json:"Operations"`
}

type scimPatchOperation struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

func (s *Server) handleSCIMPatchUser(w http.ResponseWriter, r *http.Request) {
	principal := scimFromContext(r.Context())
	ctx := db.WithOrg(r.Context(), principal.OrgID)
	existing, err := s.repo.SCIMUserByID(ctx, r.PathValue("id"))
	if errors.Is(err, db.ErrNotFound) {
		writeSCIMError(w, http.StatusNotFound, "notFound", "SCIM user not found.")
		return
	}
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	body, err := scimDecode[scimPatchBody](r)
	if err != nil {
		writeSCIMError(w, http.StatusBadRequest, "invalidSyntax", "The SCIM PATCH body is not valid JSON.")
		return
	}
	update := db.SCIMUserUpdate{
		UserName: existing.UserName, DisplayName: existing.DisplayName, Role: existing.Role,
		Active: existing.Active, InviterID: principal.CreatedBy,
	}
	if existing.ExternalID != nil {
		update.ExternalID = *existing.ExternalID
	}
	givenName, familyName := "", ""
	for _, operation := range body.Operations {
		op := strings.ToLower(strings.TrimSpace(operation.Op))
		if op != "add" && op != "replace" && op != "remove" {
			writeSCIMError(w, http.StatusBadRequest, "invalidValue", "PATCH op must be add, replace, or remove.")
			return
		}
		path := strings.ToLower(strings.TrimSpace(operation.Path))
		if path == "" {
			var object map[string]json.RawMessage
			if err := json.Unmarshal(operation.Value, &object); err != nil {
				writeSCIMError(w, http.StatusBadRequest, "invalidValue", "A PATCH operation without a path needs an object value.")
				return
			}
			for key, value := range object {
				if err := applySCIMMapField(&update, &givenName, &familyName, strings.ToLower(key), value); err != nil {
					writeSCIMError(w, http.StatusBadRequest, "invalidPath", err.Error())
					return
				}
			}
			continue
		}
		if err := applySCIMOperation(&update, &givenName, &familyName, op, path, operation.Value); err != nil {
			writeSCIMError(w, http.StatusBadRequest, "invalidPath", err.Error())
			return
		}
	}
	if strings.TrimSpace(givenName+familyName) != "" {
		if name := strings.TrimSpace(givenName + " " + familyName); name != "" {
			update.DisplayName = truncateRunes(name, 100)
		}
	}
	if !validEmail(update.UserName) {
		writeSCIMError(w, http.StatusBadRequest, "invalidValue", "userName must be a valid email address.")
		return
	}
	if err := s.scimGuardLastOwner(ctx, existing, update.Role, update.Active); err != nil {
		writeSCIMError(w, http.StatusConflict, "mutability", err.Error())
		return
	}
	updated, err := s.repo.UpdateSCIMUser(ctx, existing.ID, update)
	if errors.Is(err, db.ErrEmailExists) {
		writeSCIMError(w, http.StatusConflict, "uniqueness", "A user with that userName already exists.")
		return
	}
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	s.scimAudit(r, principal.OrgID, "scim.user.updated", updated.ID, map[string]any{"role": updated.Role, "active": updated.Active})
	writeSCIM(w, http.StatusOK, scimUserResource(s.cfg.AppURL, updated))
}

func applySCIMMapField(update *db.SCIMUserUpdate, givenName, familyName *string, key string, raw json.RawMessage) error {
	switch key {
	case "username":
		return json.Unmarshal(raw, &update.UserName)
	case "displayname":
		return json.Unmarshal(raw, &update.DisplayName)
	case "active":
		return scimParseBool(raw, &update.Active)
	case "externalid":
		return json.Unmarshal(raw, &update.ExternalID)
	case "name":
		var name scimName
		if err := json.Unmarshal(raw, &name); err != nil {
			return err
		}
		if name.Formatted != "" {
			update.DisplayName = name.Formatted
		}
		*givenName, *familyName = name.GivenName, name.FamilyName
		return nil
	case "roles":
		role, ok := scimRoleFromRaw(raw)
		if !ok {
			return fmt.Errorf("roles must contain owner, admin, or agent")
		}
		update.Role = role
		return nil
	case "emails":
		var emails []scimEmail
		if err := json.Unmarshal(raw, &emails); err != nil || len(emails) == 0 {
			return fmt.Errorf("emails must contain at least one value")
		}
		update.UserName = strings.ToLower(strings.TrimSpace(emails[0].Value))
		return nil
	default:
		return fmt.Errorf("unsupported PATCH field %s", key)
	}
}

func applySCIMOperation(update *db.SCIMUserUpdate, givenName, familyName *string, op, path string, raw json.RawMessage) error {
	path = strings.TrimPrefix(path, "urn:ietf:params:scim:schemas:core:2.0:user:")
	switch path {
	case "active":
		if op == "remove" {
			update.Active = false
			return nil
		}
		return scimParseBool(raw, &update.Active)
	case "username":
		if op == "remove" {
			return fmt.Errorf("userName cannot be removed")
		}
		return json.Unmarshal(raw, &update.UserName)
	case "displayname":
		if op == "remove" {
			update.DisplayName = ""
			return nil
		}
		return json.Unmarshal(raw, &update.DisplayName)
	case "externalid":
		if op == "remove" {
			update.ExternalID = ""
			return nil
		}
		return json.Unmarshal(raw, &update.ExternalID)
	case "name":
		var name scimName
		if err := json.Unmarshal(raw, &name); err != nil {
			return err
		}
		if name.Formatted != "" {
			update.DisplayName = name.Formatted
		}
		*givenName, *familyName = name.GivenName, name.FamilyName
		return nil
	case "name.formatted":
		if op == "remove" {
			update.DisplayName = ""
			return nil
		}
		return json.Unmarshal(raw, &update.DisplayName)
	case "name.givenname":
		if op == "remove" {
			*givenName = ""
			return nil
		}
		if err := json.Unmarshal(raw, givenName); err != nil {
			return err
		}
		update.DisplayName = strings.TrimSpace(*givenName + " " + *familyName)
		return nil
	case "name.familyname":
		if op == "remove" {
			*familyName = ""
			return nil
		}
		if err := json.Unmarshal(raw, familyName); err != nil {
			return err
		}
		update.DisplayName = strings.TrimSpace(*givenName + " " + *familyName)
		return nil
	case "roles":
		if op == "remove" {
			update.Role = "agent"
			return nil
		}
		role, ok := scimRoleFromRaw(raw)
		if !ok {
			return fmt.Errorf("roles must contain owner, admin, or agent")
		}
		update.Role = role
		return nil
	case "emails":
		if op == "remove" {
			return fmt.Errorf("emails cannot be removed")
		}
		var emails []scimEmail
		if err := json.Unmarshal(raw, &emails); err != nil || len(emails) == 0 {
			return fmt.Errorf("emails must contain at least one value")
		}
		update.UserName = strings.ToLower(strings.TrimSpace(emails[0].Value))
		return nil
	default:
		return fmt.Errorf("unsupported PATCH path %s", path)
	}
}

func scimParseBool(raw json.RawMessage, target *bool) error {
	var value bool
	if err := json.Unmarshal(raw, &value); err == nil {
		*target = value
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return fmt.Errorf("active must be a boolean")
	}
	switch strings.ToLower(text) {
	case "true":
		*target = true
	case "false":
		*target = false
	default:
		return fmt.Errorf("active must be a boolean")
	}
	return nil
}

func scimRoleFromRaw(raw json.RawMessage) (string, bool) {
	var roles []scimRole
	if err := json.Unmarshal(raw, &roles); err == nil {
		return scimRoleValue(roles)
	}
	var single scimRole
	if err := json.Unmarshal(raw, &single); err == nil {
		return scimRoleValue([]scimRole{single})
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return scimRoleValue([]scimRole{{Value: value}})
	}
	return "", false
}

func (s *Server) handleSCIMDeleteUser(w http.ResponseWriter, r *http.Request) {
	principal := scimFromContext(r.Context())
	ctx := db.WithOrg(r.Context(), principal.OrgID)
	existing, err := s.repo.SCIMUserByID(ctx, r.PathValue("id"))
	if errors.Is(err, db.ErrNotFound) {
		writeSCIMError(w, http.StatusNotFound, "notFound", "SCIM user not found.")
		return
	}
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if existing.Active && existing.Role == "owner" {
		owners, err := s.repo.CountOwners(ctx)
		if err != nil {
			s.respondError(w, r, err)
			return
		}
		if owners <= 1 {
			writeSCIMError(w, http.StatusConflict, "mutability", "The last owner cannot be deactivated.")
			return
		}
	}
	if existing.Active {
		externalID := ""
		if existing.ExternalID != nil {
			externalID = *existing.ExternalID
		}
		if _, err := s.repo.UpdateSCIMUser(ctx, existing.ID, db.SCIMUserUpdate{
			UserName: existing.UserName, DisplayName: existing.DisplayName, Role: existing.Role,
			Active: false, ExternalID: externalID, InviterID: principal.CreatedBy,
		}); err != nil {
			s.respondError(w, r, err)
			return
		}
		s.scimAudit(r, principal.OrgID, "scim.user.deactivated", existing.ID, map[string]any{"userName": existing.UserName})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) scimGuardLastOwner(ctx db.RepoCtx, existing db.SCIMUserRow, role string, active bool) error {
	if existing.Role == "owner" && (role != "owner" || !active) {
		owners, err := s.repo.CountOwners(ctx)
		if err != nil {
			return err
		}
		if owners <= 1 {
			return errors.New("the last owner cannot be deactivated or demoted")
		}
	}
	return nil
}

func (s *Server) scimAudit(r *http.Request, orgID, eventType, targetID string, payload map[string]any) {
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), orgID), db.AuditInput{
		ActorType: db.ActorSystem, Type: eventType, TargetType: "scim_user",
		TargetID: &targetID, Payload: payload,
	})
}

func scimDecode[T any](r *http.Request) (T, error) {
	var out T
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err := dec.Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

type scimTokenCreateBody struct {
	Name string `json:"name"`
}

func (s *Server) handleConsoleSCIMTokenCreate(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapOrgAdmin)
	if !ok {
		return
	}
	body, err := decodeBody[scimTokenCreateBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 80 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Give the SCIM token a name between 1 and 80 characters.")
		return
	}
	token := "cd_scim_" + id.Token(32)
	tokenID, err := s.repo.CreateSCIMToken(db.WithOrg(r.Context(), sess.OrgID), body.Name, scimTokenHash(token), sess.UserID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "scim.token.created",
		TargetType: "scim_token", TargetID: &tokenID, Payload: map[string]any{"name": body.Name},
	})
	w.Header().Set("Cache-Control", "no-store")
	writeData(w, http.StatusCreated, map[string]any{
		"id": tokenID, "name": body.Name, "token": token, "baseUrl": s.cfg.AppURL + "/scim/v2",
	})
}

func (s *Server) handleConsoleSCIMTokenRevoke(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapOrgAdmin)
	if !ok {
		return
	}
	tokenID := r.PathValue("id")
	found, err := s.repo.RevokeSCIMToken(db.WithOrg(r.Context(), sess.OrgID), tokenID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !found {
		s.respondError(w, r, apperr.NotFound("SCIM token not found."))
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "scim.token.revoked",
		TargetType: "scim_token", TargetID: &tokenID, Payload: map[string]any{},
	})
	writeData(w, http.StatusOK, map[string]any{"revoked": true})
}
