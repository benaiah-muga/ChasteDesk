// SPDX-License-Identifier: AGPL-3.0-only
package model

import "strings"

func RedactSecrets(text string, secrets []string) string {
	out := text
	for _, secret := range secrets {
		if len(secret) < 4 {
			continue
		}
		out = strings.ReplaceAll(out, secret, "[redacted]")
	}
	return out
}

type SecretGuard struct {
	secrets []string
}

func NewSecretGuard(secrets []string) *SecretGuard {
	return &SecretGuard{secrets: append([]string(nil), secrets...)}
}

func (g *SecretGuard) Redact(text string) string {
	return RedactSecrets(text, g.secrets)
}

func (g *SecretGuard) ContainsSecret(text string) bool {
	for _, secret := range g.secrets {
		if len(secret) >= 4 && strings.Contains(text, secret) {
			return true
		}
	}
	return false
}
