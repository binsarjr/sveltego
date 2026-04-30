/*
Package auth — CSRF protection (double-submit cookie + trusted-origin check).

# Threat model

## What it stops

  - Classic CSRF: a malicious page on a different origin POSTing to the auth
    router. The browser sends cookies automatically, but cannot read the CSRF
    cookie value due to the Same-Origin Policy, so it cannot copy the token
    into the required header or form field.
  - Subdomain-based CSRF: the Origin/Referer allowlist rejects requests that
    don't come from a trusted origin, even when the attacker controls a
    sibling subdomain.

## What it does NOT stop

  - XSS — once an attacker can run JavaScript on the authenticated origin,
    they can read the (intentionally non-HttpOnly) cookie.
  - SameSite-Lax POST bypass — SameSite alone is insufficient for auth
    endpoints; the double-submit provides the second layer.

## Algorithm

 1. On the first GET that hits the auth router, Issue writes a random
    base64url token into a short-lived cookie named "__Host-sveltego-csrf".
    The cookie is intentionally NOT HttpOnly so that JavaScript form helpers
    can copy it into a header or form field.
 2. On every state-changing request (POST/PUT/PATCH/DELETE), Verify:
    a. Reads the cookie value.
    b. Reads the "X-CSRF-Token" header (falling back to the "csrf_token" form
    field for non-JSON forms).
    c. Compares them in constant time.
    d. Validates the Origin (or Referer) header against TrustedOrigins when
    the list is non-empty.
*/
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	defaultCSRFCookieName = "__Host-sveltego-csrf"
	defaultCSRFHeaderName = "X-CSRF-Token"
	defaultCSRFFieldName  = "csrf_token"
	defaultCSRFTokenSize  = 32
)

// stateMutating is the set of HTTP methods that change server state.
var stateMutating = map[string]bool{
	http.MethodPost:   true,
	http.MethodPut:    true,
	http.MethodPatch:  true,
	http.MethodDelete: true,
}

// CSRF implements double-submit cookie CSRF protection with an optional
// trusted-origin allowlist. Construct via NewCSRF.
type CSRF struct {
	// CookieName is the name of the CSRF cookie. Default: "__Host-sveltego-csrf".
	// The "__Host-" prefix requires Secure + no Domain attribute per OWASP;
	// set AllowInsecure to override for local dev.
	CookieName string

	// HeaderName is the request header carrying the CSRF token. Default: "X-CSRF-Token".
	HeaderName string

	// FieldName is the form field name for non-header clients. Default: "csrf_token".
	FieldName string

	// TrustedOrigins is an exact-match allowlist of origins (scheme+host, e.g.
	// "https://example.com"). Empty means no origin check is performed — use
	// this only in test environments or when all origins are trusted.
	TrustedOrigins []string

	// TokenSize is the number of random bytes in the token. Default: 32.
	TokenSize int

	// AllowInsecure disables the Secure flag on the CSRF cookie. Required for
	// HTTP-only local dev environments. Never set in production.
	AllowInsecure bool
}

// CSRFOption is a functional option for NewCSRF.
type CSRFOption func(*CSRF)

// WithCSRFCookieName overrides the default CSRF cookie name.
func WithCSRFCookieName(name string) CSRFOption {
	return func(c *CSRF) { c.CookieName = name }
}

// WithCSRFHeaderName overrides the default CSRF header name.
func WithCSRFHeaderName(name string) CSRFOption {
	return func(c *CSRF) { c.HeaderName = name }
}

// WithCSRFFieldName overrides the default CSRF form field name.
func WithCSRFFieldName(name string) CSRFOption {
	return func(c *CSRF) { c.FieldName = name }
}

// WithTrustedOrigins sets the exact-match origin allowlist.
func WithTrustedOrigins(origins ...string) CSRFOption {
	return func(c *CSRF) { c.TrustedOrigins = origins }
}

// WithCSRFTokenSize overrides the random token size in bytes.
func WithCSRFTokenSize(n int) CSRFOption {
	return func(c *CSRF) { c.TokenSize = n }
}

// WithCSRFAllowInsecure disables the Secure cookie flag. For local dev only.
func WithCSRFAllowInsecure() CSRFOption {
	return func(c *CSRF) { c.AllowInsecure = true }
}

// NewCSRF constructs a CSRF with default values, applying opts.
func NewCSRF(opts ...CSRFOption) *CSRF {
	c := &CSRF{
		CookieName: defaultCSRFCookieName,
		HeaderName: defaultCSRFHeaderName,
		FieldName:  defaultCSRFFieldName,
		TokenSize:  defaultCSRFTokenSize,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Issue generates a fresh CSRF token, sets it as a cookie on w, and returns
// the raw token string. The caller may embed the token in a rendered page or
// form; JavaScript helpers can also read it from the cookie directly.
//
// The cookie is NOT HttpOnly by design: client-side JS must be able to copy
// the value into the X-CSRF-Token header or csrf_token form field.
func (c *CSRF) Issue(w http.ResponseWriter) (string, error) {
	raw := make([]byte, c.TokenSize)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("auth: csrf: generate token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	cookie := &http.Cookie{
		Name:     c.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // intentional — JS must read this value
		SameSite: http.SameSiteStrictMode,
		Secure:   !c.AllowInsecure,
	}
	http.SetCookie(w, cookie)
	return token, nil
}

// Verify validates the CSRF token on a state-changing request. It:
//  1. Reads the CSRF cookie.
//  2. Reads the token from the X-CSRF-Token header (falling back to the
//     csrf_token form field for multipart/form-encoded submissions).
//  3. Compares them in constant time.
//  4. Validates the Origin (or Referer) against TrustedOrigins when set.
//
// Returns ErrCSRFInvalid on any failure. Safe (GET/HEAD/OPTIONS) methods
// pass through without any check.
func (c *CSRF) Verify(r *http.Request) error {
	if !stateMutating[r.Method] {
		return nil
	}

	// Origin / Referer check.
	if len(c.TrustedOrigins) > 0 {
		if err := c.checkOrigin(r); err != nil {
			return err
		}
	}

	// Double-submit token check.
	cookie, err := r.Cookie(c.CookieName)
	if err != nil {
		return ErrCSRFInvalid
	}
	cookieVal := cookie.Value

	submitted := r.Header.Get(c.HeaderName)
	if submitted == "" {
		// Fall through to form field.
		submitted = r.FormValue(c.FieldName)
	}
	if submitted == "" {
		return ErrCSRFInvalid
	}

	if subtle.ConstantTimeCompare([]byte(cookieVal), []byte(submitted)) != 1 {
		return ErrCSRFInvalid
	}
	return nil
}

// Middleware wraps an http.Handler: GET requests receive a new CSRF token
// (via Issue); state-changing requests are validated (via Verify) before
// the next handler is called.
func (c *CSRF) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if stateMutating[r.Method] {
			if err := c.Verify(r); err != nil {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
		} else if r.Method == http.MethodGet {
			if _, err := c.Issue(w); err != nil {
				http.Error(w, "auth: csrf: failed to issue token", http.StatusInternalServerError)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// checkOrigin validates the Origin (or Referer) header against TrustedOrigins.
func (c *CSRF) checkOrigin(r *http.Request) error {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Fall back to Referer.
		if ref := r.Header.Get("Referer"); ref != "" {
			u, err := url.Parse(ref)
			if err == nil {
				origin = strings.TrimSuffix(u.Scheme+"://"+u.Host, "/")
			}
		}
	}
	if origin == "" {
		return ErrCSRFInvalid
	}

	for _, trusted := range c.TrustedOrigins {
		if origin == trusted {
			return nil
		}
	}
	return ErrCSRFInvalid
}
