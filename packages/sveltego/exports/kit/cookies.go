package kit

import (
	"net/http"
	"time"
)

// Cookies is the request/response cookie jar surface. The full
// implementation lands with issue #28 in v0.2; for MVP this exists so
// codegen output and user route code referencing kit.Cookies compiles.
// All methods are stubs returning zero values.
type Cookies struct{}

// Get returns the cookie value for name, or "" if absent. MVP stub:
// always returns "".
func (c *Cookies) Get(_ string) string {
	return ""
}

// Set queues a Set-Cookie header. MVP stub: no-op.
func (c *Cookies) Set(_, _ string, _ ...CookieOption) {}

// Delete queues a Set-Cookie clear for name. MVP stub: no-op.
func (c *Cookies) Delete(_ string) {}

// CookieOption configures Set behavior. MVP stub: option constructors
// produce functions that mutate cookieOpts but the jar does not yet
// consume those values.
type CookieOption func(*cookieOpts)

type cookieOpts struct {
	Path     string
	Domain   string
	MaxAge   int
	Secure   bool
	HTTPOnly bool
	SameSite http.SameSite
	Expires  time.Time
}

// Path sets the Path attribute. MVP stub: stored but unused.
func Path(p string) CookieOption {
	return func(o *cookieOpts) { o.Path = p }
}

// Domain sets the Domain attribute. MVP stub: stored but unused.
func Domain(d string) CookieOption {
	return func(o *cookieOpts) { o.Domain = d }
}

// MaxAge sets the Max-Age attribute in seconds. MVP stub: stored but unused.
func MaxAge(seconds int) CookieOption {
	return func(o *cookieOpts) { o.MaxAge = seconds }
}

// Secure sets the Secure flag. MVP stub: stored but unused.
func Secure() CookieOption {
	return func(o *cookieOpts) { o.Secure = true }
}

// HTTPOnly sets the HttpOnly flag. MVP stub: stored but unused.
func HTTPOnly() CookieOption {
	return func(o *cookieOpts) { o.HTTPOnly = true }
}

// SameSite sets the SameSite attribute. MVP stub: stored but unused.
func SameSite(s http.SameSite) CookieOption {
	return func(o *cookieOpts) { o.SameSite = s }
}

// Expires sets the Expires attribute. MVP stub: stored but unused.
func Expires(t time.Time) CookieOption {
	return func(o *cookieOpts) { o.Expires = t }
}
