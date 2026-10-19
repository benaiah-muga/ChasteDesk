// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

type WidgetConfigRow struct {
	Origins []string        `json:"origins"`
	Enabled bool            `json:"enabled"`
	Theme   json.RawMessage `json:"theme"`
}

func (r *Repo) GetWidgetConfig(ctx RepoCtx) (*WidgetConfigRow, error) {
	row := WidgetConfigRow{}
	err := r.pool.QueryRow(ctx.Ctx, `select origins, enabled, theme from widget_configs where org_id = $1`, ctx.OrgID).
		Scan(&row.Origins, &row.Enabled, &row.Theme)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repo) UpsertWidgetConfig(ctx RepoCtx, origins []string, theme json.RawMessage) error {
	if theme == nil {
		theme = json.RawMessage(`{}`)
	}
	_, err := r.pool.Exec(ctx.Ctx, `insert into widget_configs (org_id, origins, theme)
		values ($1, $2, $3::jsonb)
		on conflict (org_id) do update set origins = excluded.origins, theme = excluded.theme, updated_at = now()`,
		ctx.OrgID, origins, string(theme))
	return err
}
