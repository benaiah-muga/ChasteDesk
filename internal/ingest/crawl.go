// SPDX-License-Identifier: AGPL-3.0-only
package ingest

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	crawlerMaxRedirects = 3
	crawlerMaxBodyBytes = 5 * 1024 * 1024
	crawlerTimeout      = 15 * time.Second
	crawlerUserAgent    = "ChasteDeskIngest/0.1 (+https://chastedesk.local/bot)"
)

var allowedContentTypes = []string{"text/html", "text/plain", "text/markdown"}

type CrawlResult struct {
	Markdown    string
	Title       string
	FinalURL    string
	ContentType string
	Bytes       int
}

func defaultLookup(host string) ([]Addr, error) {
	ips, err := net.DefaultResolver.LookupNetIP(context.Background(), "ip", host)
	if err != nil {
		return nil, err
	}
	addresses := make([]Addr, 0, len(ips))
	for _, ip := range ips {
		family := 4
		if ip.Is6() {
			family = 6
		}
		addresses = append(addresses, Addr{Address: ip.Unmap().String(), Family: family})
	}
	return addresses, nil
}

func Crawl(ctx context.Context, rawURL string) (CrawlResult, error) {
	policy := AddressPolicy{AllowPrivate: AllowPrivateIngestion(os.Getenv("APP_ENV"))}
	validated, err := ValidateIngestURL(rawURL, policy, defaultLookup)
	if err != nil {
		return CrawlResult{}, err
	}
	return fetchHop(ctx, validated.URL, validated.PinnedIP, crawlerMaxRedirects, policy)
}

func originOf(u *url.URL) string {
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

func fetchHop(ctx context.Context, target *url.URL, pinnedIP string, redirectsLeft int, policy AddressPolicy) (CrawlResult, error) {
	dialer := &net.Dialer{Timeout: crawlerTimeout}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(addr)
			if err != nil {
				if target.Scheme == "https" {
					port = "443"
				} else {
					port = "80"
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(pinnedIP, port))
		},
		TLSClientConfig:   &tls.Config{ServerName: target.Hostname()},
		DisableKeepAlives: true,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   crawlerTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return CrawlResult{}, err
	}
	req.Header.Set("User-Agent", crawlerUserAgent)
	req.Header.Set("Accept", "text/html,text/plain,text/markdown,*/*;q=0.5")
	resp, err := client.Do(req)
	if err != nil {
		return CrawlResult{}, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		location := resp.Header.Get("Location")
		if location == "" || redirectsLeft <= 0 {
			return CrawlResult{}, fmt.Errorf("redirect loop or missing location at %s", target.String())
		}
		next, err := target.Parse(location)
		if err != nil {
			return CrawlResult{}, fmt.Errorf("invalid redirect location %q", location)
		}
		if originOf(next) == originOf(target) {
			return fetchHop(ctx, next, pinnedIP, redirectsLeft-1, policy)
		}
		validated, err := ValidateIngestURL(next.String(), AddressPolicy{AllowPrivate: false}, defaultLookup)
		if err != nil {
			return CrawlResult{}, err
		}
		return fetchHop(ctx, validated.URL, validated.PinnedIP, redirectsLeft-1, AddressPolicy{AllowPrivate: false})
	case http.StatusOK:
	default:
		return CrawlResult{}, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, target.String())
	}

	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	allowed := false
	for _, candidate := range allowedContentTypes {
		if strings.HasPrefix(contentType, candidate) {
			allowed = true
			break
		}
	}
	if !allowed {
		return CrawlResult{}, fmt.Errorf("content type not allowed: %s", contentType)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, crawlerMaxBodyBytes+1))
	if err != nil {
		return CrawlResult{}, err
	}
	if len(body) > crawlerMaxBodyBytes {
		return CrawlResult{}, fmt.Errorf("body exceeds %d bytes", crawlerMaxBodyBytes)
	}
	markdown, title := HtmlToMarkdown(string(body))
	return CrawlResult{
		Markdown:    markdown,
		Title:       title,
		FinalURL:    target.String(),
		ContentType: contentType,
		Bytes:       len(body),
	}, nil
}
