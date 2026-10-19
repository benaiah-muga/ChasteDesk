// SPDX-License-Identifier: AGPL-3.0-only
package crypto

import (
	"encoding/base64"
	"testing"
)

const appSecret = "unit-test-app-secret-material-0123456789"

func TestSealOpenRoundTrip(t *testing.T) {
	secret := "sk-live-abcdef1234567890"
	sealed, err := SealSecret(secret, appSecret)
	if err != nil {
		t.Fatalf("SealSecret: %v", err)
	}
	if sealed.KeyVersion != 1 {
		t.Errorf("key version = %d, want 1", sealed.KeyVersion)
	}
	if sealed.Last4 != "7890" {
		t.Errorf("last4 = %q, want 7890", sealed.Last4)
	}
	opened, err := OpenSecret(sealed, appSecret)
	if err != nil {
		t.Fatalf("OpenSecret: %v", err)
	}
	if opened != secret {
		t.Errorf("round-trip plaintext mismatch: %q", opened)
	}
}

func TestOpenWithWrongKeyFails(t *testing.T) {
	sealed, err := SealSecret("sk-live-abcdef1234567890", appSecret)
	if err != nil {
		t.Fatalf("SealSecret: %v", err)
	}
	if _, err := OpenSecret(sealed, "another-app-secret-entirely-0123456789"); err == nil {
		t.Error("decryption with wrong key must fail")
	}
}

func TestOpenTamperedCiphertextFails(t *testing.T) {
	sealed, err := SealSecret("sk-live-abcdef1234567890", appSecret)
	if err != nil {
		t.Fatalf("SealSecret: %v", err)
	}
	sealed.Ciphertext = base64.StdEncoding.EncodeToString([]byte("tampered-ciphertext-bytes"))
	if _, err := OpenSecret(sealed, appSecret); err == nil {
		t.Error("tampered ciphertext must fail authentication")
	}
}

func TestIVsAreRandomPerSeal(t *testing.T) {
	first, err := SealSecret("sk-live-abcdef1234567890", appSecret)
	if err != nil {
		t.Fatalf("SealSecret: %v", err)
	}
	second, err := SealSecret("sk-live-abcdef1234567890", appSecret)
	if err != nil {
		t.Fatalf("SealSecret: %v", err)
	}
	if first.Ciphertext == second.Ciphertext && first.IV == second.IV {
		t.Error("IVs must be random per seal")
	}
}

func TestMaskSecret(t *testing.T) {
	if got := MaskSecret("sk-live-abcdef1234567890"); got[len(got)-4:] != "7890" {
		t.Errorf("mask must end with last4, got %q", got)
	}
	if got := MaskSecret("abc"); got != "****" {
		t.Errorf("short secrets must be fully masked, got %q", got)
	}
}

func TestDeriveSubkeyIsDeterministic(t *testing.T) {
	a, err := DeriveSubkey(appSecret, "widget-org1")
	if err != nil {
		t.Fatalf("DeriveSubkey: %v", err)
	}
	b, err := DeriveSubkey(appSecret, "widget-org1")
	if err != nil {
		t.Fatalf("DeriveSubkey: %v", err)
	}
	if string(a) != string(b) {
		t.Error("subkey derivation must be deterministic")
	}
	c, err := DeriveSubkey(appSecret, "widget-org2")
	if err != nil {
		t.Fatalf("DeriveSubkey: %v", err)
	}
	if string(a) == string(c) {
		t.Error("subkeys for different orgs must differ")
	}
}
