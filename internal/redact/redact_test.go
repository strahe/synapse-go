package redact

import (
	"errors"
	"net/url"
	"testing"
)

func TestURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain URL is unchanged",
			in:   "https://pdp.example.com/pdp/pieces",
			want: "https://pdp.example.com/pdp/pieces",
		},
		{
			name: "userinfo is stripped",
			in:   "https://alice:secret@pdp.example.com/pdp",
			want: "https://pdp.example.com/pdp",
		},
		{
			name: "sensitive query values are masked",
			in:   "https://pdp.example.com/pdp?token=abc&region=us",
			want: "https://pdp.example.com/pdp?token=***&region=us",
		},
		{
			name: "multiple sensitive params masked, ordinary preserved",
			in:   "https://x/y?auth=A&key=B&page=1&signature=S",
			want: "https://x/y?auth=***&key=***&page=1&signature=***",
		},
		{
			name: "case-insensitive match",
			in:   "https://x/y?TOKEN=t&SecretID=s",
			want: "https://x/y?TOKEN=***&SecretID=***",
		},
		{
			name: "path and port preserved",
			in:   "http://host:4702/pdp/piece/bafy?token=t",
			want: "http://host:4702/pdp/piece/bafy?token=***",
		},
		{
			name: "query with both userinfo and sensitive key",
			in:   "https://u:p@x/y?apikey=abc",
			want: "https://x/y?apikey=***",
		},
		{
			name: "generic 'code' is NOT redacted (false-positive-prone)",
			in:   "https://x/y?code=abc&country_code=US",
			want: "https://x/y?code=abc&country_code=US",
		},
		{
			name: "underscore/hyphen separator tolerated",
			in:   "https://x/y?access_token=a&x-api-key=b&refresh-token=r",
			want: "https://x/y?access_token=***&x-api-key=***&refresh-token=***",
		},
		{
			name: "unrelated params with sensitive substring are preserved",
			in:   "https://x/y?turnkey=t&designation=d&keyring_id=k",
			want: "https://x/y?turnkey=t&designation=d&keyring_id=k",
		},
		{
			name: "bearer/jwt/credential/session masked",
			in:   "https://x/y?bearer=b&jwt=j&credential=c&session=s",
			want: "https://x/y?bearer=***&jwt=***&credential=***&session=***",
		},
		{
			name: "aws and gcs signed-url variants are masked",
			in:   "https://x/y?x_amz_signature=s&x.goog.signature=g&X-Goog-Credential=cred&GoogleAccessId=id&safe=1",
			want: "https://x/y?x_amz_signature=***&x.goog.signature=***&X-Goog-Credential=***&GoogleAccessId=***&safe=1",
		},
		{
			name: "canonical aws signed-url params are masked",
			in:   "https://s3.amazonaws.com/bucket/key?X-Amz-Signature=abc&X-Amz-Credential=cred&X-Amz-Security-Token=tok&other=1",
			want: "https://s3.amazonaws.com/bucket/key?X-Amz-Signature=***&X-Amz-Credential=***&X-Amz-Security-Token=***&other=1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			u, err := url.Parse(tc.in)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			origQuery := u.RawQuery
			origUser := u.User
			got := URL(u)
			if got != tc.want {
				t.Errorf("URL(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if u.RawQuery != origQuery {
				t.Errorf("URL mutated RawQuery: before=%q after=%q", origQuery, u.RawQuery)
			}
			if u.User != origUser {
				t.Errorf("URL mutated User: before=%v after=%v", origUser, u.User)
			}
		})
	}
}

func TestURL_Nil(t *testing.T) {
	t.Parallel()
	if got := URL(nil); got != "" {
		t.Errorf("URL(nil) = %q, want empty", got)
	}
}

func TestURLString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"valid URL redacts", "https://a:b@x/y?token=t", "https://x/y?token=***"},
		{
			name: "unparseable falls back to userinfo strip",
			in:   "https://user:p\x7fass@host/path",
			want: "https://host/path",
		},
		{
			name: "unparseable masks sensitive query values",
			in:   "https://host/%zz?token=secretquery&part=1",
			want: "https://host/%zz?token=***&part=1",
		},
		{
			name: "unparseable strips userinfo and masks query",
			in:   "https://secretuser:secretpass@host/%zz?token=secretquery&part=1",
			want: "https://host/%zz?token=***&part=1",
		},
		{
			name: "unparseable only strips userinfo from the authority",
			in:   "https://host/p%zz?email=a@b.com&token=secretquery",
			want: "https://host/p%zz?email=a@b.com&token=***",
		},
		{
			name: "unparseable drops the fragment",
			in:   "https://secretuser:secretpass@host/\x7f#access_token=secretquery",
			want: "https://host/\x7f",
		},
		{
			name: "unparseable without scheme masks query",
			in:   "%zz?token=secretquery",
			want: "%zz?token=***",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := URLString(tc.in); got != tc.want {
				t.Errorf("URLString(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestURLError(t *testing.T) {
	t.Parallel()

	cause := errors.New("connection refused")
	orig := &url.Error{Op: "Get", URL: "https://user@host/path?token=secret&part=1", Err: cause}

	got := URLError(orig)
	urlErr, ok := errors.AsType[*url.Error](got)
	if !ok {
		t.Fatalf("URLError returned %T, want *url.Error", got)
	}
	if urlErr == orig {
		t.Fatal("URLError returned the original *url.Error instead of a copy")
	}
	if want := "https://host/path?token=***&part=1"; urlErr.URL != want {
		t.Errorf("URL = %q, want %q", urlErr.URL, want)
	}
	if urlErr.Op != "Get" {
		t.Errorf("Op = %q, want Get", urlErr.Op)
	}
	if !errors.Is(got, cause) {
		t.Error("redacted error no longer matches the underlying cause")
	}
	if orig.URL != "https://user@host/path?token=secret&part=1" {
		t.Errorf("URLError mutated the original URL: %q", orig.URL)
	}
}

func TestURLError_NonURLError(t *testing.T) {
	t.Parallel()

	err := errors.New("plain")
	if got := URLError(err); !errors.Is(got, err) {
		t.Errorf("URLError(%v) = %v, want the same error", err, got)
	}
	if got := URLError(nil); got != nil {
		t.Errorf("URLError(nil) = %v, want nil", got)
	}
}
