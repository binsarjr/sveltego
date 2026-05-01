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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
	"github.com/binsarjr/sveltego/packages/sveltego/server"
)

// shell is the minimal app.html used by every scenario. The server
// requires both placeholders; the body is intentionally tiny so the bench
// does not measure shell-template plumbing.
const shell = "<!doctype html><html><head>%sveltego.head%</head><body>%sveltego.body%</body></html>"

// Scenario is a self-contained benchmark target.
type Scenario struct {
	// Name is the human-readable label, used to derive benchmark names.
	Name string
	// Server is the sveltego server pre-built once per scenario.
	Server *server.Server
	// Request is a representative request for the scenario's hot route.
	Request *http.Request
}

// Run executes one ServeHTTP round-trip and returns the recorded response
// body length. The body is reset before serving so callers may reuse a
// recorder across iterations without unbounded growth; testing.B callers
// pass a fresh recorder per iteration so per-iter alloc counts are honest.
func (s Scenario) Run(rec *httptest.ResponseRecorder) int {
	rec.Body.Reset()
	s.Server.ServeHTTP(rec, s.Request)
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
	return []Scenario{hello, list, detail, action}, nil
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
		Server:  srv,
		Request: httptest.NewRequest(http.MethodGet, "/posts/42", nil),
	}, nil
}

// Action exercises the +server.go POST path at "/api/echo". The handler
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
		Server:  srv,
		Request: httptest.NewRequest(http.MethodPost, "/api/echo", nil),
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
