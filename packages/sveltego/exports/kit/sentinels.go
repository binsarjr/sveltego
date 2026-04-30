package kit

import "strconv"

// RedirectErr signals a redirect short-circuit from Load. Code is the
// HTTP status (303 for POST->GET, 307/308 to preserve method).
type RedirectErr struct {
	Code     int
	Location string
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
//
// Idiomatic Go: use an error return rather than panic. Callers write
// `return data, kit.Redirect(303, "/login")`. SvelteKit's `throw redirect`
// is JS-flavored; Go uses explicit error returns to keep stack traces
// honest and avoid panic-as-control-flow.
func Redirect(code int, location string) error {
	return &RedirectErr{Code: code, Location: location}
}

// Error returns an error that, when returned from Load, makes the
// pipeline emit an HTTP error response with the given status and message.
func Error(code int, message string) error {
	return &HTTPErr{Code: code, Message: message}
}

// Fail returns an error that, when returned from a form action, makes the
// pipeline re-render the page with Data exposed to the template. Returned
// from a Load (non-action context) it is logged and surfaces as a 500.
func Fail(code int, data any) error {
	return &FailErr{Code: code, Data: data}
}
