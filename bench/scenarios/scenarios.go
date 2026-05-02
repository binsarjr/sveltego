// Package scenarios builds reproducible SSR benchmark workloads against a
// configured sveltego server. Each scenario returns a [Scenario] with a
// pre-built [server.Server], a representative request, and a label used for
// benchmark naming.
//
// Scenarios mirror the layouts described in #60: hello (single static
// page), list (10-item index), detail (single record with deeper data),
// action (POST handler that returns a redirect). They are deliberately
// small and self-contained — no DB, no network, no JSON parsing — so the
// benchmark numbers reflect the framework's render + route + pipeline cost
// rather than user code.
package scenarios

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
	svelteServer "github.com/binsarjr/sveltego/packages/sveltego/runtime/svelte/server"
	"github.com/binsarjr/sveltego/packages/sveltego/server"
)

// Mode tags a scenario with the render mode under measurement so the CLI
// driver and CI gate can run a subset of scenarios without parsing names.
type Mode string

const (
	// ModeSSR is the request-time SSR pipeline: route match → Load →
	// Render → shell + payload.
	ModeSSR Mode = "ssr"
	// ModeSSG is the prerendered static-file path: build-time HTML
	// served by the static handler with no per-request work beyond OS
	// file read + headers.
	ModeSSG Mode = "ssg"
	// ModeSPA is the SSR=false shell-only path: app.html shell + empty
	// mount point, no Render emit, JSON payload computed from Load.
	ModeSPA Mode = "spa"
	// ModeStatic is the no-Load Svelte route: shell + empty payload,
	// the cheapest pure-Svelte pipeline state.
	ModeStatic Mode = "static"
)

// shell is the minimal app.html used by every scenario. The server
// requires both placeholders; the body is intentionally tiny so the bench
// does not measure shell-template plumbing.
const shell = "<!doctype html><html><head>%sveltego.head%</head><body>%sveltego.body%</body></html>"

// Scenario is a self-contained benchmark target.
type Scenario struct {
	// Name is the human-readable label, used to derive benchmark names.
	Name string
	// Mode tags the render mode this scenario measures.
	Mode Mode
	// Server is the sveltego server pre-built once per scenario. Nil
	// when the scenario serves through Handler directly (e.g. SSG via
	// the bare static handler).
	Server *server.Server
	// Handler, when non-nil, takes precedence over Server. Used by
	// scenarios that exercise a bare http.Handler (SSG static files)
	// to avoid spinning up the full server pipeline.
	Handler http.Handler
	// Request is a representative request for the scenario's hot route.
	Request *http.Request
}

// Run executes one ServeHTTP round-trip and returns the recorded response
// body length. The body is reset before serving so callers may reuse a
// recorder across iterations without unbounded growth; testing.B callers
// pass a fresh recorder per iteration so per-iter alloc counts are honest.
func (s Scenario) Run(rec *httptest.ResponseRecorder) int {
	rec.Body.Reset()
	if s.Handler != nil {
		s.Handler.ServeHTTP(rec, s.Request)
	} else {
		s.Server.ServeHTTP(rec, s.Request)
	}
	return rec.Body.Len()
}

// All returns the benchmark scenarios in deterministic order.
func All() ([]Scenario, error) {
	hello, err := Hello()
	if err != nil {
		return nil, fmt.Errorf("hello: %w", err)
	}
	list, err := List()
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	detail, err := Detail()
	if err != nil {
		return nil, fmt.Errorf("detail: %w", err)
	}
	action, err := Action()
	if err != nil {
		return nil, fmt.Errorf("action: %w", err)
	}
	spa, err := SvelteSPA()
	if err != nil {
		return nil, fmt.Errorf("svelte-spa: %w", err)
	}
	ssrHello, err := SSRHello()
	if err != nil {
		return nil, fmt.Errorf("ssr-hello: %w", err)
	}
	ssrTypical, err := SSRTypicalPage()
	if err != nil {
		return nil, fmt.Errorf("ssr-typical: %w", err)
	}
	ssrHeavy, err := SSRHeavyList()
	if err != nil {
		return nil, fmt.Errorf("ssr-heavy: %w", err)
	}
	ssg, err := SSGServe()
	if err != nil {
		return nil, fmt.Errorf("ssg-serve: %w", err)
	}
	spaShell, err := SPAShell()
	if err != nil {
		return nil, fmt.Errorf("spa-shell: %w", err)
	}
	staticNoLoad, err := StaticNoLoad()
	if err != nil {
		return nil, fmt.Errorf("static-no-load: %w", err)
	}
	return []Scenario{
		hello, list, detail, action, spa,
		ssrHello, ssrTypical, ssrHeavy,
		ssg, spaShell, staticNoLoad,
	}, nil
}

// Hello renders a static greeting at "/".
func Hello() (Scenario, error) {
	routes := []router.Route{{
		Pattern:  "/",
		Segments: []router.Segment{},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("<h1>hello world</h1>")
			return nil
		},
	}}
	srv, err := newServer(routes)
	if err != nil {
		return Scenario{}, err
	}
	return Scenario{
		Name:    "hello",
		Mode:    ModeSSR,
		Server:  srv,
		Request: httptest.NewRequest(http.MethodGet, "/", nil),
	}, nil
}

// List renders a 10-row index at "/posts".
func List() (Scenario, error) {
	routes := []router.Route{{
		Pattern: "/posts",
		Segments: []router.Segment{
			{Kind: router.SegmentStatic, Value: "posts"},
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("<ul>")
			for i := 0; i < 10; i++ {
				w.WriteString(`<li><a href="/posts/`)
				w.WriteString(strconv.Itoa(i))
				w.WriteString(`">post `)
				w.WriteEscape(i)
				w.WriteString("</a></li>")
			}
			w.WriteString("</ul>")
			return nil
		},
	}}
	srv, err := newServer(routes)
	if err != nil {
		return Scenario{}, err
	}
	return Scenario{
		Name:    "list",
		Mode:    ModeSSR,
		Server:  srv,
		Request: httptest.NewRequest(http.MethodGet, "/posts", nil),
	}, nil
}

// Detail renders a parameterized record at "/posts/[id]".
func Detail() (Scenario, error) {
	routes := []router.Route{{
		Pattern: "/posts/[id]",
		Segments: []router.Segment{
			{Kind: router.SegmentStatic, Value: "posts"},
			{Kind: router.SegmentParam, Name: "id"},
		},
		Page: func(w *render.Writer, ctx *kit.RenderCtx, _ any) error {
			w.WriteString("<article><h1>post ")
			w.WriteEscape(ctx.Params["id"])
			w.WriteString(`</h1><p>Lorem ipsum dolor sit amet, consectetur adipiscing elit.</p></article>`)
			return nil
		},
	}}
	srv, err := newServer(routes)
	if err != nil {
		return Scenario{}, err
	}
	return Scenario{
		Name:    "detail",
		Mode:    ModeSSR,
		Server:  srv,
		Request: httptest.NewRequest(http.MethodGet, "/posts/42", nil),
	}, nil
}

// Action exercises the _server.go POST path at "/api/echo". The handler
// writes a fixed payload — enough to round-trip the server pipeline,
// without measuring user-side encoding cost.
func Action() (Scenario, error) {
	routes := []router.Route{{
		Pattern: "/api/echo",
		Segments: []router.Segment{
			{Kind: router.SegmentStatic, Value: "api"},
			{Kind: router.SegmentStatic, Value: "echo"},
		},
		Server: router.ServerHandlers{
			http.MethodPost: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true}`))
			},
		},
	}}
	srv, err := newServer(routes)
	if err != nil {
		return Scenario{}, err
	}
	return Scenario{
		Name:    "action",
		Mode:    ModeSSR,
		Server:  srv,
		Request: httptest.NewRequest(http.MethodPost, "/api/echo", nil),
	}, nil
}

// SvelteSPA exercises the pure-Svelte hot path: the route declares no Go
// Page handler, sets `Templates: "svelte"`, and returns the app shell
// with a JSON hydration payload computed from a small Load. This is the
// runtime-cost scenario for SPA-mode routes after the RFC #379 pivot.
func SvelteSPA() (Scenario, error) {
	type spaData struct {
		Title string `json:"title"`
		Items []int  `json:"items"`
	}
	routes := []router.Route{{
		Pattern: "/spa",
		Segments: []router.Segment{
			{Kind: router.SegmentStatic, Value: "spa"},
		},
		Load: func(_ *kit.LoadCtx) (any, error) {
			return spaData{
				Title: "spa hot path",
				Items: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			}, nil
		},
		Options: kit.PageOptions{
			SSR:       true,
			CSR:       true,
			CSRF:      true,
			Templates: kit.TemplatesSvelte,
		},
	}}
	srv, err := newServer(routes)
	if err != nil {
		return Scenario{}, err
	}
	return Scenario{
		Name:    "svelte-spa",
		Mode:    ModeSSR,
		Server:  srv,
		Request: httptest.NewRequest(http.MethodGet, "/spa", nil),
	}, nil
}

func newServer(routes []router.Route) (*server.Server, error) {
	srv, err := server.New(server.Config{
		Routes: routes,
		Shell:  shell,
		Logger: quietLogger(),
	})
	if err != nil {
		return nil, fmt.Errorf("server.New: %w", err)
	}
	return srv, nil
}

// ssrHelloData mirrors a real PageData a Phase 6 SSR Render would
// receive: a typed Go struct populated by Load and threaded through
// route.Page. Bench scenarios that simulate emitted Render() output go
// through these typed values rather than a generic any so the cost
// profile reflects the production path.
type ssrHelloData struct {
	Greeting string
}

type ssrTypicalData struct {
	Title    string
	LoggedIn bool
	Username string
	Items    []string
}

type ssrHeavyData struct {
	Title string
	Items []string
}

// SSRHello simulates the simplest emitted Render — a single Push +
// EscapeHTML pair, mirroring what svelte_js2go emits for a
// `<h1>{data.greeting}</h1>` template. Drives the ≥10k rps p50 target
// from issue #429.
func SSRHello() (Scenario, error) {
	data := ssrHelloData{Greeting: "hello world"}
	routes := []router.Route{{
		Pattern:  "/",
		Segments: []router.Segment{},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			var p svelteServer.Payload
			p.Push("<h1>")
			p.Push(svelteServer.EscapeHTML(data.Greeting))
			p.Push("</h1>")
			w.WriteString(p.Body())
			return nil
		},
	}}
	srv, err := newServer(routes)
	if err != nil {
		return Scenario{}, err
	}
	return Scenario{
		Name:    "ssr-hello",
		Mode:    ModeSSR,
		Server:  srv,
		Request: httptest.NewRequest(http.MethodGet, "/", nil),
	}, nil
}

// SSRTypicalPage simulates a mid-complexity page: header + conditional
// + 10-item list + footer. Reflects the average production page
// rendered through emitted Render.
func SSRTypicalPage() (Scenario, error) {
	data := ssrTypicalData{
		Title:    "dashboard",
		LoggedIn: true,
		Username: "alice",
		Items:    []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
	}
	routes := []router.Route{{
		Pattern: "/page",
		Segments: []router.Segment{
			{Kind: router.SegmentStatic, Value: "page"},
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			var p svelteServer.Payload
			p.Push("<header><h1>")
			p.Push(svelteServer.EscapeHTML(data.Title))
			p.Push("</h1></header>")
			if data.LoggedIn {
				p.Push("<p>Welcome back, ")
				p.Push(svelteServer.EscapeHTML(data.Username))
				p.Push(".</p>")
			} else {
				p.Push("<p>Please log in.</p>")
			}
			p.Push("<ul>")
			for _, item := range data.Items {
				p.Push("<li>")
				p.Push(svelteServer.EscapeHTML(item))
				p.Push("</li>")
			}
			p.Push("</ul><footer>fin</footer>")
			w.WriteString(p.Body())
			return nil
		},
	}}
	srv, err := newServer(routes)
	if err != nil {
		return Scenario{}, err
	}
	return Scenario{
		Name:    "ssr-typical",
		Mode:    ModeSSR,
		Server:  srv,
		Request: httptest.NewRequest(http.MethodGet, "/page", nil),
	}, nil
}

// SSRHeavyList simulates a 100-item each-loop page. Stress-tests the
// hot loop and per-iteration EscapeHTML cost.
func SSRHeavyList() (Scenario, error) {
	items := make([]string, 100)
	for i := range items {
		items[i] = "row " + strconv.Itoa(i)
	}
	data := ssrHeavyData{Title: "heavy", Items: items}
	routes := []router.Route{{
		Pattern: "/heavy",
		Segments: []router.Segment{
			{Kind: router.SegmentStatic, Value: "heavy"},
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			var p svelteServer.Payload
			p.Push("<h1>")
			p.Push(svelteServer.EscapeHTML(data.Title))
			p.Push("</h1><ul>")
			for _, item := range data.Items {
				p.Push("<li>")
				p.Push(svelteServer.EscapeHTML(item))
				p.Push("</li>")
			}
			p.Push("</ul>")
			w.WriteString(p.Body())
			return nil
		},
	}}
	srv, err := newServer(routes)
	if err != nil {
		return Scenario{}, err
	}
	return Scenario{
		Name:    "ssr-heavy",
		Mode:    ModeSSR,
		Server:  srv,
		Request: httptest.NewRequest(http.MethodGet, "/heavy", nil),
	}, nil
}

// SSGServe measures the SSG hot path as production serves it: a request
// against `Server.ServeHTTP` short-circuits to `servePrerendered`, which
// looks up the URL in the prerender manifest and writes the on-disk file
// to the response. No Load, no Render, no shell merge per request — just
// a map lookup, an OS file read, and the response headers the runtime
// emits (`Content-Type`, `Content-Length`, `X-Sveltego-Prerendered`).
//
// The bench drives the full server (not the bare static handler) because
// `servePrerendered` is the runtime path real users hit; static handler
// is a different code path with different overhead. The v1.0 budget for
// prerendered routes (#421) is measured against the path the server
// actually serves.
//
// Setup runs `Server.Prerender` once against a Page that mirrors the
// previous synthetic body so baseline-ssg.txt remains roughly comparable
// (shell boilerplate + small body). The produced manifest is then loaded
// and wired into a fresh server via `Config.Prerender`; the hot loop
// hits that server.
func SSGServe() (Scenario, error) {
	root, err := os.MkdirTemp("", "sveltego-bench-ssg-*")
	if err != nil {
		return Scenario{}, fmt.Errorf("mkdir temp: %w", err)
	}
	routes := []router.Route{{
		Pattern:  "/",
		Segments: []router.Segment{},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("<h1>hello world</h1><p>prerendered at build time.</p>")
			return nil
		},
		Options: kit.PageOptions{Prerender: true, SSR: true, CSR: true, CSRF: true},
	}}
	prerSrv, err := newServer(routes)
	if err != nil {
		return Scenario{}, err
	}
	if _, err := prerSrv.Prerender(context.Background(), root, server.PrerenderOptions{}); err != nil {
		return Scenario{}, fmt.Errorf("prerender: %w", err)
	}

	table, err := server.LoadPrerenderManifest(root, "")
	if err != nil {
		return Scenario{}, fmt.Errorf("load prerender manifest: %w", err)
	}
	if table == nil {
		return Scenario{}, fmt.Errorf("prerender manifest missing under %s", root)
	}
	srv, err := server.New(server.Config{
		Routes:    routes,
		Shell:     shell,
		Logger:    quietLogger(),
		Prerender: table,
	})
	if err != nil {
		return Scenario{}, fmt.Errorf("server.New: %w", err)
	}
	return Scenario{
		Name:    "ssg-serve",
		Mode:    ModeSSG,
		Server:  srv,
		Request: httptest.NewRequest(http.MethodGet, "/", nil),
	}, nil
}

// SPAShell measures the SSR=false shell-only path: the route declares
// `Options.SSR = false`, so the server short-circuits to renderEmptyShell
// and returns app.html with an empty mount point plus the JSON
// hydration payload computed from Load. No template render, no payload
// inlined into the body. Cheaper than full SSR; quantifies the SSR vs
// SPA tradeoff for v1.0 perf signoff (#421).
func SPAShell() (Scenario, error) {
	type spaShellData struct {
		Title string `json:"title"`
		Items []int  `json:"items"`
	}
	routes := []router.Route{{
		Pattern: "/spa-shell",
		Segments: []router.Segment{
			{Kind: router.SegmentStatic, Value: "spa-shell"},
		},
		Load: func(_ *kit.LoadCtx) (any, error) {
			return spaShellData{
				Title: "spa shell",
				Items: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			}, nil
		},
		Options: kit.PageOptions{
			SSR:       false,
			CSR:       true,
			CSRF:      true,
			Templates: kit.TemplatesSvelte,
		},
	}}
	srv, err := newServer(routes)
	if err != nil {
		return Scenario{}, err
	}
	return Scenario{
		Name:    "spa-shell",
		Mode:    ModeSPA,
		Server:  srv,
		Request: httptest.NewRequest(http.MethodGet, "/spa-shell", nil),
	}, nil
}

// StaticNoLoad measures the cheapest pure-Svelte pipeline state: a
// route with Templates="svelte", no Load handler, no _page.server.go.
// The pipeline returns the Svelte shell with an empty data payload.
// Distinguishes "no work to do" cost from full SSR cost so static-only
// pages have their own baseline.
func StaticNoLoad() (Scenario, error) {
	routes := []router.Route{{
		Pattern: "/static",
		Segments: []router.Segment{
			{Kind: router.SegmentStatic, Value: "static"},
		},
		Options: kit.PageOptions{
			SSR:       true,
			CSR:       true,
			CSRF:      true,
			Templates: kit.TemplatesSvelte,
		},
	}}
	srv, err := newServer(routes)
	if err != nil {
		return Scenario{}, err
	}
	return Scenario{
		Name:    "static-no-load",
		Mode:    ModeStatic,
		Server:  srv,
		Request: httptest.NewRequest(http.MethodGet, "/static", nil),
	}, nil
}
