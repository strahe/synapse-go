package storage

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"
)

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

// newSafeHTTPClient returns an *http.Client whose transport refuses to dial
// local, private, multicast, unspecified, or reserved addresses. This is the
// default for Service.httpClient when neither a custom HTTPClient nor
// AllowPrivateNetworks=true is supplied to prevent SSRF via Service.Download
// URL-based calls. Environment-variable proxies are intentionally disabled:
// callers that need an explicit proxy must supply HTTPClient and provide
// equivalent SSRF safeguards themselves.
func newSafeHTTPClient(timeout time.Duration, allowPrivate bool) *http.Client {
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

// safeDialContext returns a DialContext that resolves the target host and,
// when allowPrivate is false, rejects any IP in local, private, multicast,
// unspecified, or reserved ranges. Resolution is performed once and the
// resolved IP is dialed directly, eliminating the DNS-rebinding window between
// check and connect.
func safeDialContext(base *net.Dialer, allowPrivate bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		// If the host is already an IP literal, validate it directly.
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
			conn, derr := base.DialContext(ctx, network, net.JoinHostPort(ipa.IP.String(), port))
			if derr == nil {
				return conn, nil
			}
			if firstErr == nil {
				firstErr = derr
			}
		}
		if firstErr == nil {
			firstErr = fmt.Errorf("%w: no acceptable address for %s", ErrPrivateNetwork, host)
		}
		return nil, firstErr
	}
}

// isPrivateAddress returns true for IPs that should never be dialed from
// SDK-initiated downloads of remote provider URLs: loopback, link-local,
// RFC1918 / ULA, multicast, unspecified, and selected special-use prefixes.
func isPrivateAddress(ip net.IP) bool {
	if ip == nil {
		return true
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	return isForbiddenFetchAddr(addr)
}

func isForbiddenFetchAddr(addr netip.Addr) bool {
	if addr.IsUnspecified() || addr.IsLoopback() || addr.IsMulticast() ||
		addr.IsLinkLocalUnicast() || addr.IsPrivate() {
		return true
	}
	if nat64WellKnownPrefix.Contains(addr) {
		b := addr.As16()
		if isForbiddenFetchAddr(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]})) {
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
