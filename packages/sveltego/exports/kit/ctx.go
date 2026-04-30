package kit

import (
	"net/http"
	"net/url"
	"strings"
)

// HeaderWriter exposes Set/Add/Del on a response header map. Returned by
// [LoadCtx.Header]; backed by the same http.Header the pipeline merges
// into the final response, so all mutations are immediately visible.
type HeaderWriter struct {
	h http.Header
}

// Set replaces all values for key with a single value, mirroring
// [net/http.Header.Set]. Use [HeaderWriter.Add] to append instead.
func (hw *HeaderWriter) Set(key, value string) {
	hw.h.Set(key, value)
}

// Add appends value to the existing values for key, mirroring
// [net/http.Header.Add]. Multiple calls with the same key accumulate
// values, which is the correct behavior for headers such as Set-Cookie,
// Link, and Vary.
func (hw *HeaderWriter) Add(key, value string) {
	hw.h.Add(key, value)
}

// Del removes all values for key, mirroring [net/http.Header.Del].
func (hw *HeaderWriter) Del(key string) {
	hw.h.Del(key)
}

// RenderCtx is the request-scoped context handed to generated Render
// methods across the SSR lifecycle (Load, Render, Hooks).
//
// Note: RenderCtx and [LoadCtx] share an identical set of request fields
// (Locals, URL, Params, RawParams, Cookies, Request). Consolidation into a
// shared embedded struct is deferred to v0.5 ctx work because it touches
// generated Render signatures across every page.
type RenderCtx struct {
	Locals map[string]any
	URL    *url.URL
	// OriginalURL is the request URL before any Reroute hook rewrote it.
	// Nil when no Reroute was applied (i.e. the request was served at its
	// original path). Use this to recover the inbound URL in error
	// boundaries and layouts rendered after a reroute.
	OriginalURL *url.URL
	Params      map[string]string
	RawParams   map[string]string
	Cookies     *Cookies
	Request     *http.Request
	Writer      http.ResponseWriter
}

// RawParam returns the un-decoded route parameter value for name exactly
// as it appears in the request path (e.g. "hello%20world" rather than
// "hello world"). Returns ("", false) when name is not a capture in the
// matched route.
func (c *RenderCtx) RawParam(name string) (string, bool) {
	v, ok := c.RawParams[name]
	return v, ok
}

// CSRFToken returns the per-request CSRF token the pipeline issued for
// this render, or the empty string when CSRF is disabled for the route.
// Codegen emits ctx.CSRFToken() into the hidden _csrf_token input on
// every POST form unless the form carries the nocsrf attribute.
func (c *RenderCtx) CSRFToken() string {
	if c == nil || c.Locals == nil {
		return ""
	}
	if v, ok := c.Locals[csrfTokenKey].(string); ok {
		return v
	}
	return ""
}

// LoadCtx is the request-scoped context handed to user-written Load
// functions in +page.server.go and +layout.server.go.
//
// Locals is the same map the Handle hook populated before any Load runs;
// reading it never requires calling [LoadCtx.Parent]. Values set by a
// parent layout's Load are only available after [LoadCtx.Parent] returns
// that layout's data — but values written by Handle (session, user,
// nonce, …) are always present immediately.
//
// parents stores layout Load() returns in outer→inner order; the pipeline
// pushes each layout result before invoking the next layer's Load. User
// code reads only the immediate parent through [LoadCtx.Parent].
type LoadCtx struct {
	// Locals is the shared per-request bag populated by Handle before any
	// Load runs. All layout and page Loads in the chain read the same map
	// without waiting for a parent Load to complete.
	Locals    map[string]any
	URL       *url.URL
	Params    map[string]string
	RawParams map[string]string
	Cookies   *Cookies
	Request   *http.Request
	parents   []any
	headers   http.Header
}

// Param returns the URL-decoded route parameter value for name and
// whether name is a capture in the matched route.
func (c *LoadCtx) Param(name string) (string, bool) {
	v, ok := c.Params[name]
	return v, ok
}

// RawParam returns the un-decoded route parameter value for name exactly
// as it appears in the request path (e.g. "hello%20world" rather than
// "hello world"). Returns ("", false) when name is not a capture in the
// matched route.
func (c *LoadCtx) RawParam(name string) (string, bool) {
	v, ok := c.RawParams[name]
	return v, ok
}

// Parent returns the immediate parent layout's loaded data, or nil when
// the current layer is the outermost. Children type-assert the result:
// `parent := ctx.Parent().(rootlayout.LayoutData)`.
func (c *LoadCtx) Parent() any {
	if len(c.parents) == 0 {
		return nil
	}
	return c.parents[len(c.parents)-1]
}

// PushParent appends data to the parent stack. Codegen-emitted glue calls
// this between layout Load() invocations so each layer sees its direct
// parent via [LoadCtx.Parent]. User code never calls this.
func (c *LoadCtx) PushParent(data any) {
	c.parents = append(c.parents, data)
}

// Header returns the response-header writer for this Load invocation.
// Callers use Set to replace a header, Add to append (e.g. multiple
// Set-Cookie or Vary entries), and Del to remove. All mutations are
// merged into the final HTTP response by the pipeline.
func (c *LoadCtx) Header() *HeaderWriter {
	if c.headers == nil {
		c.headers = http.Header{}
	}
	return &HeaderWriter{h: c.headers}
}

// CollectHeaders returns the accumulated response headers set during Load.
// The pipeline calls this after the full load chain completes and merges
// the result into the kit.Response.Headers map. User code never calls
// this directly.
func (c *LoadCtx) CollectHeaders() http.Header {
	return c.headers
}

// Speculative reports whether this Load was triggered by a speculative
// prefetch rather than a real navigation. Use it to skip expensive work
// (analytics recording, rate-limit counters, cache warming) that should
// not fire on hover-triggered preloads.
//
// The server detects speculation via two headers, both treated as hints
// (not security boundaries — a caller can forge them):
//
//   - X-Sveltego-Preload: 1  — emitted by the sveltego client SPA router
//     on every __data.json preload fetch (#40).
//   - Sec-Purpose: prefetch   — standardized HTTP hint emitted by browsers
//     on speculative preload requests (RFC 8941 / Fetch spec).
func (c *LoadCtx) Speculative() bool {
	// DO NOT promote to a security boundary. These headers are user-forgeable
	// hints. Gate analytics / rate-limit suppression here, not access control.
	if c.Request == nil {
		return false
	}
	if c.Request.Header.Get("X-Sveltego-Preload") == "1" {
		return true
	}
	// Sec-Purpose may carry multiple tokens; "prefetch" is sufficient.
	if strings.Contains(c.Request.Header.Get("Sec-Purpose"), "prefetch") {
		return true
	}
	return false
}

// NewRenderCtx builds a RenderCtx for the given request, response, and
// route params. Locals and Cookies are initialized non-nil.
func NewRenderCtx(r *http.Request, w http.ResponseWriter, params map[string]string) *RenderCtx {
	ctx := &RenderCtx{
		Locals:  map[string]any{},
		Params:  params,
		Cookies: NewCookies(r),
		Request: r,
		Writer:  w,
	}
	if r != nil {
		ctx.URL = r.URL
	}
	return ctx
}

// NewLoadCtx builds a LoadCtx for the given request and route params.
// Locals and Cookies are initialized non-nil. The server pipeline replaces
// Locals with the shared [RequestEvent.Locals] map so Handle-populated
// values are visible to every Load without requiring a [LoadCtx.Parent] call.
func NewLoadCtx(r *http.Request, params map[string]string) *LoadCtx {
	ctx := &LoadCtx{
		Locals:  map[string]any{},
		Params:  params,
		Cookies: NewCookies(r),
		Request: r,
	}
	if r != nil {
		ctx.URL = r.URL
	}
	return ctx
}
