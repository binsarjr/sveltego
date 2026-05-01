package kit

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// M is a convenience alias for an untyped map literal so user code in
// server.go writes `kit.M{"ok": true}` instead of `map[string]any{...}`.
type M = map[string]any

// JSON builds a Response carrying body marshaled as JSON with status. A
// marshaling error becomes a 500 plain-text Response so handlers do not
// need to guard json.Marshal themselves.
func JSON(status int, body any) *Response {
	buf, err := json.Marshal(body)
	if err != nil {
		fail := NewResponse(http.StatusInternalServerError, []byte("internal error: "+err.Error()))
		fail.Headers.Set("Content-Type", "text/plain; charset=utf-8")
		return fail
	}
	res := NewResponse(status, buf)
	res.Headers.Set("Content-Type", "application/json; charset=utf-8")
	res.Headers.Set("Content-Length", strconv.Itoa(len(buf)))
	return res
}

// Text builds a Response carrying body as text/plain with status.
func Text(status int, body string) *Response {
	buf := []byte(body)
	res := NewResponse(status, buf)
	res.Headers.Set("Content-Type", "text/plain; charset=utf-8")
	res.Headers.Set("Content-Length", strconv.Itoa(len(buf)))
	return res
}

// XML builds a Response carrying body as application/xml. Used by
// kit.SitemapBuilder.Bytes() callers and any _server.go route that
// emits XML directly.
func XML(status int, body []byte) *Response {
	res := NewResponse(status, body)
	res.Headers.Set("Content-Type", "application/xml; charset=utf-8")
	res.Headers.Set("Content-Length", strconv.Itoa(len(body)))
	return res
}

// NoContent returns an empty 204 Response.
func NoContent() *Response {
	return NewResponse(http.StatusNoContent, nil)
}

// MethodNotAllowed builds a 405 Response with a sorted Allow header.
// The body is a short plain-text marker; clients consult Allow.
func MethodNotAllowed(allowed []string) *Response {
	cleaned := make([]string, 0, len(allowed))
	for _, m := range allowed {
		if m == "" {
			continue
		}
		cleaned = append(cleaned, m)
	}
	sort.Strings(cleaned)
	res := Text(http.StatusMethodNotAllowed, "405 method not allowed\n")
	res.Headers.Set("Allow", strings.Join(cleaned, ", "))
	return res
}
