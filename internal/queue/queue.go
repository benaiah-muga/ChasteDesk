// SPDX-License-Identifier: AGPL-3.0-only
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/id"
)

type Job struct {
	ID   string
	Name string
	Data json.RawMessage
}

type Handler func(ctx context.Context, job Job) error

type SendOptions struct {
	RetryLimit        int
	RetryDelaySeconds int
	SingletonKey      string
}

type Queue struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Queue {
	return &Queue{pool: pool}
}

func (q *Queue) Send(ctx context.Context, name string, data any, opts SendOptions) (*string, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("queue: marshal job data: %w", err)
	}
	jobID := id.UUID()
	var singletonKey *string
	if opts.SingletonKey != "" {
		singletonKey = &opts.SingletonKey
	}
	var inserted string
	err = q.pool.QueryRow(ctx, `insert into jobs (id, name, data, retry_limit, retry_delay_seconds, singleton_key)
		values ($1, $2, $3::jsonb, $4, $5, $6)
		on conflict (name, singleton_key) where singleton_key is not null and status in ('created', 'active', 'retry') do nothing
		returning id`, jobID, name, string(encoded), opts.RetryLimit, opts.RetryDelaySeconds, singletonKey).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inserted, nil
}

func (q *Queue) Work(ctx context.Context, name string, concurrency int, handler Handler) {
	if concurrency < 1 {
		concurrency = 1
	}
	for i := 0; i < concurrency; i++ {
		go q.workerLoop(ctx, name, handler)
	}
}

func (q *Queue) workerLoop(ctx context.Context, name string, handler Handler) {
	idle := 200 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		processed, err := q.runOne(ctx, name, handler)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("queue: worker error", "queue", name, "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		if !processed {
			select {
			case <-ctx.Done():
				return
			case <-time.After(idle):
			}
		}
	}
}

func truncateError(err error) string {
	message := err.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}

func (q *Queue) runOne(ctx context.Context, name string, handler Handler) (bool, error) {
	var job Job
	var retryLimit, retryCount, retryDelay int
	err := q.pool.QueryRow(ctx, `update jobs set status = 'active', started_at = now()
		where id = (
			select id from jobs
			where name = $1 and status in ('created', 'retry') and run_at <= now()
			order by run_at asc
			limit 1
			for update skip locked
		)
		returning id, name, data, retry_limit, retry_count, retry_delay_seconds`, name).
		Scan(&job.ID, &job.Name, &job.Data, &retryLimit, &retryCount, &retryDelay)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	handlerErr := handler(ctx, job)
	if handlerErr == nil {
		_, err = q.pool.Exec(ctx, `update jobs set status = 'completed', completed_at = now(), last_error = null where id = $1`, job.ID)
		return true, err
	}

	failedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if retryCount < retryLimit {
		runAt := time.Now().Add(time.Duration(retryDelay) * time.Second)
		_, err = q.pool.Exec(failedCtx, `update jobs set status = 'retry', retry_count = retry_count + 1, run_at = $2, last_error = $3 where id = $1`,
			job.ID, runAt, truncateError(handlerErr))
	} else {
		_, err = q.pool.Exec(failedCtx, `update jobs set status = 'failed', completed_at = now(), last_error = $2 where id = $1`,
			job.ID, truncateError(handlerErr))
	}
	if err != nil {
		return true, err
	}
	return true, nil
}
