package safehttp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type closeTrackingConn struct {
	net.Conn
	closed chan struct{}
	once   sync.Once
}

func (c *closeTrackingConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

func TestIsPrivateAddress(t *testing.T) {
	tests := []struct {
		name string
		ip   net.IP
		want bool
	}{
		{name: "nil", ip: nil, want: true},
		{name: "invalid length", ip: net.IP{1, 2, 3}, want: true},
		{name: "unspecified IPv4", ip: net.ParseIP("0.0.0.0"), want: true},
		{name: "zero IPv4 network", ip: net.ParseIP("0.0.0.1"), want: true},
		{name: "loopback IPv4", ip: net.ParseIP("127.0.0.1"), want: true},
		{name: "private IPv4", ip: net.ParseIP("10.0.0.1"), want: true},
		{name: "link local metadata", ip: net.ParseIP("169.254.169.254"), want: true},
		{name: "multicast IPv4", ip: net.ParseIP("224.0.0.1"), want: true},
		{name: "CGNAT", ip: net.ParseIP("100.64.0.1"), want: true},
		{name: "IETF protocol assignments", ip: net.ParseIP("192.0.0.1"), want: true},
		{name: "IPv4 dummy address", ip: net.ParseIP("192.0.0.8"), want: true},
		{name: "NAT64 discovery", ip: net.ParseIP("192.0.0.170"), want: true},
		{name: "documentation IPv4", ip: net.ParseIP("192.0.2.1"), want: true},
		{name: "6a44 relay anycast", ip: net.ParseIP("192.88.99.2"), want: true},
		{name: "benchmarking IPv4", ip: net.ParseIP("198.18.0.1"), want: true},
		{name: "reserved IPv4", ip: net.ParseIP("240.0.0.1"), want: true},
		{name: "mapped loopback", ip: net.ParseIP("::ffff:127.0.0.1"), want: true},
		{name: "mapped private", ip: net.ParseIP("::ffff:10.0.0.1"), want: true},
		{name: "mapped link local", ip: net.ParseIP("::ffff:169.254.169.254"), want: true},
		{name: "unspecified IPv6", ip: net.ParseIP("::"), want: true},
		{name: "loopback IPv6", ip: net.ParseIP("::1"), want: true},
		{name: "private IPv6", ip: net.ParseIP("fd00::1"), want: true},
		{name: "link local IPv6", ip: net.ParseIP("fe80::1"), want: true},
		{name: "multicast IPv6", ip: net.ParseIP("ff02::1"), want: true},
		{name: "local-use NAT64", ip: net.ParseIP("64:ff9b:1::c000:201"), want: true},
		{name: "discard-only IPv6", ip: net.ParseIP("100::1"), want: true},
		{name: "dummy IPv6", ip: net.ParseIP("100:0:0:1::1"), want: true},
		{name: "benchmarking IPv6", ip: net.ParseIP("2001:2::1"), want: true},
		{name: "documentation IPv6", ip: net.ParseIP("2001:db8::1"), want: true},
		{name: "documentation IPv6 2", ip: net.ParseIP("3fff::1"), want: true},
		{name: "segment routing SIDs", ip: net.ParseIP("5f00::1"), want: true},
		{name: "NAT64 private IPv4", ip: net.ParseIP("64:ff9b::10.0.0.1"), want: true},
		{name: "NAT64 CGNAT IPv4", ip: net.ParseIP("64:ff9b::100.64.0.1"), want: true},
		{name: "NAT64 documentation IPv4", ip: net.ParseIP("64:ff9b::192.0.2.1"), want: true},
		{name: "public IPv4", ip: net.ParseIP("8.8.8.8"), want: false},
		{name: "mapped public IPv4", ip: net.ParseIP("::ffff:8.8.8.8"), want: false},
		{name: "public IPv6", ip: net.ParseIP("2606:4700:4700::1111"), want: false},
		{name: "NAT64 public IPv4", ip: net.ParseIP("64:ff9b::8.8.8.8"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPrivateAddress(tt.ip); got != tt.want {
				t.Fatalf("isPrivateAddress(%v) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestNewClientConfiguration(t *testing.T) {
	const timeout = 17 * time.Second
	for _, allowPrivate := range []bool{false, true} {
		client := NewClient(timeout, allowPrivate)
		if client.Timeout != timeout {
			t.Fatalf("allowPrivate=%v: Timeout = %v, want %v", allowPrivate, client.Timeout, timeout)
		}
		transport, ok := client.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("allowPrivate=%v: Transport type = %T, want *http.Transport", allowPrivate, client.Transport)
		}
		if transport.Proxy != nil {
			t.Fatalf("allowPrivate=%v: Proxy must be nil", allowPrivate)
		}
		if transport.DialContext == nil {
			t.Fatalf("allowPrivate=%v: DialContext must be configured", allowPrivate)
		}
	}
}

func TestDialResolvedContextFallsBackAndClosesLoser(t *testing.T) {
	left, right := net.Pipe()
	t.Cleanup(func() { _ = right.Close() })
	loser := &closeTrackingConn{Conn: left, closed: make(chan struct{})}
	started := make(chan string, 3)
	dial := func(ctx context.Context, _, addr string) (net.Conn, error) {
		started <- addr
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if net.ParseIP(host).To4() == nil {
			<-ctx.Done()
			return loser, nil
		}
		conn, peer := net.Pipe()
		_ = peer.Close()
		return conn, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := dialResolvedContext(ctx, "tcp", "443", []net.IPAddr{
		{IP: net.ParseIP("2606:4700:4700::1111")},
		{IP: net.ParseIP("2001:4860:4860::8888")},
		{IP: net.ParseIP("8.8.8.8")},
	}, 5*time.Millisecond, dial)
	if err != nil {
		t.Fatalf("dialResolvedContext: %v", err)
	}
	_ = conn.Close()

	if got := <-started; got != "[2606:4700:4700::1111]:443" {
		t.Fatalf("first address = %q, want first IPv6 address", got)
	}
	if got := <-started; got != "8.8.8.8:443" {
		t.Fatalf("fallback address = %q, want interleaved IPv4 address", got)
	}
	select {
	case <-loser.closed:
	case <-time.After(time.Second):
		t.Fatal("losing connection was not canceled and closed")
	}
}

func TestNewClientRejectsPrivateDestinations(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, "unexpected")
	}))
	defer server.Close()

	client := NewClient(time.Second, false)
	defer client.CloseIdleConnections()
	for _, target := range []string{
		server.URL,
		strings.Replace(server.URL, "127.0.0.1", "localhost", 1),
	} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
		if err != nil {
			t.Fatalf("NewRequestWithContext: %v", err)
		}
		resp, err := client.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if !errors.Is(err, ErrPrivateNetwork) {
			t.Fatalf("GET %s error = %v, want ErrPrivateNetwork", target, err)
		}
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("server received %d requests, want 0", got)
	}
}

func TestNewClientAllowsPrivateDestinationsWhenEnabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	client := NewClient(time.Second, true)
	defer client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
}
