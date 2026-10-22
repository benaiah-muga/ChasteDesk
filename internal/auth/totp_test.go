// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"testing"
	"time"
)

func TestTOTPCodeAndWindow(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	at := time.Unix(59, 0)
	code, err := TOTPCode(secret, at)
	if err != nil {
		t.Fatalf("code: %v", err)
	}
	if code != "287082" {
		t.Fatalf("unexpected RFC code: %s", code)
	}
	if !ValidateTOTP(secret, code, at) {
		t.Fatal("current code must validate")
	}
	if ValidateTOTP(secret, "000000", at) {
		t.Fatal("wrong code must not validate")
	}
}

func TestRecoveryCodesNormalizeAndHash(t *testing.T) {
	codes, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("codes: %v", err)
	}
	if len(codes) != 10 {
		t.Fatalf("expected 10 codes, got %d", len(codes))
	}
	if HashRecoveryCode(" abcde-fghij ") != HashRecoveryCode("ABCDEFGHIJ") {
		t.Fatal("recovery normalization must ignore case and separators")
	}
}
