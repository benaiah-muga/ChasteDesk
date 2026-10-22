// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	totpPeriod       = 30
	totpDigits       = 6
	recoveryAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

func GenerateTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("auth: totp secret: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

func TOTPCode(secret string, at time.Time) (string, error) {
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(normalized)
	if err != nil {
		return "", fmt.Errorf("auth: decode totp secret: %w", err)
	}
	counter := uint64(at.Unix() / totpPeriod)
	return hotp(key, counter), nil
}

func ValidateTOTP(secret, code string, at time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(normalized)
	if err != nil {
		return false
	}
	counter := at.Unix() / totpPeriod
	for skew := int64(-1); skew <= 1; skew++ {
		if counter+skew < 0 {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(hotp(key, uint64(counter+skew))), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

func hotp(key []byte, counter uint64) string {
	buffer := make([]byte, 8)
	binary.BigEndian.PutUint64(buffer, counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(buffer)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	return fmt.Sprintf("%0*d", totpDigits, value%1_000_000)
}

func TOTPURI(email, secret string) string {
	label := url.PathEscape("ChasteDesk:" + strings.TrimSpace(email))
	query := url.Values{}
	query.Set("secret", secret)
	query.Set("issuer", "ChasteDesk")
	query.Set("algorithm", "SHA1")
	query.Set("digits", strconv.Itoa(totpDigits))
	query.Set("period", strconv.Itoa(totpPeriod))
	return "otpauth://totp/" + label + "?" + query.Encode()
}

func GenerateRecoveryCodes() ([]string, error) {
	codes := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		raw := make([]byte, 10)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("auth: recovery code: %w", err)
		}
		chars := make([]byte, len(raw))
		for j, value := range raw {
			chars[j] = recoveryAlphabet[int(value)%len(recoveryAlphabet)]
		}
		codes = append(codes, string(chars[:5])+"-"+string(chars[5:]))
	}
	return codes, nil
}

func NormalizeRecoveryCode(code string) string {
	var builder strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(code)) {
		if (r >= 'A' && r <= 'Z') || (r >= '2' && r <= '9') {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func HashRecoveryCode(code string) string {
	normalized := NormalizeRecoveryCode(code)
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
