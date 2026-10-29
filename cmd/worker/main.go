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
	"chastedesk/internal/email"
	"chastedesk/internal/ingest"
	"chastedesk/internal/logging"
	"chastedesk/internal/model"
	"chastedesk/internal/observability"
	"chastedesk/internal/queue"
	"chastedesk/internal/retrieval"
	"chastedesk/internal/sla"
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
	logger := logging.New(cfg.LogLevel, cfg.AppSecret, cfg.SMTPPassword, cfg.EmbeddingAPIKey, cfg.AlertWebhookSecret, cfg.MetricsToken)
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
	notifier := observability.NewNotifier(cfg.AlertWebhookURL, cfg.AlertWebhookSecret, logger, nil)
	withAlerts := func(name string, handler queue.Handler) queue.Handler {
		return func(ctx context.Context, job queue.Job) error {
			err := handler(ctx, job)
			if err != nil && ctx.Err() == nil {
				notifier.NotifyAsync(ctx, "job.failed:"+name, "Background job failed", map[string]any{
					"queue": name, "jobId": job.ID,
				})
			}
			return err
		}
	}
	var embedder *model.EmbeddingClient
	var queryEmbedder agent.QueryEmbedder
	if cfg.EmbeddingProvider != "" {
		embedder, err = model.NewEmbeddingClient(cfg.EmbeddingProvider, cfg.EmbeddingBaseURL, cfg.EmbeddingAPIKey, cfg.EmbeddingModel)
		if err != nil {
			return err
		}
		queryEmbedder = embedder
		logger.Info("embeddings enabled", "provider", cfg.EmbeddingProvider, "model", cfg.EmbeddingModel)
	}
	handlers := ingest.Handlers(ingest.Deps{Pool: pool, UploadsDir: cfg.UploadsDir, Embedder: embedder})
	names := make([]string, 0, len(handlers))
	for name, handler := range handlers {
		jobs.Work(ctx, name, cfg.WorkerConcurrency, withAlerts(name, handler))
		names = append(names, name)
	}
	engine := retrieval.NewEngine(pool)
	jobs.Work(ctx, "execute-action", cfg.WorkerConcurrency, withAlerts("execute-action", agent.ExecuteActionHandler(pool, engine)))
	jobs.Work(ctx, "ai-respond", cfg.WorkerConcurrency, withAlerts("ai-respond", agent.RespondHandler(pool, cfg.AppSecret, engine, queryEmbedder)))
	jobs.Work(ctx, "analytics-rollup", 1, withAlerts("analytics-rollup", analytics.Handler(pool)))
	names = append(names, "execute-action", "ai-respond", "analytics-rollup")
	go sla.StartMonitor(ctx, pool, notifier, logger)
	mailer := email.NewSender(email.SenderConfig{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword, From: cfg.SMTPFrom, TLS: cfg.SMTPTLS,
	})
	go email.StartDispatcher(ctx, db.NewRepo(pool), mailer, logger)
	if mailer.Enabled() {
		logger.Info("email delivery enabled", "from", mailer.From())
	}
	logger.Info("worker started", "queues", names)
	<-ctx.Done()
	logger.Info("worker stopped")
	return nil
}
