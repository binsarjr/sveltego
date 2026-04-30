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
// functions in +page.server.go and +layout.server.go.
//
// parents stores layout Load() returns in outer→inner order; the pipeline
// pushes each layout result before invoking the next layer's Load. User
// code reads only the immediate parent through [LoadCtx.Parent].
type LoadCtx struct {
	Locals  map[string]any
	URL     *url.URL
	Params  map[string]string
	Cookies *Cookies
	Request *http.Request
	parents []any
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
// Locals and Cookies are initialized non-nil.
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
