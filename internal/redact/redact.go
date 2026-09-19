// Package redact removes credentials from URLs before they are placed in
// errors or logs.
package redact

import (
	"errors"
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

// URL returns a string representation of u that is safe to log:
//
//   - any userinfo component is dropped entirely;
//   - any query parameter whose name matches isSensitiveQueryKey has its
//     value replaced with the literal "***";
//   - the path, scheme, host, port, and non-sensitive query values are
//     preserved so operators can still identify the endpoint.
//
// The original *url.URL is not mutated.
func URL(u *url.URL) string {
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
// every key matching isSensitiveQueryKey with "***". Using manual parsing
// (rather than url.Values.Encode) keeps the placeholder human-readable
// instead of emitting "%2A%2A%2A".
func redactRawQuery(raw string) string {
	pairs := strings.Split(raw, "&")
	for i, p := range pairs {
		before, _, ok := strings.Cut(p, "=")
		var k string
		if !ok {
			k = p
		} else {
			k = before
		}
		decoded, err := url.QueryUnescape(k)
		if err == nil {
			k = decoded
		}
		if !isSensitiveQueryKey(k) {
			continue
		}
		if !ok {
			pairs[i] = p + "=***"
		} else {
			pairs[i] = before + "=***"
		}
	}
	return strings.Join(pairs, "&")
}

// URLString parses raw and returns the redacted form. If parsing fails it
// splits raw the same way net/url does: the fragment is dropped, sensitive
// query values are masked, and userinfo is removed from the authority only,
// so an "@" in the path or query is left alone.
func URLString(raw string) string {
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil {
		return URL(u)
	}
	rest, _, _ := strings.Cut(raw, "#")
	rest, query, hasQuery := strings.Cut(rest, "?")
	if scheme, afterScheme, ok := strings.Cut(rest, "://"); ok {
		authority, path, hasPath := strings.Cut(afterScheme, "/")
		if at := strings.LastIndexByte(authority, '@'); at >= 0 {
			authority = authority[at+1:]
		}
		rest = scheme + "://" + authority
		if hasPath {
			rest += "/" + path
		}
	}
	if hasQuery {
		rest += "?" + redactRawQuery(query)
	}
	return rest
}

// URLError returns a copy of the *url.Error in err's chain with its URL
// redacted. The copy keeps Op and Err, so errors.Is and errors.As still reach
// the underlying cause. Pass errors returned directly by net/http or net/url:
// wrapping around the *url.Error is not preserved. Errors without a
// *url.Error are returned unchanged.
func URLError(err error) error {
	urlErr, ok := errors.AsType[*url.Error](err)
	if !ok {
		return err
	}
	redacted := *urlErr
	redacted.URL = URLString(urlErr.URL)
	return &redacted
}
