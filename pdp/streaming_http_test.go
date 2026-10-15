package pdp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHTTPClientPolicies(t *testing.T) {
	for _, tt := range []struct {
		name           string
		options        func(general, streaming *http.Client) []Option
		generalTimeout time.Duration
		wantControl    time.Duration
		wantStreaming  time.Duration
		callerTimeout  time.Duration
	}{
		{name: "defaults", wantControl: DefaultHTTPTimeout},
		{
			name: "custom zero",
			options: func(general, _ *http.Client) []Option {
				return []Option{WithHTTPClient(general)}
			},
		},
		{
			name: "custom positive", generalTimeout: 5 * time.Minute,
			options: func(general, _ *http.Client) []Option {
				return []Option{WithHTTPClient(general)}
			},
			wantControl: 5 * time.Minute, wantStreaming: 5 * time.Minute,
		},
		{
			name: "custom default value", generalTimeout: DefaultHTTPTimeout,
			options: func(general, _ *http.Client) []Option {
				return []Option{WithHTTPClient(general)}
			},
			wantControl: DefaultHTTPTimeout, wantStreaming: DefaultHTTPTimeout,
		},
		{
			name: "streaming override", generalTimeout: 5 * time.Minute,
			options: func(general, streaming *http.Client) []Option {
				return []Option{WithHTTPClient(general), WithStreamingHTTPClient(streaming)}
			},
			wantControl: 5 * time.Minute, wantStreaming: 2 * time.Minute,
		},
		{
			name: "general override", generalTimeout: 5 * time.Minute,
			options: func(general, streaming *http.Client) []Option {
				return []Option{WithStreamingHTTPClient(streaming), WithHTTPClient(general)}
			},
			wantControl: 5 * time.Minute, wantStreaming: 5 * time.Minute,
		},
		{
			name: "streaming only",
			options: func(_, streaming *http.Client) []Option {
				return []Option{WithStreamingHTTPClient(streaming)}
			},
			wantControl: DefaultHTTPTimeout, wantStreaming: 2 * time.Minute,
		},
		{
			name: "nil options preserve clients", generalTimeout: 5 * time.Minute,
			options: func(general, streaming *http.Client) []Option {
				return []Option{WithHTTPClient(general), WithStreamingHTTPClient(streaming), WithHTTPClient(nil), nil, WithStreamingHTTPClient(nil)}
			},
			wantControl: 5 * time.Minute, wantStreaming: 2 * time.Minute,
		},
		{
			name: "caller deadline first", generalTimeout: 5 * time.Minute,
			options: func(general, _ *http.Client) []Option {
				return []Option{WithHTTPClient(general)}
			},
			wantControl: 5 * time.Minute, wantStreaming: 5 * time.Minute, callerTimeout: 10 * time.Second,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			general := &http.Client{Timeout: tt.generalTimeout}
			streaming := &http.Client{Timeout: 2 * time.Minute}
			var opts []Option
			if tt.options != nil {
				opts = tt.options(general, streaming)
			}
			c, err := New("https://provider.example", opts...)
			if err != nil {
				t.Fatal(err)
			}
			if tt.options == nil && c.httpClient.Transport != c.streamingHTTPClient.Transport {
				t.Fatal("default clients do not share a transport")
			}
			ctx := context.Background()
			if tt.callerTimeout != 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.callerTimeout)
				defer cancel()
			}
			var calls int
			rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				want := tt.wantControl
				if strings.HasPrefix(r.URL.Path, "/piece/") {
					want = tt.wantStreaming
				}
				deadline, hasDeadline := r.Context().Deadline()
				switch {
				case tt.callerTimeout != 0 && (want == 0 || tt.callerTimeout < want):
					callerDeadline, _ := ctx.Deadline()
					if !hasDeadline || !deadline.Equal(callerDeadline) {
						t.Errorf("deadline=%v, want caller deadline %v", deadline, callerDeadline)
					}
				case want == 0:
					if hasDeadline {
						t.Errorf("unexpected deadline %v", deadline)
					}
				case !hasDeadline || time.Until(deadline) > want || time.Until(deadline) < want-time.Second:
					t.Errorf("deadline remaining=%v, want approximately %v", time.Until(deadline), want)
				}
				if got := r.Header.Get("User-Agent"); got != DefaultUserAgent {
					t.Errorf("User-Agent=%q, want %q", got, DefaultUserAgent)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("curio-pdp"))}, nil
			})
			c.httpClient.Transport = rt
			c.streamingHTTPClient.Transport = rt
			if err := c.Ping(ctx); err != nil {
				t.Fatal(err)
			}
			body, _, err := c.DownloadPiece(ctx, testPieceInfoV2(t).CIDv2)
			if err != nil {
				t.Fatal(err)
			}
			_ = body.Close()
			if calls != 2 {
				t.Fatalf("requests=%d, want 2", calls)
			}
			if general.Timeout != tt.generalTimeout || streaming.Timeout != 2*time.Minute {
				t.Fatal("custom client timeout was mutated")
			}
		})
	}
}

type contextBlockingBody struct {
	ctx context.Context
}

func (b contextBlockingBody) Read([]byte) (int, error) {
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (contextBlockingBody) Close() error { return nil }

func TestDownloadPiece_CustomTimeoutIncludesBody(t *testing.T) {
	h := &http.Client{
		Timeout: 100 * time.Millisecond,
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: contextBlockingBody{ctx: r.Context()}}, nil
		}),
	}
	c, err := New("https://provider.example", WithHTTPClient(h))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	body, _, err := c.DownloadPiece(ctx, testPieceInfoV2(t).CIDv2)
	if err != nil {
		t.Fatalf("DownloadPiece before body read: %v", err)
	}
	defer func() { _ = body.Close() }()
	if _, err := io.ReadAll(body); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ReadAll error=%v, want deadline exceeded", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("caller deadline expired: %v", ctx.Err())
	}
	if h.Timeout != 100*time.Millisecond {
		t.Fatalf("custom Timeout=%v, want 100ms", h.Timeout)
	}
}

func TestDownloadPiece_CustomClientPolicies(t *testing.T) {
	var redirects, transports int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/piece/") {
			http.Redirect(w, r, "/download", http.StatusFound)
			return
		}
		cookie, err := r.Cookie("session")
		if err != nil || cookie.Value != "accepted" {
			t.Errorf("session cookie=%v, error=%v", cookie, err)
		}
		_, _ = io.WriteString(w, "piece payload")
	}))
	defer srv.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: "accepted"}})
	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		transports++
		return srv.Client().Transport.RoundTrip(r)
	})
	h := &http.Client{
		Transport: rt,
		Timeout:   DefaultHTTPTimeout,
		Jar:       jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			redirects++
			return nil
		},
	}
	c, err := New(srv.URL, WithHTTPClient(h))
	if err != nil {
		t.Fatal(err)
	}
	body, _, err := c.DownloadPiece(context.Background(), testPieceInfoV2(t).CIDv2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = body.Close() }()
	got, err := io.ReadAll(body)
	if err != nil || string(got) != "piece payload" {
		t.Fatalf("body=%q, error=%v", got, err)
	}
	if redirects != 1 || transports != 2 {
		t.Fatalf("redirects=%d, transports=%d, want 1 and 2", redirects, transports)
	}
	if h.Timeout != DefaultHTTPTimeout || h.Jar != jar || h.Transport == nil || h.CheckRedirect == nil {
		t.Fatal("custom client configuration was mutated")
	}
}
