// SPDX-License-Identifier: AGPL-3.0-only
package ingest

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strings"
)

type AddressPolicy struct {
	AllowPrivate bool
}

type BlockedAddressError struct {
	message string
}

func (e *BlockedAddressError) Error() string {
	return e.message
}

func blocked(format string, args ...any) error {
	return &BlockedAddressError{message: fmt.Sprintf(format, args...)}
}

type Addr struct {
	Address string
	Family  int
}

type LookupFunc func(host string) ([]Addr, error)

var privateRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("192.0.0.0/24"),
}

func IsPrivateAddress(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	if addr.Is4() {
		return addrInRanges(addr)
	}
	if addr.Is4In6() {
		return addrInRanges(addr.Unmap())
	}
	if addr == netip.IPv6Unspecified() || addr == netip.IPv6Loopback() {
		return true
	}
	lower := strings.ToLower(ip)
	if strings.HasPrefix(lower, "fe80:") {
		return true
	}
	raw := addr.As16()
	if raw[0] == 0xfc || raw[0] == 0xfd {
		return true
	}
	return false
}

func addrInRanges(addr netip.Addr) bool {
	for _, prefix := range privateRanges {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

type ValidatedTarget struct {
	URL      *url.URL
	PinnedIP string
}

func ValidateIngestURL(rawURL string, policy AddressPolicy, lookup LookupFunc) (ValidatedTarget, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return ValidatedTarget{}, blocked("invalid url: %s", rawURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ValidatedTarget{}, blocked("scheme not allowed: %s", parsed.Scheme)
	}
	addresses, err := lookup(parsed.Hostname())
	if err != nil {
		return ValidatedTarget{}, err
	}
	if len(addresses) == 0 {
		return ValidatedTarget{}, blocked("no addresses resolved for %s", parsed.Hostname())
	}
	for _, addr := range addresses {
		if !policy.AllowPrivate && IsPrivateAddress(addr.Address) {
			return ValidatedTarget{}, blocked("address %s for %s is not a public target", addr.Address, parsed.Hostname())
		}
	}
	pinned := addresses[0].Address
	if pinned == "" {
		return ValidatedTarget{}, blocked("no usable address for %s", parsed.Hostname())
	}
	return ValidatedTarget{URL: parsed, PinnedIP: pinned}, nil
}

const ingestAllowPrivateEnv = "CHASTESK_INGEST_ALLOW_PRIVATE"

func AllowPrivateIngestion(appEnv string) bool {
	if appEnv == "production" {
		return false
	}
	return os.Getenv(ingestAllowPrivateEnv) == "1"
}
