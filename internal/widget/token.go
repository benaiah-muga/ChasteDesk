// SPDX-License-Identifier: AGPL-3.0-only
package widget

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"time"

	"chastedesk/internal/crypto"
)

type TokenPayload struct {
	OrgID   string   `json:"orgId"`
	Origins []string `json:"origins"`
	Iat     int64    `json:"iat"`
	Exp     int64    `json:"exp"`
}

const tokenTTL = 30 * 24 * time.Hour

func widgetKey(appSecret, orgID string) ([]byte, error) {
	return crypto.DeriveSubkey(appSecret, "widget-"+orgID)
}

func SignToken(appSecret, orgID string, origins []string, now time.Time) (string, error) {
	key, err := widgetKey(appSecret, orgID)
	if err != nil {
		return "", err
	}
	payload := TokenPayload{
		OrgID:   orgID,
		Origins: origins,
		Iat:     now.Unix(),
		Exp:     now.Add(tokenTTL).Unix(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func VerifyToken(appSecret, token, orgID string) (*TokenPayload, error) {
	dot := -1
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			dot = i
			break
		}
	}
	if dot <= 0 {
		return nil, crypto.ErrInvalidSecret
	}
	body, sig := token[:dot], token[dot+1:]
	key, err := widgetKey(appSecret, orgID)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(body))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return nil, crypto.ErrInvalidSecret
	}
	decoded, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, crypto.ErrInvalidSecret
	}
	var payload TokenPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, crypto.ErrInvalidSecret
	}
	if payload.OrgID != orgID || time.Unix(payload.Exp, 0).Before(time.Now()) {
		return nil, crypto.ErrInvalidSecret
	}
	return &payload, nil
}
