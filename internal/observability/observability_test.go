// SPDX-License-Identifier: AGPL-3.0-only
package observability

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestMiddlewareRequestIDAndMetrics(t *testing.T) {
	metrics := NewMetrics()
	middleware := &Middleware{Metrics: metrics, Logger: testLogger()}
	server := httptest.NewServer(middleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed"))
	})))
	defer server.Close()

	response, err := http.Get(server.URL + "/broken")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer response.Body.Close()
	requestID := response.Header.Get("X-Request-ID")
	if len(requestID) < 8 {
		t.Fatalf("request id missing: %q", requestID)
	}
	body := metrics.Render(context.Background(), nil)
	if !strings.Contains(body, `chastedesk_http_requests_total{method="GET",route="unmatched",status="500"} 1`) {
		t.Fatalf("request metric missing:\n%s", body)
	}
	if !strings.Contains(body, "chastedesk_http_request_duration_seconds_count") {
		t.Fatalf("duration metric missing:\n%s", body)
	}
}

func TestNotifierSignsPayload(t *testing.T) {
	received := make(chan *http.Request, 1)
	bodyCh := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodyCh <- body
		received <- r
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	notifier := NewNotifier(server.URL, "alert-secret", testLogger(), nil)
	notifier.NotifyAsync(context.Background(), "test.event", "test alert", map[string]any{"requestId": "abc12345"})
	select {
	case request := <-received:
		body := <-bodyCh
		if !strings.Contains(string(body), `"requestId":"abc12345"`) {
			t.Fatalf("alert body missing field: %s", body)
		}
		mac := hmac.New(sha256.New, []byte("alert-secret"))
		_, _ = mac.Write(body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if request.Header.Get("X-ChasteDesk-Signature") != expected {
			t.Fatalf("signature mismatch: %s != %s", request.Header.Get("X-ChasteDesk-Signature"), expected)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("alert was not delivered")
	}
}
