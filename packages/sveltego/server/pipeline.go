package server

import (
	"net/http"
	"strconv"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/render"
)

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

	var data any
	if route.Load != nil {
		lctx := kit.NewLoadCtx(r, params)
		d, err := route.Load(lctx)
		if err != nil {
			s.handleLoadError(w, r, err)
			return
		}
		data = d
	}

	buf := render.Acquire()
	defer render.Release(buf)

	buf.WriteString(s.shellHead)
	buf.WriteString(s.shellMid)
	rctx := kit.NewRenderCtx(r, w, params)
	if err := route.Page(buf, rctx, data); err != nil {
		s.handleRenderError(w, r, err)
		return
	}
	buf.WriteString(s.shellTail)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
