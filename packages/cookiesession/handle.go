package cookiesession

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/binsarjr/sveltego/exports/kit"
)

// localsKey returns a unique Locals map key for Session[T]. The key is
// package-prefixed and type-qualified so Handle[Foo] and Handle[Bar] never
// collide even when T shares a short name across packages.
func localsKey[T any]() string {
	var zero T
	return fmt.Sprintf("cookiesession.Session[%s]", reflect.TypeOf(&zero).Elem().String())
}

// CookieOption configures optional cookie attributes on the session cookie.
type CookieOption func(*cookieConfig)

type cookieConfig struct {
	maxAge   int // seconds; 0 = session cookie
	secure   *bool
	httpOnly bool
	sameSite http.SameSite
	domain   string
	path     string
}

func defaultCookieConfig() cookieConfig {
	return cookieConfig{
		httpOnly: true,
		sameSite: http.SameSiteLaxMode,
		path:     "/",
	}
}

// WithMaxAge sets the Max-Age attribute on the session cookie.
func WithMaxAge(seconds int) CookieOption {
	return func(c *cookieConfig) { c.maxAge = seconds }
}

// WithSecure overrides automatic HTTPS detection for the Secure attribute.
func WithSecure(secure bool) CookieOption {
	return func(c *cookieConfig) { c.secure = &secure }
}

// WithHTTPOnly sets the HttpOnly attribute on the session cookie.
func WithHTTPOnly(v bool) CookieOption {
	return func(c *cookieConfig) { c.httpOnly = v }
}

// WithSameSite sets the SameSite attribute on the session cookie.
func WithSameSite(s http.SameSite) CookieOption {
	return func(c *cookieConfig) { c.sameSite = s }
}

// WithDomain sets the Domain attribute on the session cookie.
func WithDomain(d string) CookieOption {
	return func(c *cookieConfig) { c.domain = d }
}

// WithPath sets the Path attribute on the session cookie.
func WithPath(p string) CookieOption {
	return func(c *cookieConfig) { c.path = p }
}

// Handle returns a kit.HandleFn middleware that provides a type-safe
// Session[T] for every request. It reads cookie chunks from the incoming
// request, decrypts via codec, and stashes the session in ev.Locals under
// a typed key. After the inner handler returns, any Set-Cookie headers
// produced by session mutations are forwarded to the kit.Response.
//
// Use kit.Sequence to compose multiple Handle middlewares:
//
//	kit.Sequence(cookiesession.Handle[MyData](codec, "sess"), myAuthHandle)
func Handle[T any](codec Codec, name string, opts ...CookieOption) kit.HandleFn {
	cfg := defaultCookieConfig()
	for _, o := range opts {
		o(&cfg)
	}

	sessionOpts := Options{
		Name:     name,
		SameSite: cfg.sameSite,
		Domain:   cfg.domain,
		Path:     cfg.path,
		Secure:   cfg.secure,
	}

	key := localsKey[T]()

	return func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
		w := &headerCapture{header: http.Header{}}
		sess, err := NewSession[T](ev.Request, w, codec, sessionOpts)
		if err != nil {
			// Fail-soft: tampered or unreadable cookie → start a fresh empty session.
			// The caller gets a zero T; the bad cookie is cleared on next flush.
			sess, _ = NewSession[T](nil, w, codec, sessionOpts)
		}

		ev.Locals[key] = sess

		resp, err := resolve(ev)
		if err != nil {
			return resp, err
		}

		// Forward Set-Cookie headers from session flush into the response.
		for _, line := range w.header["Set-Cookie"] {
			if resp == nil {
				resp = kit.NewResponse(http.StatusOK, nil)
			}
			if resp.Headers == nil {
				resp.Headers = http.Header{}
			}
			resp.Headers.Add("Set-Cookie", line)
		}

		if sess.NeedsSync() {
			if resp == nil {
				resp = kit.NewResponse(http.StatusOK, nil)
			}
			if resp.Headers == nil {
				resp.Headers = http.Header{}
			}
			resp.Headers.Set("Sveltego-Cookie-Session-Sync", "1")
		}

		return resp, nil
	}
}

// From retrieves the *Session[T] stashed by Handle[T] from ev.Locals.
// Returns (nil, false) when the middleware was not installed for this type.
func From[T any](ev *kit.RequestEvent) (*Session[T], bool) {
	raw, ok := ev.Locals[localsKey[T]()]
	if !ok {
		return nil, false
	}
	s, ok := raw.(*Session[T])
	return s, ok
}

// MustFrom retrieves the *Session[T] like From but panics with a clear
// message when the middleware is not installed or the type mismatches.
func MustFrom[T any](ev *kit.RequestEvent) *Session[T] {
	s, ok := From[T](ev)
	if !ok {
		panic("cookiesession.MustFrom: Handle[T] middleware not installed for " + localsKey[T]())
	}
	return s
}

// FromCtx retrieves the *Session[T] from a Load-level context. The pipeline
// shares the RequestEvent.Locals map with LoadCtx.Locals, so values stashed
// by Handle[T] are visible here without any additional wiring.
// Returns (nil, false) when the middleware was not installed for this type.
func FromCtx[T any](ctx *kit.LoadCtx) (*Session[T], bool) {
	raw, ok := ctx.Locals[localsKey[T]()]
	if !ok {
		return nil, false
	}
	s, ok := raw.(*Session[T])
	return s, ok
}

// headerCapture is a minimal http.ResponseWriter that captures response
// headers. Session.flush writes Set-Cookie headers here; Handle copies them
// into the kit.Response after resolve returns.
type headerCapture struct {
	header http.Header
}

func (h *headerCapture) Header() http.Header       { return h.header }
func (h *headerCapture) Write([]byte) (int, error) { return 0, nil }
func (h *headerCapture) WriteHeader(int)           {}
