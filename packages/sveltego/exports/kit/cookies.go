package kit

import (
	"net/http"
	"time"
)

// CookieOpts configures a Set-Cookie. Zero values mean "use defaults":
// Path defaults to "/", SameSite to Lax, Secure follows the request
// scheme (true for HTTPS), HttpOnly is true unless explicitly false via
// SetExposed or by passing a non-nil pointer to false.
type CookieOpts struct {
	Path     string
	Domain   string
	MaxAge   time.Duration
	Expires  time.Time
	HttpOnly *bool
	Secure   *bool
	SameSite http.SameSite
}

// Cookies is the request/response cookie jar threaded through LoadCtx
// and RenderCtx. Reads pull from the incoming request; writes accumulate
// in an outgoing queue flushed to the response by Apply before the
// pipeline calls WriteHeader.
type Cookies struct {
	in    map[string]string
	out   []*http.Cookie
	https bool
}

// NewCookies builds a Cookies jar seeded with the request's incoming
// cookies. Secure defaults are inferred from r.TLS. A nil request is
// tolerated and yields an empty jar with Secure defaulting to false.
func NewCookies(r *http.Request) *Cookies {
	c := &Cookies{in: map[string]string{}}
	if r == nil {
		return c
	}
	c.https = r.TLS != nil
	for _, ck := range r.Cookies() {
		c.in[ck.Name] = ck.Value
	}
	return c
}

// Get returns the incoming cookie value for name and ok=true if
// present. Outgoing Sets do not affect Get.
func (c *Cookies) Get(name string) (string, bool) {
	if c == nil {
		return "", false
	}
	v, ok := c.in[name]
	return v, ok
}

// Set queues a Set-Cookie for name=value with secure defaults applied
// (HttpOnly=true, SameSite=Lax, Path="/", Secure=request-scheme).
func (c *Cookies) Set(name, value string, opts CookieOpts) {
	if c == nil {
		return
	}
	c.out = append(c.out, c.build(name, value, opts, true))
}

// SetExposed queues a Set-Cookie like Set but defaults HttpOnly to
// false so client-side JS can read it. Caller can still explicitly set
// HttpOnly=true in opts to override.
func (c *Cookies) SetExposed(name, value string, opts CookieOpts) {
	if c == nil {
		return
	}
	c.out = append(c.out, c.build(name, value, opts, false))
}

// Delete queues a Set-Cookie that clears name on the client by emitting
// MaxAge=-1 and Expires=epoch. Path/Domain in opts must match the path
// the cookie was originally set at; Path defaults to "/".
func (c *Cookies) Delete(name string, opts CookieOpts) {
	if c == nil {
		return
	}
	if opts.Path == "" {
		opts.Path = "/"
	}
	c.out = append(c.out, &http.Cookie{
		Name:    name,
		Value:   "",
		Path:    opts.Path,
		Domain:  opts.Domain,
		MaxAge:  -1,
		Expires: time.Unix(0, 0),
	})
}

// Apply writes every queued Set-Cookie header to w. Safe to call before
// WriteHeader; emits one header per cookie.
func (c *Cookies) Apply(w http.ResponseWriter) {
	if c == nil || w == nil {
		return
	}
	for _, ck := range c.out {
		http.SetCookie(w, ck)
	}
}

func (c *Cookies) build(name, value string, opts CookieOpts, httpOnlyDefault bool) *http.Cookie {
	if opts.Path == "" {
		opts.Path = "/"
	}
	if opts.SameSite == 0 {
		opts.SameSite = http.SameSiteLaxMode
	}
	secure := c.https
	if opts.Secure != nil {
		secure = *opts.Secure
	}
	httpOnly := httpOnlyDefault
	if opts.HttpOnly != nil {
		httpOnly = *opts.HttpOnly
	}
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     opts.Path,
		Domain:   opts.Domain,
		MaxAge:   int(opts.MaxAge.Seconds()),
		Expires:  opts.Expires,
		Secure:   secure,
		HttpOnly: httpOnly,
		SameSite: opts.SameSite,
	}
}
