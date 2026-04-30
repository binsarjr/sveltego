package kit

import "net/http"

// PrerenderAuthGate decides whether a request authorised to receive the
// static HTML produced for a route declared `PrerenderProtected: true`.
// Returning false makes the server pipeline skip the prerendered hit and
// either fall through to the live SSR pipeline (so Handle/Load can issue
// a redirect or 401) or — when AuthGateRedirect returns a non-empty
// path — short-circuit with a 303 redirect.
//
// The auth pkg (#155 onwards) is held until the v0.6 cookie-session work
// lands. This interface is the stable hook the runtime calls; concrete
// gates plug in there without recompiling the runtime.
type PrerenderAuthGate interface {
	// Allow returns true when r is authorised to receive a protected
	// prerendered route's static HTML. Implementations must not write to
	// w; the pipeline owns the response. Returning false makes the
	// pipeline skip the static hit.
	Allow(r *http.Request) bool
}

// PrerenderAuthGateFunc adapts a plain function to the PrerenderAuthGate
// interface so callers can wire a closure without declaring a struct.
type PrerenderAuthGateFunc func(r *http.Request) bool

// Allow implements PrerenderAuthGate.
func (f PrerenderAuthGateFunc) Allow(r *http.Request) bool {
	if f == nil {
		return false
	}
	return f(r)
}

// DenyAllPrerenderAuth is the default gate. It always returns false so a
// PrerenderProtected route never serves the cached HTML until the
// embedding app supplies a real gate. Fail-closed by design: a misconfigured
// app must not leak a protected page.
var DenyAllPrerenderAuth PrerenderAuthGate = PrerenderAuthGateFunc(func(*http.Request) bool { return false })
