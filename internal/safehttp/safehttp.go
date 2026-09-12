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

const fallbackDelay = 300 * time.Millisecond

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
		dialCtx, cancel := withDialTimeout(ctx, base.Timeout)
		defer cancel()

		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if ip := net.ParseIP(host); ip != nil {
			if !allowPrivate && isPrivateAddress(ip) {
				return nil, fmt.Errorf("%w: %s", ErrPrivateNetwork, ip)
			}
			return base.DialContext(dialCtx, network, addr)
		}

		ips, err := base.Resolver.LookupIPAddr(dialCtx, host)
		if err != nil {
			return nil, err
		}
		accepted := make([]net.IPAddr, 0, len(ips))
		var rejectedErr error
		for _, ipa := range ips {
			if !allowPrivate && isPrivateAddress(ipa.IP) {
				if rejectedErr == nil {
					rejectedErr = fmt.Errorf("%w: %s resolves to %s", ErrPrivateNetwork, host, ipa.IP)
				}
				continue
			}
			accepted = append(accepted, ipa)
		}
		if len(accepted) == 0 {
			if rejectedErr != nil {
				return nil, rejectedErr
			}
			return nil, fmt.Errorf("%w: no acceptable address for %s", ErrPrivateNetwork, host)
		}
		return dialResolvedContext(dialCtx, network, port, accepted, fallbackDelay, base.DialContext)
	}
}

func withDialTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

type dialContextFunc func(context.Context, string, string) (net.Conn, error)

type dialResult struct {
	conn  net.Conn
	err   error
	index int
}

// dialResolvedContext staggers connection attempts across address families.
// All attempts share ctx; the first success cancels the others, and any late
// successful connection is closed by its dialing goroutine.
func dialResolvedContext(
	ctx context.Context,
	network string,
	port string,
	ips []net.IPAddr,
	delay time.Duration,
	dial dialContextFunc,
) (net.Conn, error) {
	ordered := interleaveIPAddrs(ips)
	if len(ordered) == 0 {
		return nil, errors.New("safehttp: no resolved addresses")
	}

	raceCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan dialResult)
	start := func(index int) {
		go func() {
			conn, err := dial(raceCtx, network, net.JoinHostPort(ordered[index].String(), port))
			select {
			case results <- dialResult{conn: conn, err: err, index: index}:
			case <-raceCtx.Done():
				if conn != nil {
					_ = conn.Close()
				}
			}
		}()
	}

	errs := make([]error, len(ordered))
	next := 1
	active := 1
	start(0)

	var timer *time.Timer
	var timerC <-chan time.Time
	schedule := func() {
		if next >= len(ordered) {
			return
		}
		timer = time.NewTimer(delay)
		timerC = timer.C
	}
	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerC = nil
	}
	schedule()
	defer stopTimer()

	for active > 0 || next < len(ordered) {
		select {
		case result := <-results:
			active--
			if result.err == nil {
				return result.conn, nil
			}
			errs[result.index] = result.err
			if active == 0 && next < len(ordered) {
				stopTimer()
				start(next)
				next++
				active++
				schedule()
			}
		case <-timerC:
			timer = nil
			timerC = nil
			start(next)
			next++
			active++
			schedule()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return nil, errors.New("safehttp: all connection attempts failed")
}

func interleaveIPAddrs(ips []net.IPAddr) []net.IPAddr {
	if len(ips) < 2 {
		return ips
	}
	v4 := make([]net.IPAddr, 0, len(ips))
	v6 := make([]net.IPAddr, 0, len(ips))
	for _, ip := range ips {
		if ip.IP.To4() != nil {
			v4 = append(v4, ip)
		} else {
			v6 = append(v6, ip)
		}
	}
	primary, fallback := v6, v4
	if ips[0].IP.To4() != nil {
		primary, fallback = v4, v6
	}
	ordered := make([]net.IPAddr, 0, len(ips))
	for len(primary) > 0 || len(fallback) > 0 {
		if len(primary) > 0 {
			ordered = append(ordered, primary[0])
			primary = primary[1:]
		}
		if len(fallback) > 0 {
			ordered = append(ordered, fallback[0])
			fallback = fallback[1:]
		}
	}
	return ordered
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
