// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chastedesk/internal/agent"
	"chastedesk/internal/analytics"
	"chastedesk/internal/config"
	"chastedesk/internal/db"
	"chastedesk/internal/ingest"
	"chastedesk/internal/logging"
	"chastedesk/internal/queue"
	"chastedesk/internal/retrieval"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker: fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pool, err := db.Open(connectCtx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	jobs := queue.New(pool)
	handlers := ingest.Handlers(ingest.Deps{Pool: pool, UploadsDir: cfg.UploadsDir})
	names := make([]string, 0, len(handlers))
	for name, handler := range handlers {
		jobs.Work(ctx, name, cfg.WorkerConcurrency, handler)
		names = append(names, name)
	}
	engine := retrieval.NewEngine(pool)
	jobs.Work(ctx, "execute-action", cfg.WorkerConcurrency, agent.ExecuteActionHandler(pool, engine))
	jobs.Work(ctx, "ai-respond", cfg.WorkerConcurrency, agent.RespondHandler(pool, cfg.AppSecret, engine))
	jobs.Work(ctx, "analytics-rollup", 1, analytics.Handler(pool))
	names = append(names, "execute-action", "ai-respond", "analytics-rollup")
	logger.Info("worker started", "queues", names)
	<-ctx.Done()
	logger.Info("worker stopped")
	return nil
}
