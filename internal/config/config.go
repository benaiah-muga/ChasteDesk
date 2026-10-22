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
	Port               int
	AppURL             string
	AppEnv             string
	DatabaseURL        string
	AppSecret          string
	LogLevel           string
	UploadsDir         string
	WorkerConcurrency  int
	EmbeddingProvider  string
	EmbeddingModel     string
	EmbeddingBaseURL   string
	EmbeddingAPIKey    string
	MetricsToken       string
	AlertWebhookURL    string
	AlertWebhookSecret string
}

func Load() (Config, error) {
	cfg := Config{
		Port:               3000,
		AppURL:             "http://localhost:3000",
		AppEnv:             "development",
		LogLevel:           "info",
		UploadsDir:         "data/uploads",
		WorkerConcurrency:  2,
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		AppSecret:          os.Getenv("APP_SECRET"),
		EmbeddingProvider:  strings.TrimSpace(os.Getenv("EMBEDDING_PROVIDER")),
		EmbeddingModel:     strings.TrimSpace(os.Getenv("EMBEDDING_MODEL")),
		EmbeddingBaseURL:   strings.TrimSpace(os.Getenv("EMBEDDING_BASE_URL")),
		EmbeddingAPIKey:    strings.TrimSpace(os.Getenv("EMBEDDING_API_KEY")),
		MetricsToken:       strings.TrimSpace(os.Getenv("METRICS_TOKEN")),
		AlertWebhookURL:    strings.TrimSpace(os.Getenv("ALERT_WEBHOOK_URL")),
		AlertWebhookSecret: strings.TrimSpace(os.Getenv("ALERT_WEBHOOK_SECRET")),
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
	if cfg.MetricsToken != "" && len(cfg.MetricsToken) < 16 {
		return Config{}, errors.New("config: METRICS_TOKEN must be at least 16 characters")
	}
	if cfg.AlertWebhookURL != "" && !strings.HasPrefix(cfg.AlertWebhookURL, "http://") && !strings.HasPrefix(cfg.AlertWebhookURL, "https://") {
		return Config{}, errors.New("config: ALERT_WEBHOOK_URL must be an http(s) URL")
	}
	if cfg.EmbeddingProvider != "" {
		if cfg.EmbeddingModel == "" {
			return Config{}, errors.New("config: EMBEDDING_MODEL is required when EMBEDDING_PROVIDER is set")
		}
		if cfg.EmbeddingProvider == "anthropic" {
			return Config{}, errors.New("config: anthropic does not provide embeddings")
		}
		if cfg.EmbeddingBaseURL != "" && !strings.HasPrefix(cfg.EmbeddingBaseURL, "http://") && !strings.HasPrefix(cfg.EmbeddingBaseURL, "https://") {
			return Config{}, errors.New("config: EMBEDDING_BASE_URL must be an http(s) URL")
		}
	}
	return cfg, nil
}
