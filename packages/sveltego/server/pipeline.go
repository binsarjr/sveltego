package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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

// errServerRouteWrote is the sentinel resolve returns when a +server.go
// handler has already written directly to the http.ResponseWriter and
// the surrounding pipeline must not emit anything else.
var errServerRouteWrote = errors.New("server: server route wrote response")

// handle is the request entry point. It builds a RequestEvent, runs the
// optional Reroute hook, then dispatches through the user's Handle (or
// kit.IdentityHandle when none was authored). The inner resolve closure
// performs the existing match → load → render path and either writes
// directly (server routes) or returns a buffered *kit.Response.
//
// Panics in Handle, Load, or Render are recovered, wrapped, and routed
// through HandleError so the user-supplied hook can sanitize them. This
// is the one explicit panic-recovery boundary the framework owns.
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	ev := kit.NewRequestEvent(r, nil)
	ev.SetFetcher(s.hooks.HandleFetch)

	if rewritten := s.hooks.Reroute(ev.URL); rewritten != "" {
		ev.MatchPath = rewritten
	}

	resolve := func(ev *kit.RequestEvent) (*kit.Response, error) {
		return s.resolve(w, r, ev)
	}

	res, err := s.runHandle(ev, resolve)
	if err != nil {
		if errors.Is(err, errServerRouteWrote) {
			return
		}
		s.handlePipelineError(w, r, ev, err)
		return
	}
	if res == nil {
		return
	}
	s.writeResponse(w, ev, res)
}

// runHandle invokes the configured Handle hook with panic recovery so a
// panic anywhere in Handle, Load, or Render surfaces as a regular error
// the rest of the pipeline can route through HandleError.
func (s *Server) runHandle(ev *kit.RequestEvent, resolve kit.ResolveFn) (res *kit.Response, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			if recErr, ok := rec.(error); ok {
				err = fmt.Errorf("server: pipeline panic: %w", recErr)
				return
			}
			err = fmt.Errorf("server: pipeline panic: %v", rec)
		}
	}()
	return s.hooks.Handle(ev, resolve)
}

// resolve runs the SvelteKit-shaped match → load → render path and
// returns either a buffered Response (page routes) or errServerRouteWrote
// (server routes wrote directly via the user's http.HandlerFunc).
func (s *Server) resolve(w http.ResponseWriter, r *http.Request, ev *kit.RequestEvent) (*kit.Response, error) {
	matchPath := ev.MatchPath
	if matchPath == "" {
		matchPath = ev.URL.Path
	}
	route, params, ok := s.tree.Match(matchPath)
	if !ok {
		// Retry with the toggled trailing slash so a Never route hit
		// at /about/ or an Always route hit at /about can redirect to
		// the canonical form. The retry only fires when no original
		// match existed; routes whose canonical form is /about/ still
		// match /about exactly without this fallback.
		alt := togglePathSlash(matchPath)
		if alt != matchPath {
			if r2, p2, ok2 := s.tree.Match(alt); ok2 {
				route, params, ok = r2, p2, ok2
			}
		}
	}
	if !ok {
		return nil, kit.SafeError{Code: http.StatusNotFound, Message: http.StatusText(http.StatusNotFound)}
	}
	for k, v := range params {
		ev.Params[k] = v
	}

	if redirect := trailingSlashRedirect(ev.URL, route.Options.TrailingSlash); redirect != "" {
		return &kit.Response{
			Status:  http.StatusPermanentRedirect,
			Headers: http.Header{"Location": []string{redirect}},
		}, nil
	}

	if len(route.Server) > 0 {
		h := route.Server[r.Method]
		if h == nil {
			s.methodNotAllowed(w, r, methodsOf(route.Server))
			return nil, errServerRouteWrote
		}
		h(w, r)
		return nil, errServerRouteWrote
	}
	if route.Page == nil {
		return nil, kit.SafeError{Code: http.StatusNotFound, Message: http.StatusText(http.StatusNotFound)}
	}
	if !optionsAllowSSR(route.Options) {
		return s.renderEmptyShell(), nil
	}
	return s.renderPage(r, ev, route)
}

// optionsAllowSSR returns true unless the route declared SSR=false. The
// codegen-time cascade fills SSR=true by default; the field only flips
// to false when user code explicitly opted out. Routes with the
// zero-value PageOptions (no codegen Options emitted yet) also pass
// because the zero value carries TrailingSlashDefault and false SSR
// together — used here as a proxy for "no options were resolved" so
// the legacy render path stays the default for older manifests.
func optionsAllowSSR(opts kit.PageOptions) bool {
	if opts == (kit.PageOptions{}) {
		return true
	}
	return opts.SSR
}

// renderEmptyShell builds a Response carrying just the app shell with
// an empty mount point. Used when route.Options.SSR is false: the
// browser receives a valid HTML document and hydrates from the client
// bundle once delivered (#34). Cookies queued during Reroute / Handle
// still flow through writeResponse.
func (s *Server) renderEmptyShell() *kit.Response {
	body := s.shellHead + s.shellMid + `<div id="app"></div>` + s.shellTail
	headers := http.Header{}
	headers.Set("Content-Type", "text/html; charset=utf-8")
	headers.Set("Content-Length", strconv.Itoa(len(body)))
	return &kit.Response{
		Status:  http.StatusOK,
		Headers: headers,
		Body:    []byte(body),
	}
}

// togglePathSlash flips path's trailing slash. "/" returns "/"; "/x"
// returns "/x/"; "/x/" returns "/x". Used to retry a route match when
// the request and the route's canonical form differ only by a slash.
func togglePathSlash(path string) string {
	if path == "/" || path == "" {
		return path
	}
	if strings.HasSuffix(path, "/") {
		return strings.TrimRight(path, "/")
	}
	return path + "/"
}

// trailingSlashRedirect returns the canonical path when the request's
// trailing slash disagrees with the route policy, or the empty string
// when no redirect is needed. The canonical path preserves the URL
// query string. Root "/" is exempt from the Never policy because
// stripping its slash would yield an empty path.
func trailingSlashRedirect(u *url.URL, policy kit.TrailingSlash) string {
	if u == nil {
		return ""
	}
	path := u.Path
	switch policy {
	case kit.TrailingSlashAlways:
		if !strings.HasSuffix(path, "/") {
			return canonicalRedirect(u, path+"/")
		}
	case kit.TrailingSlashNever:
		if path != "/" && strings.HasSuffix(path, "/") {
			return canonicalRedirect(u, strings.TrimRight(path, "/"))
		}
	}
	return ""
}

func canonicalRedirect(u *url.URL, path string) string {
	out := *u
	out.Path = path
	out.Scheme = ""
	out.Host = ""
	out.User = nil
	return out.RequestURI()
}

// renderPage runs the load chain and renders the page into a fresh
// buffer, returning a Response carrying the rendered HTML, status, and
// the Set-Cookie headers accumulated by Load handlers.
func (s *Server) renderPage(r *http.Request, ev *kit.RequestEvent, route *router.Route) (*kit.Response, error) {
	var (
		data        any
		layoutDatas []any
	)
	if route.Load != nil || hasAnyLayoutLoader(route.LayoutLoaders) {
		lctx := kit.NewLoadCtx(r, ev.Params)
		lctx.Locals = ev.Locals
		lctx.Cookies = ev.Cookies
		layoutDatas = make([]any, len(route.LayoutChain))
		for i, layoutLoad := range route.LayoutLoaders {
			if layoutLoad == nil {
				continue
			}
			d, err := layoutLoad(lctx)
			if err != nil {
				return nil, err
			}
			layoutDatas[i] = d
			lctx.PushParent(d)
		}
		if route.Load != nil {
			d, err := route.Load(lctx)
			if err != nil {
				return nil, err
			}
			data = d
		}
	}

	buf := render.Acquire()
	defer render.Release(buf)

	rctx := &kit.RenderCtx{
		Locals:  ev.Locals,
		URL:     ev.URL,
		Params:  ev.Params,
		Cookies: ev.Cookies,
		Request: r,
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
		return nil, err
	}
	buf.WriteString(s.shellTail)

	body := make([]byte, buf.Len())
	copy(body, buf.Bytes())

	headers := http.Header{}
	headers.Set("Content-Type", "text/html; charset=utf-8")
	headers.Set("Content-Length", strconv.Itoa(len(body)))
	return &kit.Response{
		Status:  http.StatusOK,
		Headers: headers,
		Body:    body,
	}, nil
}

// writeResponse flushes a Response built by Handle (or its short-circuit
// path) to the underlying ResponseWriter. Cookies queued during Load are
// applied first so they appear before WriteHeader.
func (s *Server) writeResponse(w http.ResponseWriter, ev *kit.RequestEvent, res *kit.Response) {
	if ev.Cookies != nil {
		ev.Cookies.Apply(w)
	}
	for k, vs := range res.Headers {
		w.Header()[k] = vs
	}
	status := res.Status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	if len(res.Body) > 0 {
		_, _ = w.Write(res.Body)
	}
}
