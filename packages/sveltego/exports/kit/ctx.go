package kit

import (
	"net/http"
	"net/url"
)

// RenderCtx is the request-scoped context handed to generated Render
// methods across the SSR lifecycle (Load, Render, Hooks).
type RenderCtx struct {
	Locals  map[string]any
	URL     *url.URL
	Params  map[string]string
	Cookies *Cookies
	Request *http.Request
	Writer  http.ResponseWriter
}

// LoadCtx is the request-scoped context handed to user-written Load
// functions in +page.server.go.
type LoadCtx struct {
	Locals  map[string]any
	URL     *url.URL
	Params  map[string]string
	Cookies *Cookies
	Request *http.Request
}

// NewRenderCtx builds a RenderCtx for the given request, response, and
// route params. Locals and Cookies are initialized non-nil.
func NewRenderCtx(r *http.Request, w http.ResponseWriter, params map[string]string) *RenderCtx {
	ctx := &RenderCtx{
		Locals:  map[string]any{},
		Params:  params,
		Cookies: &Cookies{},
		Request: r,
		Writer:  w,
	}
	if r != nil {
		ctx.URL = r.URL
	}
	return ctx
}

// NewLoadCtx builds a LoadCtx for the given request and route params.
// Locals and Cookies are initialized non-nil.
func NewLoadCtx(r *http.Request, params map[string]string) *LoadCtx {
	ctx := &LoadCtx{
		Locals:  map[string]any{},
		Params:  params,
		Cookies: &Cookies{},
		Request: r,
	}
	if r != nil {
		ctx.URL = r.URL
	}
	return ctx
}
