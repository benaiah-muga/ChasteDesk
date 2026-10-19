// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"chastedesk/internal/config"
	"chastedesk/internal/httpapi"
	"chastedesk/internal/logging"
)

func main() {
	if err := run(); err != nil {
		slog.Error("web: fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg.LogLevel)
	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           httpapi.New(cfg, logger),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("web listening", "port", cfg.Port, "app_url", cfg.AppURL)
		errCh <- server.ListenAndServe()
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
