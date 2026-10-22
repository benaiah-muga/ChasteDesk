// SPDX-License-Identifier: AGPL-3.0-only
package observability

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type Notifier struct {
	url      string
	secret   string
	logger   *slog.Logger
	metrics  *Metrics
	client   *http.Client
	mu       sync.Mutex
	lastSent map[string]time.Time
	window   time.Duration
}

func NewNotifier(url, secret string, logger *slog.Logger, metrics *Metrics) *Notifier {
	if logger == nil {
		logger = slog.Default()
	}
	return &Notifier{
		url: url, secret: secret, logger: logger, metrics: metrics,
		client: &http.Client{Timeout: 4 * time.Second}, lastSent: map[string]time.Time{}, window: time.Minute,
	}
}

func (n *Notifier) Enabled() bool { return n != nil && n.url != "" }

func (n *Notifier) NotifyAsync(ctx context.Context, event, message string, fields map[string]any) {
	if !n.Enabled() {
		return
	}
	go n.notify(context.WithoutCancel(ctx), event, message, fields)
}

func (n *Notifier) notify(ctx context.Context, event, message string, fields map[string]any) {
	n.mu.Lock()
	now := time.Now()
	if last, ok := n.lastSent[event]; ok && now.Sub(last) < n.window {
		n.mu.Unlock()
		return
	}
	n.lastSent[event] = now
	n.mu.Unlock()

	payload := map[string]any{
		"source": "chastedesk", "event": event, "message": message,
		"timestamp": now.UTC().Format(time.RFC3339Nano),
	}
	for key, value := range fields {
		payload[key] = value
	}
	body, err := json.Marshal(payload)
	if err != nil {
		n.metricsAlert(false)
		return
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		n.metricsAlert(false)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "ChasteDesk-Alerts/1")
	if n.secret != "" {
		mac := hmac.New(sha256.New, []byte(n.secret))
		_, _ = mac.Write(body)
		request.Header.Set("X-ChasteDesk-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	response, err := n.client.Do(request)
	if err != nil {
		n.metricsAlert(false)
		n.logger.Warn("alert delivery failed", "event", event, "error", err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		n.metricsAlert(false)
		n.logger.Warn("alert webhook rejected", "event", event, "status", response.StatusCode)
		return
	}
	n.metricsAlert(true)
}

func (n *Notifier) metricsAlert(sent bool) {
	if n.metrics != nil {
		n.metrics.IncAlert(sent)
	}
}
