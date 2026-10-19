// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"strings"
	"testing"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("hash must be argon2id PHC string, got %q", hash)
	}
	if !VerifyPassword("correct horse battery staple", hash) {
		t.Error("correct password must verify")
	}
	if VerifyPassword("wrong password", hash) {
		t.Error("wrong password must not verify")
	}
	if VerifyPassword("correct horse battery staple", "not-a-hash") {
		t.Error("garbage hash must not verify")
	}
	second, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if hash == second {
		t.Error("hashes must use random salts")
	}
}

func TestRBACMatrix(t *testing.T) {
	all := []Capability{CapSettingsRead, CapSettingsWrite, CapKnowledgeRead, CapKnowledgeWrite, CapAuditRead, CapInboxUse, CapOrgAdmin}
	for _, cap := range all {
		if !Can(RoleOwner, cap) {
			t.Errorf("owner must have %s", cap)
		}
	}
	if !Can(RoleAdmin, CapSettingsWrite) || !Can(RoleAdmin, CapAuditRead) || !Can(RoleAdmin, CapKnowledgeWrite) || !Can(RoleAdmin, CapInboxUse) {
		t.Error("admin must have settings.write, knowledge.write, audit.read, inbox.use")
	}
	if Can(RoleAdmin, CapOrgAdmin) {
		t.Error("admin must not have org.admin")
	}
	if !Can(RoleAgent, CapKnowledgeRead) || !Can(RoleAgent, CapInboxUse) {
		t.Error("agent must have knowledge.read and inbox.use")
	}
	for _, cap := range []Capability{CapSettingsRead, CapSettingsWrite, CapKnowledgeWrite, CapAuditRead, CapOrgAdmin} {
		if Can(RoleAgent, cap) {
			t.Errorf("agent must not have %s", cap)
		}
	}
	if Can("unknown-role", CapInboxUse) {
		t.Error("unknown role must have no capabilities")
	}
}
