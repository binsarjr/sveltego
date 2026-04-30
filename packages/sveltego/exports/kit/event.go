package kit

import (
	"net/http"
	"net/url"
)

// RequestEvent is the request-scoped value handed to hooks.Handle and the
// resolve callback. Locals carries values derived inside Handle (session,
// request id, ...) into Load and the route handler. URL is the request's
// original URL; OriginalURL preserves the URL as it arrived even if a
// later step rewrites URL via Reroute.
//
// Fetch performs an outgoing HTTP request through the user-provided
// HandleFetch hook (when set) so server-side fetch traffic remains
// observable and rewritable from one place. Generated Load wrappers
// route their fetches through this method rather than http.DefaultClient.
type RequestEvent struct {
	Request     *http.Request
	URL         *url.URL
	OriginalURL *url.URL
	Params      map[string]string
	Locals      map[string]any
	Cookies     *Cookies

	// MatchPath is the URL path used for route matching after Reroute.
	// Empty means "no rewrite" — the router uses URL.Path.
	MatchPath string

	// responseHeader collects headers the user wants applied to the response,
	// including on error paths. Lazily initialized by ResponseHeader().
	responseHeader http.Header

	// RawParams holds the un-decoded route parameter values exactly as
	// they appear in the request path (e.g. "hello%20world" rather than
	// "hello world"). Populated by the pipeline after a successful route
	// match; nil before matching runs.
	RawParams map[string]string

	// fetcher is the chained HandleFetch implementation. nil means
	// "use http.DefaultClient.Do".
	fetcher HandleFetchFn
}

// ResponseHeader returns the mutable response header map for this event.
// Headers set here are applied to every response, including error responses,
// so user code can set WWW-Authenticate on 401s or clear cookies on errors.
// The map is lazily initialized on first call.
func (e *RequestEvent) ResponseHeader() http.Header {
	if e.responseHeader == nil {
		e.responseHeader = http.Header{}
	}
	return e.responseHeader
}

// NewRequestEvent constructs an event for r. Locals is initialized
// non-nil; Cookies is seeded from r. Params defaults to a non-nil empty
// map when nil. r must not be nil.
func NewRequestEvent(r *http.Request, params map[string]string) *RequestEvent {
	if params == nil {
		params = map[string]string{}
	}
	ev := &RequestEvent{
		Request: r,
		Params:  params,
		Locals:  map[string]any{},
		Cookies: NewCookies(r),
	}
	if r != nil {
		ev.URL = r.URL
		ev.OriginalURL = r.URL
	}
	return ev
}

// SetFetcher installs the HandleFetch implementation invoked by Fetch.
// The pipeline calls this once before Handle runs; user code does not.
func (e *RequestEvent) SetFetcher(fn HandleFetchFn) {
	if e == nil {
		return
	}
	e.fetcher = fn
}

// Fetch performs req through the configured HandleFetch hook, falling
// back to http.DefaultClient when no hook is installed. Generated Load
// wrappers reach for this method so HandleFetch can intercept every
// outbound request from user code.
func (e *RequestEvent) Fetch(req *http.Request) (*http.Response, error) {
	if e == nil || e.fetcher == nil {
		return http.DefaultClient.Do(req)
	}
	return e.fetcher(e, req)
}
