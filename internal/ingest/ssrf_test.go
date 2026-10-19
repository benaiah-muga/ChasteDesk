// SPDX-License-Identifier: AGPL-3.0-only
package ingest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func lookupReturning(ip string, family int) LookupFunc {
	return func(host string) ([]Addr, error) {
		return []Addr{{Address: ip, Family: family}}, nil
	}
}

func familyOf(ip string) int {
	if strings.Contains(ip, ":") {
		return 6
	}
	return 4
}

func TestSSRFGuardBlocksPrivateTargets(t *testing.T) {
	blocked := [][2]string{
		{"http://127.0.0.1/x", "127.0.0.1"},
		{"http://localhost/x", "127.0.0.1"},
		{"http://10.1.2.3/x", "10.1.2.3"},
		{"http://172.16.3.4/x", "172.16.3.4"},
		{"http://192.168.1.1/x", "192.168.1.1"},
		{"http://169.254.169.254/latest/meta-data/", "169.254.169.254"},
		{"http://100.64.0.1/x", "100.64.0.1"},
		{"http://0.0.0.0/x", "0.0.0.0"},
		{"http://[::1]/x", "::1"},
		{"http://[fd00::1]/x", "fd00::1"},
		{"http://[fe80::1]/x", "fe80::1"},
		{"http://[::ffff:127.0.0.1]/x", "::ffff:127.0.0.1"},
	}
	for _, entry := range blocked {
		rawURL, ip := entry[0], entry[1]
		if !IsPrivateAddress(ip) {
			t.Errorf("IsPrivateAddress must flag %s", ip)
		}
		_, err := ValidateIngestURL(rawURL, AddressPolicy{AllowPrivate: false}, lookupReturning(ip, familyOf(ip)))
		var blockedErr *BlockedAddressError
		if !errors.As(err, &blockedErr) {
			t.Errorf("crawler did not block %s (err=%v)", rawURL, err)
		}
	}
}

func TestSSRFGuardAllowsPublicTarget(t *testing.T) {
	const publicIP = "93.184.215.14"
	target, err := ValidateIngestURL("http://"+publicIP+"/x", AddressPolicy{AllowPrivate: false}, lookupReturning(publicIP, 4))
	if err != nil {
		t.Fatalf("crawler must allow a public address: %v", err)
	}
	if target.PinnedIP != publicIP {
		t.Fatalf("pinned IP = %q, want %q", target.PinnedIP, publicIP)
	}
}

func TestAllowPrivateIngestionPolicy(t *testing.T) {
	t.Setenv("CHASTESK_INGEST_ALLOW_PRIVATE", "1")
	if !AllowPrivateIngestion("") {
		t.Error("test escape hatch must enable private ingestion outside production")
	}
	if AllowPrivateIngestion("production") {
		t.Error("escape hatch must be inert when APP_ENV=production")
	}
	t.Setenv("CHASTESK_INGEST_ALLOW_PRIVATE", "")
	if AllowPrivateIngestion("") {
		t.Error("private ingestion must default to denied")
	}
}

func TestCrawlerLocalFixtureAndRedirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/redirect-private", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://0.0.0.0:9/meta-data")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/redirect-ok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/final")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><head><title>Local Fixture</title></head><body><h1>Fixture</h1><p>reachable</p></body></html>"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	t.Setenv("CHASTESK_INGEST_ALLOW_PRIVATE", "1")
	t.Setenv("APP_ENV", "test")
	ctx := context.Background()

	ok1, err := Crawl(ctx, server.URL+"/")
	if err != nil {
		t.Fatalf("crawl of allowed local target failed: %v", err)
	}
	if ok1.Title != "Local Fixture" {
		t.Errorf("title = %q, want Local Fixture", ok1.Title)
	}
	if !strings.Contains(ok1.Markdown, "Fixture") {
		t.Error("crawled content missing")
	}

	_, err = Crawl(ctx, server.URL+"/redirect-private")
	var blockedErr *BlockedAddressError
	if !errors.As(err, &blockedErr) {
		t.Errorf("redirect to private address must be blocked even with the test escape hatch (err=%v)", err)
	}

	redirected, err := Crawl(ctx, server.URL+"/redirect-ok")
	if err != nil {
		t.Fatalf("same-origin redirect must be followed: %v", err)
	}
	if !strings.HasSuffix(redirected.FinalURL, "/final") {
		t.Errorf("final URL = %q, want suffix /final", redirected.FinalURL)
	}
}
