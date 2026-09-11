// Package safehttp builds HTTP clients that reject private and reserved
// network destinations.
package safehttp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"
)

// ErrPrivateNetwork is returned when a guarded client refuses to dial a
// private, local, multicast, unspecified, or reserved network address.
// Its text preserves the established storage.ErrPrivateNetwork error string.
var ErrPrivateNetwork = errors.New("storage: private / local network address disallowed")

var forbiddenFetchPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/29"),
	netip.MustParsePrefix("192.0.0.8/32"),
	netip.MustParsePrefix("192.0.0.170/31"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.2/32"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("100:0:0:1::/64"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
}

var nat64WellKnownPrefix = netip.MustParsePrefix("64:ff9b::/96")

// NewClient returns an HTTP client whose transport resolves each target once
// and dials the accepted IP directly. Environment-variable proxies are
// disabled. When allowPrivate is false, private and reserved destinations are
// rejected with ErrPrivateNetwork.
func NewClient(timeout time.Duration, allowPrivate bool) *http.Client {
	base := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           safeDialContext(base, allowPrivate),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}

func safeDialContext(base *net.Dialer, allowPrivate bool) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if ip := net.ParseIP(host); ip != nil {
			if !allowPrivate && isPrivateAddress(ip) {
				return nil, fmt.Errorf("%w: %s", ErrPrivateNetwork, ip)
			}
			return base.DialContext(ctx, network, addr)
		}

		ips, err := base.Resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		var firstErr error
		for _, ipa := range ips {
			if !allowPrivate && isPrivateAddress(ipa.IP) {
				if firstErr == nil {
					firstErr = fmt.Errorf("%w: %s resolves to %s", ErrPrivateNetwork, host, ipa.IP)
				}
				continue
			}
			conn, dialErr := base.DialContext(ctx, network, net.JoinHostPort(ipa.IP.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			if firstErr == nil {
				firstErr = dialErr
			}
		}
		if firstErr == nil {
			firstErr = fmt.Errorf("%w: no acceptable address for %s", ErrPrivateNetwork, host)
		}
		return nil, firstErr
	}
}

func isPrivateAddress(ip net.IP) bool {
	if ip == nil {
		return true
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	return isForbiddenFetchAddr(addr.Unmap())
}

func isForbiddenFetchAddr(addr netip.Addr) bool {
	if addr.IsUnspecified() || addr.IsLoopback() || addr.IsMulticast() ||
		addr.IsLinkLocalUnicast() || addr.IsPrivate() {
		return true
	}
	if nat64WellKnownPrefix.Contains(addr) {
		bits := addr.As16()
		if isForbiddenFetchAddr(netip.AddrFrom4([4]byte{bits[12], bits[13], bits[14], bits[15]})) {
			return true
		}
	}
	for _, prefix := range forbiddenFetchPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
