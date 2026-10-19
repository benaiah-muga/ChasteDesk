// SPDX-License-Identifier: AGPL-3.0-only
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const minSecretLen = 32

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

type Config struct {
	Port              int
	AppURL            string
	AppEnv            string
	DatabaseURL       string
	AppSecret         string
	LogLevel          string
	UploadsDir        string
	WorkerConcurrency int
}

func Load() (Config, error) {
	cfg := Config{
		Port:              3000,
		AppURL:            "http://localhost:3000",
		AppEnv:            "development",
		LogLevel:          "info",
		UploadsDir:        "data/uploads",
		WorkerConcurrency: 2,
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		AppSecret:         os.Getenv("APP_SECRET"),
	}
	if raw := os.Getenv("PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("config: invalid PORT %q", raw)
		}
		cfg.Port = port
	}
	if raw := os.Getenv("APP_URL"); raw != "" {
		cfg.AppURL = strings.TrimRight(raw, "/")
	}
	if raw := os.Getenv("LOG_LEVEL"); raw != "" {
		if !validLogLevels[raw] {
			return Config{}, fmt.Errorf("config: invalid LOG_LEVEL %q", raw)
		}
		cfg.LogLevel = raw
	}
	if raw := os.Getenv("APP_ENV"); raw != "" {
		switch raw {
		case "development", "test", "production":
			cfg.AppEnv = raw
		default:
			return Config{}, fmt.Errorf("config: invalid APP_ENV %q", raw)
		}
	}
	if raw := os.Getenv("UPLOADS_DIR"); raw != "" {
		cfg.UploadsDir = raw
	}
	if raw := os.Getenv("WORKER_CONCURRENCY"); raw != "" {
		concurrency, err := strconv.Atoi(raw)
		if err != nil || concurrency < 1 || concurrency > 64 {
			return Config{}, fmt.Errorf("config: invalid WORKER_CONCURRENCY %q", raw)
		}
		cfg.WorkerConcurrency = concurrency
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("config: DATABASE_URL is required")
	}
	if len(cfg.AppSecret) < minSecretLen {
		return Config{}, fmt.Errorf("config: APP_SECRET must be at least %d characters", minSecretLen)
	}
	return cfg, nil
}
