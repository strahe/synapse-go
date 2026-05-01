package pdp

import (
	"net/url"
	"strings"
)

// sensitiveQueryKeys is the allowlist of query parameter names whose
// values are redacted from logs and error messages. Matching is
// case-insensitive and keys are matched exactly after normalising
// underscores / hyphens — this avoids the classic false positives of a
// substring regex (e.g. "country_code" contains "code"; "turnkey" contains
// "key"; "designation" contains "sig").
var sensitiveQueryKeys = map[string]struct{}{
	"token":         {},
	"apitoken":      {},
	"apikey":        {},
	"xapikey":       {},
	"accesstoken":   {},
	"refreshtoken":  {},
	"idtoken":       {},
	"bearer":        {},
	"authorization": {},
	"auth":          {},
	"jwt":           {},
	"secret":        {},
	"apisecret":     {},
	"secretid":      {},
	"password":      {},
	"passwd":        {},
	"pwd":           {},
	"signature":     {},
	"sig":           {},
	"session":       {},
	"sessionid":     {},
	"sessiontoken":  {},
	"cookie":        {},
	"credential":    {},
	"credentials":   {},
	"key":           {},
	"privatekey":    {},
	"accesskey":     {},
	"secretkey":     {},
	// AWS / GCP presigned URL params:
	"xamzsignature":     {},
	"xamzcredential":    {},
	"xamzsecuritytoken": {},
	"xgoogsignature":    {},
	"xgoogcredential":   {},
	"googleaccessid":    {},
}

// isSensitiveQueryKey returns true when k (case-insensitively, with
// separators removed) matches the sensitiveQueryKeys allowlist.
func isSensitiveQueryKey(k string) bool {
	normalised := strings.ToLower(k)
	if _, ok := sensitiveQueryKeys[normalised]; ok {
		return true
	}
	// Tolerate `access_token`, `access-token`, `access.token` by stripping
	// common separators before a second lookup.
	stripped := strings.NewReplacer("_", "", "-", "", ".", "").Replace(normalised)
	if stripped != normalised {
		if _, ok := sensitiveQueryKeys[stripped]; ok {
			return true
		}
	}
	return false
}

// redactURL returns a string representation of u that is safe to log:
//
//   - any userinfo component is dropped entirely;
//   - any query parameter whose name matches redactSensitiveKey has its
//     value replaced with the literal "***";
//   - the path, scheme, host, port, and non-sensitive query values are
//     preserved so operators can still identify the endpoint.
//
// The original *url.URL is not mutated.
func redactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	clone := *u
	if clone.User != nil {
		clone.User = nil
	}
	if clone.RawQuery != "" {
		clone.RawQuery = redactRawQuery(clone.RawQuery)
	}
	return clone.String()
}

// redactRawQuery walks a raw "k=v&k=v" string and replaces the value of
// every key matching redactSensitiveKey with "***". Using manual parsing
// (rather than url.Values.Encode) keeps the placeholder human-readable
// instead of emitting "%2A%2A%2A".
func redactRawQuery(raw string) string {
	pairs := strings.Split(raw, "&")
	for i, p := range pairs {
		eq := strings.IndexByte(p, '=')
		var k string
		if eq < 0 {
			k = p
		} else {
			k = p[:eq]
		}
		decoded, err := url.QueryUnescape(k)
		if err == nil {
			k = decoded
		}
		if !isSensitiveQueryKey(k) {
			continue
		}
		if eq < 0 {
			pairs[i] = p + "=***"
		} else {
			pairs[i] = p[:eq] + "=***"
		}
	}
	return strings.Join(pairs, "&")
}

// redactURLString parses raw and returns the redacted form. If parsing fails
// it strips any "user:pass@" userinfo substring as a best-effort fallback.
func redactURLString(raw string) string {
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil {
		return redactURL(u)
	}
	// Fallback: strip anything of the shape "scheme://user:pass@host/...".
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		if at := strings.Index(rest, "@"); at >= 0 {
			// Drop the userinfo prefix only.
			return raw[:i+3] + rest[at+1:]
		}
	}
	return raw
}
