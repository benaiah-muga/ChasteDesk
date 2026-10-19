// SPDX-License-Identifier: AGPL-3.0-only
package model

import (
	"context"
	"errors"

	"chastedesk/internal/crypto"

	"github.com/jackc/pgx/v5"
)

type RowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func ResolveConfig(ctx context.Context, q RowQuerier, appSecret, orgID string, providerID ProviderID) (Config, error) {
	cfg := Config{}
	var keyCipher, keyIV, keyTag *string
	var keyVersion *int
	var baseURL *string
	err := q.QueryRow(ctx,
		`select key_cipher, key_iv, key_tag, key_version, base_url
		   from provider_configs
		  where org_id = $1 and provider_id = $2 and enabled = true`,
		orgID, string(providerID),
	).Scan(&keyCipher, &keyIV, &keyTag, &keyVersion, &baseURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if keyCipher != nil && keyIV != nil && keyTag != nil {
		version := 0
		if keyVersion != nil {
			version = *keyVersion
		}
		secret, err := crypto.OpenSecret(crypto.SealedSecret{
			Ciphertext: *keyCipher,
			IV:         *keyIV,
			AuthTag:    *keyTag,
			KeyVersion: version,
		}, appSecret)
		if err != nil {
			return cfg, err
		}
		cfg.APIKey = secret
	}
	if baseURL != nil {
		cfg.BaseURL = *baseURL
	}
	return cfg, nil
}
