// SPDX-License-Identifier: AGPL-3.0-only
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

const keyInfo = "secrets-v1"
const KeyVersion = 1

var (
	ErrDecrypt       = errors.New("crypto: decryption failed")
	ErrInvalidSecret = errors.New("crypto: invalid sealed secret")
)

type SealedSecret struct {
	Ciphertext string
	IV         string
	AuthTag    string
	KeyVersion int
	Last4      string
}

func deriveKey(appSecret string) ([]byte, error) {
	return hkdf.Key(sha256.New, []byte(appSecret), nil, keyInfo, 32)
}

func DeriveSubkey(appSecret, info string) ([]byte, error) {
	return hkdf.Key(sha256.New, []byte(appSecret), nil, info, 32)
}

func SealSecret(plaintext, appSecret string) (SealedSecret, error) {
	key, err := deriveKey(appSecret)
	if err != nil {
		return SealedSecret{}, fmt.Errorf("crypto: derive key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return SealedSecret{}, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return SealedSecret{}, fmt.Errorf("crypto: new gcm: %w", err)
	}
	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		return SealedSecret{}, fmt.Errorf("crypto: random iv: %w", err)
	}
	sealed := gcm.Seal(nil, iv, []byte(plaintext), nil)
	tagStart := len(sealed) - gcm.Overhead()
	ciphertext := sealed[:tagStart]
	tag := sealed[tagStart:]
	return SealedSecret{
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		IV:         base64.StdEncoding.EncodeToString(iv),
		AuthTag:    base64.StdEncoding.EncodeToString(tag),
		KeyVersion: KeyVersion,
		Last4:      last4(plaintext),
	}, nil
}

func OpenSecret(sealed SealedSecret, appSecret string) (string, error) {
	key, err := deriveKey(appSecret)
	if err != nil {
		return "", fmt.Errorf("crypto: derive key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: new gcm: %w", err)
	}
	iv, err := base64.StdEncoding.DecodeString(sealed.IV)
	if err != nil || len(iv) != gcm.NonceSize() {
		return "", ErrInvalidSecret
	}
	ciphertext, err := base64.StdEncoding.DecodeString(sealed.Ciphertext)
	if err != nil {
		return "", ErrInvalidSecret
	}
	tag, err := base64.StdEncoding.DecodeString(sealed.AuthTag)
	if err != nil || len(tag) != gcm.Overhead() {
		return "", ErrInvalidSecret
	}
	plaintext, err := gcm.Open(nil, iv, append(ciphertext, tag...), nil)
	if err != nil {
		return "", ErrDecrypt
	}
	return string(plaintext), nil
}

func MaskSecret(plaintext string) string {
	if len(plaintext) <= 4 {
		return "****"
	}
	return plaintext[:3] + "..." + plaintext[len(plaintext)-4:]
}

func last4(plaintext string) string {
	if len(plaintext) <= 4 {
		return plaintext
	}
	return plaintext[len(plaintext)-4:]
}
