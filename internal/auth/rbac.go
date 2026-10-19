// SPDX-License-Identifier: AGPL-3.0-only
package auth

type Role string

const (
	RoleOwner Role = "owner"
	RoleAdmin Role = "admin"
	RoleAgent Role = "agent"
)

type Capability string

const (
	CapSettingsRead   Capability = "settings.read"
	CapSettingsWrite  Capability = "settings.write"
	CapKnowledgeRead  Capability = "knowledge.read"
	CapKnowledgeWrite Capability = "knowledge.write"
	CapAuditRead      Capability = "audit.read"
	CapInboxUse       Capability = "inbox.use"
	CapOrgAdmin       Capability = "org.admin"
)

var matrix = map[Role]map[Capability]bool{
	RoleOwner: {
		CapSettingsRead: true, CapSettingsWrite: true,
		CapKnowledgeRead: true, CapKnowledgeWrite: true,
		CapAuditRead: true, CapInboxUse: true, CapOrgAdmin: true,
	},
	RoleAdmin: {
		CapSettingsRead: true, CapSettingsWrite: true,
		CapKnowledgeRead: true, CapKnowledgeWrite: true,
		CapAuditRead: true, CapInboxUse: true,
	},
	RoleAgent: {
		CapKnowledgeRead: true, CapInboxUse: true,
	},
}

func Can(role Role, capability Capability) bool {
	return matrix[role][capability]
}

type Session struct {
	SessionID string
	UserID    string
	Name      string
	Email     string
	OrgID     string
	Role      Role
}
