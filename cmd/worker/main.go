// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"chastedesk/internal/config"
	"chastedesk/internal/logging"
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
	logger.Info("worker started")
	<-ctx.Done()
	logger.Info("worker stopped")
	return nil
}
