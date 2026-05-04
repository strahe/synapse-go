package storage

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// newSafeHTTPClient returns an *http.Client whose transport refuses to dial
// private/loopback/link-local/multicast/unspecified addresses. This is the
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
// when allowPrivate is false, rejects any IP that falls into loopback,
// link-local, RFC1918 / ULA, multicast, or unspecified ranges. Resolution is
// performed once and the resolved IP is dialed directly, eliminating the
// DNS-rebinding window between check and connect.
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
// RFC1918 / ULA (net.IP.IsPrivate), RFC6598 (CGNAT), multicast, unspecified,
// and zero IPv4 network addresses.
func isPrivateAddress(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsPrivate() {
		return true
	}
	// RFC6598 CGNAT range: 100.64.0.0/10
	if v4 := ip.To4(); v4 != nil {
		// Zero IPv4 network: 0.0.0.0/8
		if v4[0] == 0 {
			return true
		}
		if v4[0] == 100 && (v4[1]&0xc0) == 64 {
			return true
		}
		// TEST-NET-1: 192.0.2.0/24
		if v4[0] == 192 && v4[1] == 0 && v4[2] == 2 {
			return true
		}
		// TEST-NET-2: 198.51.100.0/24
		if v4[0] == 198 && v4[1] == 51 && v4[2] == 100 {
			return true
		}
		// TEST-NET-3: 203.0.113.0/24
		if v4[0] == 203 && v4[1] == 0 && v4[2] == 113 {
			return true
		}
		// Benchmarking: 198.18.0.0/15
		if v4[0] == 198 && (v4[1]&0xfe) == 18 {
			return true
		}
		// Reserved: 240.0.0.0/4
		if (v4[0] & 0xf0) == 0xf0 {
			return true
		}
	}
	return false
}
