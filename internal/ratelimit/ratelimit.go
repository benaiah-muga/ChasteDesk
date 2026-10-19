// SPDX-License-Identifier: AGPL-3.0-only
package ratelimit

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/apperr"
)

type Result struct {
	Count     int
	Remaining int
}

func Enforce(ctx context.Context, pool *pgxpool.Pool, key string, limit, windowSeconds int) (Result, error) {
	var count int
	err := pool.QueryRow(ctx, `
		insert into rate_limit_windows (key, window_start, count)
		values ($1, now(), 1)
		on conflict (key) do update set
			count = case
				when rate_limit_windows.window_start < now() - ($2 * interval '1 second') then 1
				else rate_limit_windows.count + 1
			end,
			window_start = case
				when rate_limit_windows.window_start < now() - ($2 * interval '1 second') then now()
				else rate_limit_windows.window_start
			end
		returning count`,
		key, windowSeconds).Scan(&count)
	if err != nil {
		return Result{}, err
	}
	if count > limit {
		return Result{Count: count}, apperr.RateLimited("too many requests, slow down")
	}
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}
	return Result{Count: count, Remaining: remaining}, nil
}
