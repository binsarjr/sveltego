package kit

import (
	"net/http"
	"strconv"
)

// RedirectOption configures a RedirectErr. Apply options via the variadic
// argument to Redirect; the zero set leaves all flags at their default.
type RedirectOption func(*RedirectErr)

// RedirectReload returns an option that marks the redirect as requiring a
// full document reload. The pipeline sets X-Sveltego-Reload: 1 on the
// response so the client router performs location.assign() instead of an
// in-place SPA navigation. Use this for auth state changes, locale
// switches, or any flow where the SPA shell may hold stale hydration data.
//
// Client-side honor of the header lands with the SPA router (#37).
func RedirectReload() RedirectOption {
	return func(r *RedirectErr) { r.ForceReload = true }
}

// RedirectErr signals a redirect short-circuit from Load. Code is the
// HTTP status (303 for POST->GET, 307/308 to preserve method).
// ForceReload, when true, instructs the client router to perform a full
// document fetch rather than an SPA navigation.
type RedirectErr struct {
	Code        int
	Location    string
	ForceReload bool
}

// Error implements error.
func (r *RedirectErr) Error() string {
	return "redirect " + strconv.Itoa(r.Code) + " -> " + r.Location
}

// HTTPStatus reports the redirect status code; the pipeline uses this
// alongside the Location header.
func (r *RedirectErr) HTTPStatus() int { return r.Code }

// HTTPErr signals an HTTP-level error short-circuit (404, 500, ...).
type HTTPErr struct {
	Code    int
	Message string
}

// Error implements error.
func (h *HTTPErr) Error() string { return h.Message }

// HTTPStatus reports the response status code.
func (h *HTTPErr) HTTPStatus() int { return h.Code }

// FailErr signals a form action validation failure. Data is the per-field
// error map or shape the page re-renders; Code is the response status.
//
// Phase 0m-X2 ships the value type and pipeline detection; full action
// re-render with Data injection lands with form actions (#30).
type FailErr struct {
	Code int
	Data any
}

// Error implements error.
func (f *FailErr) Error() string { return "fail " + strconv.Itoa(f.Code) }

// HTTPStatus reports the response status code.
func (f *FailErr) HTTPStatus() int { return f.Code }

// Redirect returns an error that, when returned from Load, makes the
// pipeline emit an HTTP redirect with the given status and Location.
// Pass kit.RedirectReload() to force a full document reload on the client.
//
// Idiomatic Go: use an error return rather than panic. Callers write
// `return data, kit.Redirect(303, "/login")`. SvelteKit's `throw redirect`
// is JS-flavored; Go uses explicit error returns to keep stack traces
// honest and avoid panic-as-control-flow.
func Redirect(code int, location string, opts ...RedirectOption) error {
	r := &RedirectErr{Code: code, Location: location}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Error returns an error that, when returned from Load, makes the
// pipeline emit an HTTP error response with the given status. When message
// is omitted, Message defaults to http.StatusText(code).
func Error(code int, message ...string) error {
	msg := http.StatusText(code)
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return &HTTPErr{Code: code, Message: msg}
}

// Fail returns an error that, when returned from a form action, makes the
// pipeline re-render the page with Data exposed to the template. Returned
// from a Load (non-action context) it is logged and surfaces as a 500.
func Fail(code int, data any) error {
	return &FailErr{Code: code, Data: data}
}

// HTTPError is implemented by user-defined error types that want to carry
// their own HTTP status code and a safe public message into the pipeline.
// When the pipeline encounters an error satisfying this interface (via
// errors.As), it converts it to a kit.Error automatically — so handlers
// can return rich domain errors directly without manual rewrapping.
//
// Example:
//
//	type NotFoundError struct{ ID string }
//
//	func (e *NotFoundError) Error() string  { return "not found: " + e.ID }
//	func (e *NotFoundError) Status() int    { return http.StatusNotFound }
//	func (e *NotFoundError) Public() string { return "The requested item does not exist." }
//
//	// In Load:
//	return nil, &NotFoundError{ID: id}  // pipeline renders _error.svelte with 404
type HTTPError interface {
	error
	// Status returns the HTTP status code for this error (e.g. 404, 409).
	Status() int
	// Public returns a message safe to expose to end users.
	Public() string
}
