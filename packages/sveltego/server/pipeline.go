package server

import (
	"net/http"
	"strconv"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/render"
	"github.com/binsarjr/sveltego/runtime/router"
)

// hasAnyLayoutLoader reports whether at least one entry in loaders is
// non-nil. The pipeline skips LoadCtx allocation when both the route
// Load and every layout loader are absent.
func hasAnyLayoutLoader(loaders []router.LayoutLoadHandler) bool {
	for _, l := range loaders {
		if l != nil {
			return true
		}
	}
	return false
}

// handle is the request lifecycle: match, branch on +server.go vs page,
// run Load if present, render the page into a pooled buffer, and write
// the response with Content-Type and Content-Length set from the buffer.
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	route, params, ok := s.tree.Match(r.URL.Path)
	if !ok {
		s.notFound(w, r)
		return
	}
	if len(route.Server) > 0 {
		h := route.Server[r.Method]
		if h != nil {
			h(w, r)
			return
		}
		s.methodNotAllowed(w, r, methodsOf(route.Server))
		return
	}
	if route.Page == nil {
		s.notFound(w, r)
		return
	}

	var (
		data        any
		cookies     *kit.Cookies
		layoutDatas []any
	)
	if route.Load != nil || hasAnyLayoutLoader(route.LayoutLoaders) {
		lctx := kit.NewLoadCtx(r, params)
		layoutDatas = make([]any, len(route.LayoutChain))
		for i, layoutLoad := range route.LayoutLoaders {
			if layoutLoad == nil {
				continue
			}
			d, err := layoutLoad(lctx)
			if err != nil {
				s.handleLoadError(w, r, err)
				return
			}
			layoutDatas[i] = d
			lctx.PushParent(d)
		}
		if route.Load != nil {
			d, err := route.Load(lctx)
			if err != nil {
				s.handleLoadError(w, r, err)
				return
			}
			data = d
		}
		cookies = lctx.Cookies
	}

	buf := render.Acquire()
	defer render.Release(buf)

	rctx := kit.NewRenderCtx(r, w, params)
	if cookies != nil {
		rctx.Cookies = cookies
	}
	inner := func(buf *render.Writer) error {
		return route.Page(buf, rctx, data)
	}
	for i := len(route.LayoutChain) - 1; i >= 0; i-- {
		layout := route.LayoutChain[i]
		var layoutData any
		if i < len(layoutDatas) {
			layoutData = layoutDatas[i]
		}
		next := inner
		inner = func(buf *render.Writer) error {
			return layout(buf, rctx, layoutData, next)
		}
	}

	buf.WriteString(s.shellHead)
	buf.WriteString(s.shellMid)
	if err := inner(buf); err != nil {
		s.handleRenderError(w, r, err)
		return
	}
	buf.WriteString(s.shellTail)

	rctx.Cookies.Apply(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
