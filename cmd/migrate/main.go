// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"chastedesk/internal/config"
	"chastedesk/internal/db"
	"chastedesk/internal/logging"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("migrate: config", "error", err)
		os.Exit(1)
	}
	logger := logging.New(cfg.LogLevel)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("migrate: connect", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		logger.Error("migrate: apply", "error", err)
		os.Exit(1)
	}
	logger.Info("migrations applied")
}
