package curio

import (
	"net/url"
	"testing"
)

func TestRedactURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain URL is unchanged",
			in:   "https://curio.example.com/pdp/pieces",
			want: "https://curio.example.com/pdp/pieces",
		},
		{
			name: "userinfo is stripped",
			in:   "https://alice:secret@curio.example.com/pdp",
			want: "https://curio.example.com/pdp",
		},
		{
			name: "sensitive query values are masked",
			in:   "https://curio.example.com/pdp?token=abc&region=us",
			want: "https://curio.example.com/pdp?token=***&region=us",
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			u, err := url.Parse(tc.in)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			origQuery := u.RawQuery
			origUser := u.User
			got := redactURL(u)
			if got != tc.want {
				t.Errorf("redactURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if u.RawQuery != origQuery {
				t.Errorf("redactURL mutated RawQuery: before=%q after=%q", origQuery, u.RawQuery)
			}
			if u.User != origUser {
				t.Errorf("redactURL mutated User: before=%v after=%v", origUser, u.User)
			}
		})
	}
}

func TestRedactURL_Nil(t *testing.T) {
	t.Parallel()
	if got := redactURL(nil); got != "" {
		t.Errorf("redactURL(nil) = %q, want empty", got)
	}
}

func TestRedactURLString(t *testing.T) {
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
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := redactURLString(tc.in); got != tc.want {
				t.Errorf("redactURLString(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
