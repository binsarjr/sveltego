package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// bytesAsString aliases p as a string without copying. Safe only when
// the caller guarantees p is not mutated for the lifetime of the
// returned string. Used to feed []byte into render.Writer.WriteRaw,
// which only appends bytes (no retention).
func bytesAsString(p []byte) string {
	if len(p) == 0 {
		return ""
	}
	return unsafe.String(&p[0], len(p))
}

// nonceBytes is the entropy source for a CSP nonce. 16 bytes (128 bits)
// matches the OWASP recommendation; base64-encoded that becomes a
// 22-character token (RawURLEncoding) safe to embed in HTML attributes.
const nonceBytes = 16

// generateNonce returns a fresh per-request CSP nonce using crypto/rand.
// Returns the empty string on rand failure; callers treat empty as "no
// nonce" and skip the header so a transient PRNG hiccup doesn't surface
// as a broken page.
func generateNonce() string {
	var buf [nonceBytes]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(buf[:])
}

// applyCSP generates a per-request nonce, stores it on ev.Locals, and
// sets the configured Content-Security-Policy header on w. No-op when
// s.csp.Mode is CSPOff. Runs before Handle so the header is present on
// success, error, and short-circuit paths alike.
func (s *Server) applyCSP(w http.ResponseWriter, ev *kit.RequestEvent) {
	if s.csp.Mode == kit.CSPOff {
		return
	}
	nonce := generateNonce()
	if nonce == "" {
		return
	}
	kit.SetNonce(ev, nonce)
	if name := kit.CSPHeaderName(s.csp.Mode); name != "" {
		w.Header().Set(name, s.cspTemplate.Build(nonce))
	}
}

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

// errServerRouteWrote is the sentinel resolve returns when a _server.go
// handler has already written directly to the http.ResponseWriter and
// the surrounding pipeline must not emit anything else.
var errServerRouteWrote = errors.New("server: server route wrote response")

// errStreamingWrote signals the streaming render path already wrote the
// chunked HTML response to the underlying http.ResponseWriter, so the
// surrounding pipeline must skip writeResponse and any error wrapping.
var errStreamingWrote = errors.New("server: streaming response wrote")

// afterDrainTimeout is the context deadline given to the After-callback
// drain phase. Slow callbacks are cancelled rather than holding the
// goroutine indefinitely.
const afterDrainTimeout = 30 * time.Second

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
	s.applyCSP(w, ev)

	if rewritten := s.hooks.Reroute(ev.URL); rewritten != "" {
		ev.MatchPath = rewritten
	}

	// pageBuf carries the pooled render.Writer ownership across the
	// Handle hook. renderPage hands the buffer's underlying bytes to
	// kit.Response.Body without copying, so the writer must outlive
	// writeResponse. Released here once the response is fully flushed.
	var (
		matched *router.Route
		pageBuf *render.Writer
	)
	defer func() {
		if pageBuf != nil {
			render.Release(pageBuf)
		}
	}()
	resolve := func(ev *kit.RequestEvent) (*kit.Response, error) {
		return s.resolve(w, r, ev, &matched, &pageBuf)
	}

	res, err := s.runHandle(ev, resolve)
	if err != nil {
		if !errors.Is(err, errServerRouteWrote) && !errors.Is(err, errStreamingWrote) {
			s.handlePipelineError(w, r, ev, matched, err)
		}
		// Response was already written (server route, streaming, or error
		// page). Drain any After callbacks before releasing resources.
		s.drainAfter(ev) //nolint:contextcheck // intentional: request ctx is cancelled on return; After runs on Background
		return
	}
	if res == nil {
		return
	}
	s.writeResponse(w, ev, res)
	s.drainAfter(ev) //nolint:contextcheck // intentional: request ctx is cancelled on return; After runs on Background
}

// drainAfter runs all functions queued via RequestEvent.After with a
// bounded context derived from context.Background (not the request
// context, which is cancelled once ServeHTTP returns). Errors logged
// here do not affect the already-sent response.
func (s *Server) drainAfter(ev *kit.RequestEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), afterDrainTimeout)
	defer cancel()
	kit.DrainAfter(ctx, ev)
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
// pageBuf receives the pooled render.Writer when renderPage produces a
// buffered Response so the caller can release it after writeResponse.
func (s *Server) resolve(w http.ResponseWriter, r *http.Request, ev *kit.RequestEvent, matched **router.Route, pageBuf **render.Writer) (*kit.Response, error) {
	matchPath := ev.MatchPath
	if matchPath == "" {
		matchPath = ev.URL.Path
	}
	// For __data.json requests, strip the virtual suffix before tree
	// matching so /blog/__data.json resolves to the same route as /blog.
	// We strip for any method here; method enforcement happens later in
	// renderDataJSON (GET only) or when POST is rejected with 405.
	if isDataJSONPath(r.URL.Path) {
		matchPath = strings.TrimSuffix(matchPath, "/__data.json")
		if matchPath == "" {
			matchPath = "/"
		}
	}
	matchedPath := matchPath
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
				matchedPath = alt
			}
		}
	}
	if !ok {
		return nil, kit.SafeError{Code: http.StatusNotFound, Message: http.StatusText(http.StatusNotFound)}
	}
	if matched != nil {
		*matched = route
	}
	for k, v := range params {
		ev.Params[k] = v
	}
	ev.RawParams = rawParamsFromPath(matchedPath, route.Segments, params)

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
	// Svelte-mode routes have no Go-side Page handler; the server
	// returns the shell + JSON hydration payload and Vite's client
	// bundle mounts and renders. As of RFC #379 phase 5, "svelte" is
	// the only template pipeline new routes use; routes without a
	// Templates value fall back to the legacy Page handler when one
	// is wired (kept for runtime tests that construct Routes by hand).
	isSvelteMode := route.Options.Templates == kit.TemplatesSvelte
	if !isSvelteMode && route.Page == nil {
		return nil, kit.SafeError{Code: http.StatusNotFound, Message: http.StatusText(http.StatusNotFound)}
	}
	var form *formData
	if isDataJSONPath(r.URL.Path) {
		if route.Options.SSROnly {
			return nil, kit.SafeError{Code: http.StatusNotFound, Message: http.StatusText(http.StatusNotFound)}
		}
		// Run applyCSRF for the GET-only __data.json path so the
		// payload carries `csrfToken` after SPA nav. Without this,
		// the second-and-later page rendered via the client router
		// has no token in its hydration payload, the splicer reads
		// undefined, and the user's first POST returns 403 (#541).
		s.applyCSRF(r, ev, route)
		return s.renderDataJSON(r, ev, route, nil)
	}
	if rejected := s.applyCSRF(r, ev, route); rejected != nil {
		if isEnhanceRequest(r) {
			return enhanceForbiddenResponse(), nil
		}
		return rejected, nil
	}
	if r.Method == http.MethodPost {
		res, fd, err := s.dispatchAction(r, ev, route)
		if err != nil {
			return nil, err
		}
		if isEnhanceRequest(r) {
			return enhanceResponse(res, fd), nil
		}
		if res != nil {
			return res, nil
		}
		form = fd
	}
	if !optionsAllowSSR(route.Options) {
		// Svelte-mode SPA routes (Templates=svelte, SSR=false) need
		// the Vite client bundle <script> tag and the JSON hydration
		// payload — without them the empty shell stays blank because
		// the browser has nothing to mount. renderSvelteShell already
		// runs the load chain, builds the payload, and injects asset
		// tags; reuse it here so SPA mode actually hydrates instead
		// of shipping the bare shell from renderEmptyShell.
		if isSvelteMode {
			return s.renderSvelteShell(r, ev, route, form)
		}
		return s.renderEmptyShell(ev), nil
	}
	// ADR 0009 phase 6 (#428): Svelte-mode routes with a generated
	// Render emit have route.Page wired to the bridge adapter that
	// dispatches usersrc.Render(payload, data) and writes payload.Body
	// into the page writer. Routes without a Render emit (no
	// _page.server.go, build skipped, Node missing, etc.) keep the
	// SPA shell fallback so the page still hydrates client-side.
	if isSvelteMode && route.Page == nil {
		return s.renderSvelteShell(r, ev, route, form)
	}
	return s.renderPage(w, r, ev, route, form, pageBuf)
}

// isDataJSONPath reports whether the URL path is a __data.json endpoint,
// regardless of method. Used to decide whether to strip the suffix before
// route matching.
func isDataJSONPath(path string) bool {
	return strings.HasSuffix(path, "/__data.json")
}

// isDataJSONRequest reports whether r is a direct XHR-style fetch of a
// route's __data.json endpoint. The SPA router (#37, #38) uses these
// requests to invalidate page data without a full navigation; SSROnly
// routes must reject them so callers fall back to a full document fetch.
func isDataJSONRequest(r *http.Request) bool {
	return r.Method == http.MethodGet && isDataJSONPath(r.URL.Path)
}

// optionsAllowSSR returns true unless the route declared SSR=false. The
// codegen-time cascade fills SSR=true by default; the field only flips
// to false when user code explicitly opted out. Routes with the
// zero-value PageOptions (no codegen Options emitted yet) also pass
// because the zero value carries TrailingSlashDefault and false SSR
// together — used here as a proxy for "no options were resolved" so
// the legacy render path stays the default for older manifests.
func optionsAllowSSR(opts kit.PageOptions) bool {
	if opts.Equal(kit.PageOptions{}) {
		return true
	}
	return opts.SSR
}

// renderEmptyShell builds a Response carrying just the app shell with
// an empty mount point. Used when route.Options.SSR is false: the
// browser receives a valid HTML document and hydrates from the client
// bundle once delivered (#34). Cookies queued during Reroute / Handle
// still flow through writeResponse. The service-worker registration
// tag (when enabled) inherits the per-request CSP nonce from ev.
func (s *Server) renderEmptyShell(ev *kit.RequestEvent) *kit.Response {
	body := s.shellHead + s.shellMid + `<div id="app"></div>` + s.serviceWorkerTag(kit.Nonce(ev)) + s.shellTail
	headers := http.Header{}
	headers.Set("Content-Type", "text/html; charset=utf-8")
	headers.Set("Content-Length", strconv.Itoa(len(body)))
	return &kit.Response{
		Status:  http.StatusOK,
		Headers: headers,
		Body:    []byte(body),
	}
}

// clientPayload is the JSON blob injected into SSR HTML and returned by
// the __data.json endpoint. The shape mirrors SvelteKit's inline payload
// convention, scoped to the sveltego namespace.
//
// Data holds the page data returned by Load() (and each layout loader in
// outer→inner order when LayoutData is non-nil). Form carries ActionData
// from a POST action on the same request. RouteID is the canonical route
// pattern used by the client router to look up the component. URL is the
// full request URL string. Params is the route param map (decoded). Status
// is the HTTP status (200 on the success path, form.code when a form
// action overrode it, or the SafeError code on error renders). PageError
// is non-nil only when the server is rendering the error boundary; it
// carries the safe public-facing message exposed to the page. Manifest
// is non-empty only on the initial SSR render so the SPA router (#37)
// can match link URLs and pick the right route module on subsequent
// navigations; __data.json fetches omit it because the client already
// has it from the first paint.
type clientPayload struct {
	RouteID    string                `json:"routeId"`
	Data       any                   `json:"data"`
	LayoutData []any                 `json:"layoutData,omitempty"`
	Form       any                   `json:"form"`
	URL        string                `json:"url"`
	Params     map[string]string     `json:"params"`
	Status     int                   `json:"status"`
	PageError  *clientPageError      `json:"error"`
	Manifest   []clientManifestEntry `json:"manifest,omitempty"`
	Deps       []string              `json:"deps,omitempty"`
	AppVersion string                `json:"appVersion,omitempty"`
	// CSRFToken carries the per-request double-submit token so the
	// client-side mount hook can splice a hidden `_csrf_token` input
	// into POST forms it renders into the empty SPA shell. Mirrors the
	// build-time AST splice (svelte_js2go/lower_csrf.go) and the runtime
	// sidecar splice (runtime/svelte/csrfinject) for the third path:
	// SPA / Static routes whose forms are constructed entirely in the
	// browser. Empty when the route opts out of CSRF.
	CSRFToken   string             `json:"csrfToken,omitempty"`
	VersionPoll *clientVersionPoll `json:"versionPoll,omitempty"`
}

// clientVersionPoll mirrors kit.VersionPollConfig on the wire. Emitted
// only when an AppVersion is known (ViteManifest supplied) so the
// client never spins a poller against a 404 endpoint. IntervalMS is
// the resolved cadence in milliseconds — matches the typed primitive
// JS expects from setTimeout/setInterval without a unit conversion on
// the client.
type clientVersionPoll struct {
	IntervalMS int64 `json:"intervalMs"`
	Disabled   bool  `json:"disabled,omitempty"`
}

// clientPageError mirrors SvelteKit's App.Error wire shape — a safe
// public-facing message paired with the HTTP status. Populated only on
// error-boundary renders; the success path emits PageError = nil so the
// client-side `page.error` reads as null.
type clientPageError struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}

// clientManifestEntry is one route descriptor shipped to the client SPA
// router. Pattern is the SvelteKit-canonical pattern (the same string
// used as RouteID), Segments is the parsed form so the client can match
// URLs without re-parsing the bracket syntax. Only routes with a Page
// handler are emitted; pure _server.go routes do not participate in SPA
// navigation.
type clientManifestEntry struct {
	Pattern  string                  `json:"pattern"`
	Segments []clientManifestSegment `json:"segments"`
}

// clientManifestSegment mirrors router.Segment for the wire. Kind uses
// the same numeric values as router.SegmentKind so the client can switch
// on it directly: 0=static, 1=param, 2=optional, 3=rest.
type clientManifestSegment struct {
	Kind  uint8  `json:"kind"`
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// buildClientManifest walks routes and returns the SPA router manifest
// ordered the same way the server tree was built (router preserves the
// caller-supplied order after sorting for specificity). Routes without a
// Page handler are skipped so the client never tries to mount a server-only
// route. The result is cached on the Server at New time.
func buildClientManifest(routes []router.Route) []clientManifestEntry {
	out := make([]clientManifestEntry, 0, len(routes))
	for i := range routes {
		r := &routes[i]
		if r.Page == nil {
			continue
		}
		segs := make([]clientManifestSegment, len(r.Segments))
		for j, s := range r.Segments {
			segs[j] = clientManifestSegment{
				Kind:  uint8(s.Kind),
				Name:  s.Name,
				Value: s.Value,
			}
		}
		out = append(out, clientManifestEntry{
			Pattern:  r.Pattern,
			Segments: segs,
		})
	}
	return out
}

// escapeScriptSpecialInPlace rewrites the input bytes so any "</" and
// "<!--" sequences are neutralized via "<", preventing a payload
// containing those bytes from terminating the enclosing <script> tag or
// opening an HTML comment. Operates on a fresh slice — the input is not
// retained — so callers may pass a shared buffer's slice safely.
func escapeScriptSpecial(raw []byte) []byte {
	if indexScriptSpecial(raw) < 0 {
		return raw
	}
	var out []byte
	i := 0
	for i < len(raw) {
		if i+1 < len(raw) && raw[i] == '<' && (raw[i+1] == '/' || raw[i+1] == '!') {
			out = append(out, raw[:i]...)
			out = append(out, '\\', 'u', '0', '0', '3', 'c')
			raw = raw[i+1:]
			i = 0
			continue
		}
		i++
	}
	out = append(out, raw...)
	return out
}

// buildClientPayload assembles a clientPayload from the data gathered
// during the load chain. formData is nil unless the request was a POST
// that ran an action. route.Pattern is used as RouteID. params is the
// decoded route-param map; it is always emitted (empty object when the
// route has no captures) so client code can iterate without nil checks.
// Status defaults to 200 on the success path; the caller swaps it for
// form.code when a form action overrode the status. ev carries the
// per-request CSRF token (issue #510) so SPA / Static routes that
// render forms entirely in the browser can splice the hidden
// `_csrf_token` input from a known-good value rather than parsing the
// `_csrf` cookie themselves.
func buildClientPayload(r *http.Request, ev *kit.RequestEvent, route *router.Route, data any, layoutDatas []any, params map[string]string, form *formData) clientPayload {
	p := clientPayload{
		RouteID: route.Pattern,
		Data:    data,
		URL:     r.URL.String(),
		Params:  params,
		Status:  http.StatusOK,
	}
	if p.Params == nil {
		p.Params = map[string]string{}
	}
	if len(layoutDatas) > 0 {
		// Copy only non-nil entries so the client doesn't receive sparse nils.
		lds := make([]any, 0, len(layoutDatas))
		for _, ld := range layoutDatas {
			lds = append(lds, ld)
		}
		p.LayoutData = lds
	}
	if form != nil {
		p.Form = form.data
		if form.code != 0 {
			p.Status = form.code
		}
	}
	if ev != nil {
		p.CSRFToken = kit.CSRFToken(ev)
	}
	return p
}

// applyInitialPayloadFields stamps the per-build SPA manifest, the
// build-version digest, and the resolved version-poll config onto an
// initial-render hydration payload. These fields belong on the very
// first paint only; subsequent __data.json fetches reuse the values
// already cached by the running client. Callers populate p.Deps
// separately because Deps comes from the per-request load context.
func (s *Server) applyInitialPayloadFields(p *clientPayload) {
	p.Manifest = s.clientManifest
	if s.appVersion != "" {
		p.AppVersion = s.appVersion
		p.VersionPoll = &clientVersionPoll{
			IntervalMS: s.versionPoll.PollInterval.Milliseconds(),
			Disabled:   s.versionPoll.Disabled,
		}
	}
}

// emitPayloadScriptTag writes the hydration payload as a JSON <script>
// tag into buf. Uses id "sveltego-data" so client entry.ts can read it
// via document.getElementById("sveltego-data"). The splice writer reuses
// pre-encoded stable fields owned by s; the assembled bytes are then
// passed through escapeScriptSpecial so any "</" or "<!--" inside
// per-request data can't break out of the script tag.
//
// nonce is the per-request CSP nonce (kit.Nonce(ev)); when non-empty
// the emitted tag carries nonce="…" so a strict
// `script-src 'nonce-…'` directive permits the JSON payload script
// alongside the entry chunk and modulepreload hints (#539).
func (s *Server) emitPayloadScriptTag(buf *render.Writer, p clientPayload, nonce string) {
	scratch := acquirePayloadBuf()
	defer releasePayloadBuf(scratch)
	openTag := `<script id="sveltego-data" type="application/json">`
	if nonce != "" {
		openTag = `<script id="sveltego-data" nonce="` + nonce + `" type="application/json">`
	}
	if err := s.writePayloadJSON(scratch, p); err != nil {
		// Emit an empty payload rather than omitting the tag; the client
		// will mount with nil data rather than crashing on a missing element.
		buf.WriteString(openTag)
		buf.WriteString(`{}</script>`)
		return
	}
	encoded := escapeScriptSpecial(scratch.Bytes())
	buf.WriteString(openTag)
	buf.WriteRaw(bytesAsString(encoded))
	buf.WriteString(`</script>`)
}

// renderDataJSON runs the full load chain for route and returns a JSON
// response carrying the clientPayload. It is called when the request
// path ends in /__data.json (#38). Hooks (Handle, HandleError) already
// wrap this call via the normal resolve path.
func (s *Server) renderDataJSON(r *http.Request, ev *kit.RequestEvent, route *router.Route, form *formData) (*kit.Response, error) {
	if r.Method != http.MethodGet {
		return &kit.Response{
			Status:  http.StatusMethodNotAllowed,
			Headers: http.Header{"Allow": []string{http.MethodGet}},
			Body:    []byte("method not allowed"),
		}, nil
	}
	var (
		data        any
		layoutDatas []any
		lctx        *kit.LoadCtx
	)
	if route.Load != nil || hasAnyLayoutLoader(route.LayoutLoaders) {
		lctx = kit.NewLoadCtx(r, ev.Params)
		lctx.Locals = ev.Locals
		lctx.Cookies = ev.Cookies
		lctx.RawParams = ev.RawParams
		layoutDatas = make([]any, len(route.LayoutLoaders))
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
	if form != nil {
		data = injectFormField(data, form.data)
	}
	payload := buildClientPayload(r, ev, route, data, layoutDatas, ev.Params, form)
	if lctx != nil {
		payload.Deps = lctx.CollectDeps()
	}
	scratch := acquirePayloadBuf()
	if err := s.writePayloadJSON(scratch, payload); err != nil {
		releasePayloadBuf(scratch)
		return nil, fmt.Errorf("server: marshal __data.json: %w", err)
	}
	body := make([]byte, scratch.Len())
	copy(body, scratch.Bytes())
	releasePayloadBuf(scratch)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Content-Length", strconv.Itoa(len(body)))
	headers.Set("X-Sveltego-Data", "1")
	return &kit.Response{
		Status:  http.StatusOK,
		Headers: headers,
		Body:    body,
	}, nil
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

// renderSvelteShell handles RFC #379 phase 3 Svelte-mode routes. It
// runs the load chain, packs the result into the JSON hydration
// payload, and returns the app.html shell with the payload script and
// Vite asset tags injected — no Go-side page render. Vite's client
// bundle mounts the .svelte component and renders against the payload.
//
// Layout chain rendering is intentionally skipped: the client owns
// layouts in Svelte mode. Layout LOADS still run so client-side props
// have parent data via the existing payload.LayoutData channel.
func (s *Server) renderSvelteShell(r *http.Request, ev *kit.RequestEvent, route *router.Route, form *formData) (*kit.Response, error) {
	var (
		data        any
		layoutDatas []any
		lctx        *kit.LoadCtx
	)
	if route.Load != nil || hasAnyLayoutLoader(route.LayoutLoaders) {
		lctx = kit.NewLoadCtx(r, ev.Params)
		lctx.Locals = ev.Locals
		lctx.Cookies = ev.Cookies
		lctx.RawParams = ev.RawParams
		layoutDatas = make([]any, len(route.LayoutLoaders))
		for i, layoutLoad := range route.LayoutLoaders {
			if layoutLoad == nil {
				continue
			}
			d, err := layoutLoad(lctx)
			if err != nil {
				for k, vs := range lctx.CollectHeaders() {
					ev.ResponseHeader()[k] = vs
				}
				return nil, err
			}
			layoutDatas[i] = d
			lctx.PushParent(d)
		}
		if route.Load != nil {
			d, err := route.Load(lctx)
			if err != nil {
				for k, vs := range lctx.CollectHeaders() {
					ev.ResponseHeader()[k] = vs
				}
				return nil, err
			}
			data = d
		}
	}
	if form != nil {
		data = injectFormField(data, form.data)
	}

	buf := render.Acquire()
	defer render.Release(buf)

	nonce := kit.Nonce(ev)
	buf.WriteString(s.shellHead)
	if route.ClientKey != "" {
		if tags := s.viteManifest.headAssetTags(route.ClientKey, s.viteBase, nonce); tags != "" {
			buf.WriteString(tags)
		}
	}
	buf.WriteString(s.shellMid)
	buf.WriteString(`<div id="app"></div>`)
	payload := buildClientPayload(r, ev, route, data, layoutDatas, ev.Params, form)
	s.applyInitialPayloadFields(&payload)
	if lctx != nil {
		payload.Deps = lctx.CollectDeps()
	}
	s.emitPayloadScriptTag(buf, payload, nonce)
	if route.ClientKey != "" {
		if tag := s.viteManifest.bodyEntryTag(route.ClientKey, s.viteBase, nonce); tag != "" {
			buf.WriteString(tag)
		}
	}
	if sw := s.serviceWorkerTag(nonce); sw != "" {
		buf.WriteString(sw)
	}
	buf.WriteString(s.shellTail)

	body := make([]byte, len(buf.Bytes()))
	copy(body, buf.Bytes())
	headers := http.Header{}
	if lctx != nil {
		for k, vs := range lctx.CollectHeaders() {
			headers[k] = vs
		}
	}
	headers.Set("Content-Type", "text/html; charset=utf-8")
	headers.Set("Content-Length", strconv.Itoa(len(body)))
	status := http.StatusOK
	if form != nil && form.code != 0 {
		status = form.code
	}
	return &kit.Response{
		Status:  status,
		Headers: headers,
		Body:    body,
	}, nil
}

// renderPage runs the load chain and renders the page. When the load
// chain produced no kit.Streamed values it returns a buffered Response
// carrying the rendered HTML, status, and headers; when streams are
// present it switches to the chunked streaming path which writes
// directly to w and returns errStreamingWrote so writeResponse is
// skipped. When form is non-nil the page's PageData.Form field is set
// from form.data and the response status follows form.code.
//
// The buffered-response path stores its pooled render.Writer in *pageBuf
// (when non-nil) and aliases buf.Bytes() into Response.Body. The caller
// owns the writer and must release it after writeResponse runs.
func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, ev *kit.RequestEvent, route *router.Route, form *formData, pageBuf **render.Writer) (*kit.Response, error) {
	var (
		data        any
		layoutDatas []any
		lctx        *kit.LoadCtx
	)
	if route.Load != nil || hasAnyLayoutLoader(route.LayoutLoaders) {
		lctx = kit.NewLoadCtx(r, ev.Params)
		// Share the event's Locals map so every layout and page Load in the
		// chain reads Handle-populated values (session, user, nonce, …)
		// without requiring a Parent() call first.
		lctx.Locals = ev.Locals
		lctx.Cookies = ev.Cookies
		lctx.RawParams = ev.RawParams
		layoutDatas = make([]any, len(route.LayoutLoaders))
		for i, layoutLoad := range route.LayoutLoaders {
			if layoutLoad == nil {
				continue
			}
			d, err := layoutLoad(lctx)
			if err != nil {
				// Flush any headers set during Load into the event so error
				// paths in handlePipelineError can apply them via ev.ResponseHeader().
				for k, vs := range lctx.CollectHeaders() {
					ev.ResponseHeader()[k] = vs
				}
				return nil, err
			}
			layoutDatas[i] = d
			lctx.PushParent(d)
		}
		if route.Load != nil {
			d, err := route.Load(lctx)
			if err != nil {
				// Flush any headers set during Load into the event so error
				// paths in handlePipelineError can apply them via ev.ResponseHeader().
				for k, vs := range lctx.CollectHeaders() {
					ev.ResponseHeader()[k] = vs
				}
				return nil, err
			}
			data = d
		}
	}

	if form != nil {
		data = injectFormField(data, form.data)
	}

	rctx := &kit.RenderCtx{
		Locals:      ev.Locals,
		URL:         ev.URL,
		OriginalURL: ev.OriginalURL,
		Params:      ev.Params,
		RawParams:   ev.RawParams,
		Cookies:     ev.Cookies,
		Request:     r,
	}
	// inner composes the route's render closure. RenderChain (when non-nil)
	// embeds the layout chain composition — generated at codegen time, no
	// per-request closure stack. Routes without layouts call Page directly.
	var inner func(buf *render.Writer) error
	if route.RenderChain != nil {
		inner = func(buf *render.Writer) error {
			return route.RenderChain(buf, rctx, route.Page, data, layoutDatas)
		}
	} else {
		inner = func(buf *render.Writer) error {
			return route.Page(buf, rctx, data)
		}
	}

	nonce := kit.Nonce(ev)
	streams := collectStreams(data, layoutDatas)
	if len(streams) > 0 {
		status := http.StatusOK
		if form != nil && form.code != 0 {
			status = form.code
		}
		headBytes, err := gatherHead(rctx, route, data, layoutDatas)
		if err != nil {
			return nil, err
		}
		if lctx != nil {
			for k, vs := range lctx.CollectHeaders() {
				w.Header()[k] = vs
			}
		}
		payload := buildClientPayload(r, ev, route, data, layoutDatas, ev.Params, form)
		s.applyInitialPayloadFields(&payload)
		if lctx != nil {
			payload.Deps = lctx.CollectDeps()
		}
		var (
			headAssetTags string
			bodyEntryTag  string
		)
		if route.ClientKey != "" {
			headAssetTags = s.viteManifest.headAssetTags(route.ClientKey, s.viteBase, nonce)
			bodyEntryTag = s.viteManifest.bodyEntryTag(route.ClientKey, s.viteBase, nonce)
		}
		if err := s.renderStreaming(w, r, ev, inner, streams, status, headBytes, headAssetTags, bodyEntryTag, payload, nonce); err != nil {
			if errors.Is(err, kit.ErrClientGone) {
				return nil, errStreamingWrote
			}
			return nil, err
		}
		return nil, errStreamingWrote
	}

	headBytes, err := gatherHead(rctx, route, data, layoutDatas)
	if err != nil {
		return nil, err
	}

	buf := render.Acquire()
	released := false
	defer func() {
		if !released {
			render.Release(buf)
		}
	}()

	buf.WriteString(s.shellHead)
	if len(headBytes) > 0 {
		buf.WriteRaw(bytesAsString(headBytes))
	}
	if route != nil && route.ClientKey != "" {
		if tags := s.viteManifest.headAssetTags(route.ClientKey, s.viteBase, nonce); tags != "" {
			buf.WriteString(tags)
		}
	}
	buf.WriteString(s.shellMid)
	if err := inner(buf); err != nil {
		return nil, err
	}
	payload := buildClientPayload(r, ev, route, data, layoutDatas, ev.Params, form)
	s.applyInitialPayloadFields(&payload)
	if lctx != nil {
		payload.Deps = lctx.CollectDeps()
	}
	s.emitPayloadScriptTag(buf, payload, nonce)
	if route != nil && route.ClientKey != "" {
		if tag := s.viteManifest.bodyEntryTag(route.ClientKey, s.viteBase, nonce); tag != "" {
			buf.WriteString(tag)
		}
	}
	if sw := s.serviceWorkerTag(nonce); sw != "" {
		buf.WriteString(sw)
	}
	buf.WriteString(s.shellTail)

	body := buf.Bytes()
	headers := http.Header{}
	if lctx != nil {
		for k, vs := range lctx.CollectHeaders() {
			headers[k] = vs
		}
	}
	headers.Set("Content-Type", "text/html; charset=utf-8")
	headers.Set("Content-Length", strconv.Itoa(len(body)))
	status := http.StatusOK
	if form != nil && form.code != 0 {
		status = form.code
	}
	if pageBuf != nil {
		// Release any prior buffer the caller stashed (legitimate when
		// Handle invokes resolve more than once).
		if prev := *pageBuf; prev != nil {
			render.Release(prev)
		}
		*pageBuf = buf
		released = true
	} else {
		// Fallback: caller didn't supply ownership — copy out so the
		// buffer can be returned to the pool here.
		owned := make([]byte, len(body))
		copy(owned, body)
		body = owned
	}
	return &kit.Response{
		Status:  status,
		Headers: headers,
		Body:    body,
	}, nil
}

// streamedField pairs the runtime-erased Streamed wrapper with the
// stable ID emitted in the resolve script so the client patch lands on
// the right placeholder slot.
type streamedField struct {
	id     uint64
	stream kit.StreamedAny
}

// streamedAnyType is the reflect.Type for kit.StreamedAny, used once per
// concrete type during the initial field-index scan.
var streamedAnyType = reflect.TypeOf((*kit.StreamedAny)(nil)).Elem()

// streamFieldCache maps reflect.Type → []int (exported field indices whose
// static type implements kit.StreamedAny). An absent key means the type
// has never been seen; a nil slice means it was seen and has no streamed
// fields. sync.Map is read-mostly after process warm-up.
var streamFieldCache sync.Map // map[reflect.Type][]int

// streamFieldIndices returns cached field indices for struct type t. First
// call per concrete type walks the struct fields via reflection; every
// subsequent call is a lock-free sync.Map load with no type inspection.
func streamFieldIndices(t reflect.Type) []int {
	if v, ok := streamFieldCache.Load(t); ok {
		return v.([]int)
	}
	var indices []int
	for i := range t.NumField() {
		f := t.Field(i)
		if f.IsExported() && f.Type.Implements(streamedAnyType) {
			indices = append(indices, i)
		}
	}
	// LoadOrStore so a concurrent first-caller wins; we use whichever
	// result was stored first (always identical for a given type).
	v, _ := streamFieldCache.LoadOrStore(t, indices)
	return v.([]int)
}

// collectStreams walks data and every layoutData and returns each
// kit.Streamed value found in an exported struct field. Field-index
// discovery is cached per concrete type in streamFieldCache; hot-path
// requests pay only a sync.Map load and direct field accesses — zero
// repeated type walks. Returned streams preserve discovery order so
// resolve scripts emit deterministically.
func collectStreams(data any, layoutDatas []any) []streamedField {
	var out []streamedField
	out = appendStreams(out, data)
	for _, ld := range layoutDatas {
		out = appendStreams(out, ld)
	}
	return out
}

func appendStreams(dst []streamedField, v any) []streamedField {
	if v == nil {
		return dst
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return dst
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return dst
	}
	for _, i := range streamFieldIndices(rv.Type()) {
		fv := rv.Field(i).Interface()
		if s, ok := fv.(kit.StreamedAny); ok && s != nil {
			dst = append(dst, streamedField{id: s.StreamID(), stream: s})
		}
	}
	return dst
}

// isClientGone reports whether err signals that the client closed the
// connection. Covers broken pipe, use of closed network connection, and
// context cancellation caused by the request going away — all of which
// are normal events that should not pollute error logs.
func isClientGone(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx.Err() != nil {
		return true
	}
	if errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, net.ErrClosed) {
		return true
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}
	return false
}

// renderStreaming writes the chunked HTML response to w. It flushes the
// shell + page body first, then waits on each stream in registration
// order, emitting a __sveltego__resolve script per resolution. The
// shellTail closes the document only after every stream resolves or
// fails, so the response body is well-formed HTML even on timeout.
//
// headAssetTags carries the per-route stylesheet + modulepreload links
// (head-belonging tags); bodyEntryTag carries the entry <script
// type="module"> that lands at end of body just before the shell tail.
// Splitting them matches SvelteKit's %sveltekit.head% / %sveltekit.body%
// convention so the browser paints SSR HTML before any JS chunk
// executes while still discovering the chunks during HTML parse via the
// modulepreload hints.
//
// When a write fails because the client disconnected, renderStreaming
// cancels any pending streams, logs once at debug level, and returns
// kit.ErrClientGone. The caller must treat that as errStreamingWrote so
// HandleError is not invoked — a disconnect is not a server fault.
func (s *Server) renderStreaming(w http.ResponseWriter, r *http.Request, ev *kit.RequestEvent, inner func(*render.Writer) error, streams []streamedField, status int, headBytes []byte, headAssetTags, bodyEntryTag string, payload clientPayload, nonce string) error {
	if ev.Cookies != nil {
		ev.Cookies.Apply(w)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Del("Content-Length")
	w.WriteHeader(status)

	buf := render.Acquire()
	defer render.Release(buf)

	buf.WriteString(s.shellHead)
	if len(headBytes) > 0 {
		buf.WriteRaw(bytesAsString(headBytes))
	}
	if headAssetTags != "" {
		buf.WriteString(headAssetTags)
	}
	buf.WriteString(s.shellMid)
	if err := inner(buf); err != nil {
		return err
	}
	s.emitPayloadScriptTag(buf, payload, nonce)

	ctx := r.Context()

	if err := buf.FlushTo(w); err != nil {
		if isClientGone(ctx, err) {
			s.cancelStreams(streams)
			s.Logger.DebugContext(ctx, "streaming: client disconnected before shell flush")
			return kit.ErrClientGone
		}
		return err
	}

	for i, f := range streams {
		emitResolveScript(ctx, buf, f.id, f.stream, s.streamTimeout, nonce)
		// ctx.Err() is set when the client disconnected and WaitAny
		// returned early. The resolve script carries an error payload
		// that we don't need to flush — log once and bail.
		if ctx.Err() != nil {
			cancelStreams(streams[i+1:])
			s.Logger.DebugContext(ctx, "streaming: client disconnected mid-stream",
				logKeyStreamID, f.id)
			return kit.ErrClientGone
		}
		if err := buf.FlushTo(w); err != nil {
			if isClientGone(ctx, err) {
				cancelStreams(streams[i+1:])
				s.Logger.DebugContext(ctx, "streaming: client disconnected mid-stream (write)",
					logKeyStreamID, f.id)
				return kit.ErrClientGone
			}
			return err
		}
		if ctx.Err() != nil {
			cancelStreams(streams[i+1:])
			return nil
		}
	}

	if bodyEntryTag != "" {
		buf.WriteString(bodyEntryTag)
	}
	if sw := s.serviceWorkerTag(nonce); sw != "" {
		buf.WriteString(sw)
	}
	buf.WriteString(s.shellTail)
	if err := buf.FlushTo(w); err != nil {
		if isClientGone(ctx, err) {
			s.Logger.DebugContext(ctx, "streaming: client disconnected before shell tail")
			return kit.ErrClientGone
		}
		return err
	}
	return nil
}

// cancelStreams cancels any pending stream producers so goroutines don't
// outlive the request when the client disconnects mid-stream.
func (s *Server) cancelStreams(streams []streamedField) {
	for _, f := range streams {
		if c, ok := f.stream.(interface{ Cancel() }); ok {
			c.Cancel()
		}
	}
}

// cancelStreams calls Cancel on each stream in fs. Used when the request
// context dies before all streams resolve, so producer goroutines that
// weren't waited on receive a cancellation signal promptly.
func cancelStreams(fs []streamedField) {
	for _, f := range fs {
		if c, ok := f.stream.(interface{ Cancel() }); ok {
			c.Cancel()
		}
	}
}

// emitResolveScript writes a single <script>__sveltego__resolve(id, ...)</script>
// chunk for the stream. On success the JSON value is the resolved data;
// on timeout, cancellation, or producer error the call carries an error
// object the client can branch on. Errors are intentionally string-only
// to avoid leaking goroutine internals.
//
// nonce is the per-request CSP nonce; when non-empty the emitted
// <script> tag carries nonce="…" so streaming chunks land under the
// same `script-src 'nonce-…'` directive as the initial payload (#539).
func emitResolveScript(ctx context.Context, buf *render.Writer, id uint64, stream kit.StreamedAny, timeout time.Duration, nonce string) {
	v, err := stream.WaitAny(ctx, timeout)
	if nonce != "" {
		buf.WriteString(`<script nonce="`)
		buf.WriteString(nonce)
		buf.WriteString(`">__sveltego__resolve(`)
	} else {
		buf.WriteString(`<script>__sveltego__resolve(`)
	}
	buf.WriteString(strconv.FormatUint(id, 10))
	buf.WriteString(`,`)
	if err != nil {
		buf.WriteString(`{"error":`)
		writeJSONString(buf, err.Error())
		buf.WriteString(`}`)
	} else {
		if encoded, mErr := json.Marshal(v); mErr != nil {
			buf.WriteString(`{"error":`)
			writeJSONString(buf, mErr.Error())
			buf.WriteString(`}`)
		} else {
			buf.WriteString(`{"data":`)
			writeEscapedScriptJSON(buf, encoded)
			buf.WriteString(`}`)
		}
	}
	buf.WriteString(`)</script>`)
}

// writeJSONString emits s as a JSON string into buf. The fallback path
// uses json.Marshal so escape rules match the encoder used for the
// primary payload.
func writeJSONString(buf *render.Writer, s string) {
	encoded, err := json.Marshal(s)
	if err != nil {
		buf.WriteString(`""`)
		return
	}
	writeEscapedScriptJSON(buf, encoded)
}

// writeEscapedScriptJSON appends p to buf, neutralizing "</" and "<!--"
// sequences so an attacker-controlled string can't terminate the
// enclosing <script> tag or open an HTML comment. The fast path writes
// p verbatim when no escape-trigger byte appears; the slow path rewrites
// only the offending bytes via a single rebuilt slice. Other "<"
// characters pass through because they're already inside a JSON string
// literal that the browser's JS parser treats as ordinary text.
func writeEscapedScriptJSON(buf *render.Writer, p []byte) {
	i := indexScriptSpecial(p)
	if i < 0 {
		buf.WriteRaw(bytesAsString(p))
		return
	}
	buf.WriteRaw(bytesAsString(p[:i]))
	const escape = `<`
	tail := p[i:]
	start := 0
	for j := 0; j+1 < len(tail); j++ {
		if tail[j] == '<' && (tail[j+1] == '/' || tail[j+1] == '!') {
			if j > start {
				buf.WriteRaw(bytesAsString(tail[start:j]))
			}
			buf.WriteString(escape)
			start = j + 1
		}
	}
	if start < len(tail) {
		buf.WriteRaw(bytesAsString(tail[start:]))
	}
}

// indexScriptSpecial returns the index of the first byte that triggers
// script-context escaping, or -1 when p is clean.
func indexScriptSpecial(p []byte) int {
	for i := 0; i+1 < len(p); i++ {
		if p[i] == '<' && (p[i+1] == '/' || p[i+1] == '!') {
			return i
		}
	}
	return -1
}

// rawParamsFromPath extracts un-decoded route parameter values from path
// using the pattern's segment list and the already-decoded params map
// (used only to disambiguate optional segments). It splits path on
// literal '/' bytes without URL-decoding so callers receive the
// percent-encoded form the client sent (e.g. "hello%20world" rather than
// "hello world"). Static segments are consumed silently; rest segments
// join their remaining raw pieces with "/". Returns nil when there are
// no param segments.
func rawParamsFromPath(path string, segs []router.Segment, decoded map[string]string) map[string]string {
	// Strip leading slash so splitting yields uniform segments.
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	// Split on literal '/' — no decoding.
	var parts []string
	if path != "" {
		parts = strings.Split(path, "/")
	}

	// Count param-like segments to decide whether to allocate.
	paramCount := 0
	for _, s := range segs {
		if s.Kind != router.SegmentStatic {
			paramCount++
		}
	}
	if paramCount == 0 {
		return nil
	}

	out := make(map[string]string, paramCount)
	pi := 0 // index into parts
	for _, s := range segs {
		switch s.Kind {
		case router.SegmentStatic:
			pi++
		case router.SegmentParam:
			if pi < len(parts) {
				out[s.Name] = parts[pi]
				pi++
			}
		case router.SegmentOptional:
			// The decoded params map tells us whether the optional segment
			// matched a real value ("" means it was absent). Only consume
			// a raw part when the decoded value is non-empty.
			if decoded[s.Name] != "" && pi < len(parts) {
				out[s.Name] = parts[pi]
				pi++
			} else {
				out[s.Name] = ""
			}
		case router.SegmentRest:
			if pi < len(parts) {
				out[s.Name] = strings.Join(parts[pi:], "/")
			} else {
				out[s.Name] = ""
			}
			pi = len(parts)
		}
	}
	return out
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
