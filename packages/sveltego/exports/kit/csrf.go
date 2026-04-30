package kit

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
)

// CSRFCookieName is the cookie that carries the per-session CSRF token.
// The value is intentionally not HttpOnly so a future client-side helper
// can copy it into a hidden form field on dynamically-created forms.
const CSRFCookieName = "_csrf"

// CSRFFieldName is the form field codegen injects into POST forms (and
// the server reads on submission) to validate the double-submit token.
const CSRFFieldName = "_csrf_token"

// csrfTokenBytes is the random-token entropy in bytes. 32 bytes
// base64url-encoded yields a 43-character cookie value.
const csrfTokenBytes = 32

// csrfTokenKey is the ev.Locals key under which the per-request CSRF
// token is cached. Lookups go through CSRFToken / SetCSRFToken so user
// code never references the bare key.
const csrfTokenKey = "csrfToken"

// CSRFToken returns the per-request CSRF token stored on ev by the
// server pipeline, or the empty string when CSRF is off, ev is nil, or
// the request did not pass through a CSRF-enabled route.
func CSRFToken(ev *RequestEvent) string {
	if ev == nil || ev.Locals == nil {
		return ""
	}
	if v, ok := ev.Locals[csrfTokenKey].(string); ok {
		return v
	}
	return ""
}

// SetCSRFToken stores token on ev.Locals under the canonical key used
// by the pipeline. User code does not call this.
func SetCSRFToken(ev *RequestEvent, token string) {
	if ev == nil {
		return
	}
	if ev.Locals == nil {
		ev.Locals = map[string]any{}
	}
	ev.Locals[csrfTokenKey] = token
}

// GenerateCSRFToken returns a fresh base64url-encoded random token.
// Returns the empty string only when crypto/rand fails — callers treat
// empty as "no token issued" and must not write a cookie or trust a
// match.
func GenerateCSRFToken() string {
	var buf [csrfTokenBytes]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(buf[:])
}

// CSRFTokenEqual compares the cookie and submitted token in constant
// time. Returns false on any length mismatch or empty input so callers
// can use it as a single decision point.
func CSRFTokenEqual(cookie, submitted string) bool {
	if cookie == "" || submitted == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie), []byte(submitted)) == 1
}
