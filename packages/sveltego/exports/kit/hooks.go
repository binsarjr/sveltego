package kit

import (
	"context"
	"net/http"
	"net/url"
)

// Response is the result a hook returns from resolve or short-circuits
// with directly. The pipeline writes Status, Headers, then Body in that
// order. A zero Status defaults to http.StatusOK at write time.
//
// Body is opaque bytes — generated render code populates it from the
// pooled render buffer; hook authors short-circuit by constructing a
// Response with their own bytes.
type Response struct {
	Status  int
	Headers http.Header
	Body    []byte
}

// NewResponse returns a Response pre-allocated with non-nil Headers and
// the given status (defaulting to http.StatusOK when status is 0).
func NewResponse(status int, body []byte) *Response {
	if status == 0 {
		status = http.StatusOK
	}
	return &Response{
		Status:  status,
		Headers: http.Header{},
		Body:    body,
	}
}

// ResolveFn advances the pipeline past Handle into route resolution.
// Hooks may call resolve once, multiple times, or not at all. Returning
// without calling resolve short-circuits the route handler.
type ResolveFn func(ev *RequestEvent) (*Response, error)

// HandleFn is the signature of the user-authored Handle hook in
// src/hooks.server.go. It wraps the entire request pipeline; ev carries
// request state and resolve invokes the route handler.
type HandleFn func(ev *RequestEvent, resolve ResolveFn) (*Response, error)

// HandleErrorFn is the signature of the optional HandleError hook. The
// pipeline calls it whenever Handle, Load, Render, or a +server.go
// handler returns an error and converts the SafeError into the user-
// facing HTTP response.
//
// Returning a non-nil error short-circuits the error boundary entirely.
// The returned error is handled by the same sentinel logic as pipeline
// errors: a *RedirectErr produces a redirect, a *HTTPErr produces a
// plain HTTP response. This lets a HandleError hook redirect
// unauthenticated users to /login instead of rendering +error.svelte.
// The short-circuit does NOT re-enter HandleError (no infinite loop).
type HandleErrorFn func(ev *RequestEvent, err error) (SafeError, error)

// HandleFetchFn is the signature of the optional HandleFetch hook. The
// pipeline plugs it into RequestEvent.Fetch so outbound HTTP traffic
// from Load and route handlers can be intercepted.
type HandleFetchFn func(ev *RequestEvent, req *http.Request) (*http.Response, error)

// RerouteFn is the signature of the optional Reroute hook. Reroute runs
// before route matching: returning a non-empty path rewrites the URL
// used for lookup while ev.URL is preserved as the original request URL.
// Returning the empty string means "no rewrite".
type RerouteFn func(u *url.URL) string

// InitFn is the signature of the optional Init hook. Init runs once at
// server start before the first request is processed. When Init returns
// an error the server does not crash: every incoming request receives a
// 500 response with the configured InitErrorHTML body. While Init is
// still running, requests that exceed InitTimeout receive a 503 response
// with the configured InitPendingHTML body.
type InitFn func(ctx context.Context) error

// SafeError is the user-facing error contract HandleError returns and
// the error boundary consumes. Code is the HTTP status, Message is the
// public-facing string, and ID is a correlation token for log lookups.
type SafeError struct {
	Code    int
	Message string
	ID      string
}

// Error implements error so SafeError can flow through error-typed
// pipeline branches. The string form prefers Message; absent that it
// falls back to the canonical text for Code (or "internal server error"
// when Code is also unset).
func (s SafeError) Error() string {
	if s.Message != "" {
		return s.Message
	}
	if text := http.StatusText(s.Code); text != "" {
		return text
	}
	return http.StatusText(http.StatusInternalServerError)
}

// HTTPStatus reports the response status code so the existing
// httpStatuser branch in the server pipeline routes SafeError correctly.
func (s SafeError) HTTPStatus() int {
	if s.Code == 0 {
		return http.StatusInternalServerError
	}
	return s.Code
}

// IdentityHandle is the default Handle hook installed when the user did
// not author one. It calls resolve and returns its result unchanged.
func IdentityHandle(ev *RequestEvent, resolve ResolveFn) (*Response, error) {
	return resolve(ev)
}

// IdentityHandleError is the default HandleError hook. It maps any error
// to a generic 500 SafeError without exposing internal detail.
func IdentityHandleError(_ *RequestEvent, _ error) (SafeError, error) {
	return SafeError{Code: http.StatusInternalServerError, Message: http.StatusText(http.StatusInternalServerError)}, nil
}

// IdentityHandleFetch is the default HandleFetch hook. It dispatches req
// through http.DefaultClient.
func IdentityHandleFetch(_ *RequestEvent, req *http.Request) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

// IdentityReroute is the default Reroute hook. It returns the empty
// string so the router uses the original URL path.
func IdentityReroute(_ *url.URL) string { return "" }

// IdentityInit is the default Init hook. It returns nil.
func IdentityInit(_ context.Context) error { return nil }

// Sequence composes multiple Handle hooks left-to-right. The first
// handler runs first; calling resolve advances to the next handler in
// the chain, and the innermost resolve invokes the original route
// resolver. Returning early from any handler short-circuits the rest.
func Sequence(handlers ...HandleFn) HandleFn {
	if len(handlers) == 0 {
		return IdentityHandle
	}
	return func(ev *RequestEvent, resolve ResolveFn) (*Response, error) {
		next := resolve
		for i := len(handlers) - 1; i >= 0; i-- {
			h := handlers[i]
			n := next
			next = func(ev *RequestEvent) (*Response, error) {
				return h(ev, n)
			}
		}
		return next(ev)
	}
}

// Hooks bundles every optional server hook. Generated code populates
// missing fields with the corresponding Identity* default before passing
// the value to the server. User code in src/hooks.server.go does not
// touch this type; the build wires it for them.
type Hooks struct {
	Handle      HandleFn
	HandleError HandleErrorFn
	HandleFetch HandleFetchFn
	Reroute     RerouteFn
	Init        InitFn
}

// DefaultHooks returns a Hooks bundle filled with identity defaults so a
// server with no user-authored hooks behaves exactly as if Handle is the
// identity passthrough and the rest are absent.
func DefaultHooks() Hooks {
	return Hooks{
		Handle:      IdentityHandle,
		HandleError: IdentityHandleError,
		HandleFetch: IdentityHandleFetch,
		Reroute:     IdentityReroute,
		Init:        IdentityInit,
	}
}

// WithDefaults returns h with any nil field replaced by the matching
// identity default. Idempotent: calling on an already-filled bundle is
// a no-op.
func (h Hooks) WithDefaults() Hooks {
	if h.Handle == nil {
		h.Handle = IdentityHandle
	}
	if h.HandleError == nil {
		h.HandleError = IdentityHandleError
	}
	if h.HandleFetch == nil {
		h.HandleFetch = IdentityHandleFetch
	}
	if h.Reroute == nil {
		h.Reroute = IdentityReroute
	}
	if h.Init == nil {
		h.Init = IdentityInit
	}
	return h
}
