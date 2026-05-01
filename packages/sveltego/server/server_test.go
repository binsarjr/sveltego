package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit/params"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

const testShell = "<!doctype html><html><head>%sveltego.head%</head><body>%sveltego.body%</body></html>"

// quietLogger silences slog output during tests; the discard handler
// drops every record without formatting cost.
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

func staticPage(body string) router.PageHandler {
	return func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
		w.WriteString(body)
		return nil
	}
}

func paramPage() router.PageHandler {
	return func(w *render.Writer, ctx *kit.RenderCtx, _ any) error {
		w.WriteString("<h1>id=")
		w.WriteString(ctx.Params["id"])
		w.WriteString("</h1>")
		return nil
	}
}

func loadingPage() router.PageHandler {
	return func(w *render.Writer, _ *kit.RenderCtx, data any) error {
		s, _ := data.(string)
		w.WriteString("<h1>")
		w.WriteString(s)
		w.WriteString("</h1>")
		return nil
	}
}

func segmentsFor(pattern string) []router.Segment {
	switch pattern {
	case "/":
		return []router.Segment{}
	case "/about":
		return []router.Segment{{Kind: router.SegmentStatic, Value: "about"}}
	case "/post/[id=int]":
		return []router.Segment{
			{Kind: router.SegmentStatic, Value: "post"},
			{Kind: router.SegmentParam, Name: "id", Matcher: "int"},
		}
	case "/api/data":
		return []router.Segment{
			{Kind: router.SegmentStatic, Value: "api"},
			{Kind: router.SegmentStatic, Value: "data"},
		}
	case "/[[lang]]/about":
		return []router.Segment{
			{Kind: router.SegmentOptional, Name: "lang"},
			{Kind: router.SegmentStatic, Value: "about"},
		}
	case "/docs/[...path]":
		return []router.Segment{
			{Kind: router.SegmentStatic, Value: "docs"},
			{Kind: router.SegmentRest, Name: "path"},
		}
	case "/load":
		return []router.Segment{{Kind: router.SegmentStatic, Value: "load"}}
	case "/render-fail":
		return []router.Segment{{Kind: router.SegmentStatic, Value: "render-fail"}}
	case "/orphan-server":
		return []router.Segment{{Kind: router.SegmentStatic, Value: "orphan-server"}}
	case "/lang/[name]":
		return []router.Segment{
			{Kind: router.SegmentStatic, Value: "lang"},
			{Kind: router.SegmentParam, Name: "name"},
		}
	case "/protected":
		return []router.Segment{{Kind: router.SegmentStatic, Value: "protected"}}
	case "/public":
		return []router.Segment{{Kind: router.SegmentStatic, Value: "public"}}
	case "/dashboard":
		return []router.Segment{{Kind: router.SegmentStatic, Value: "dashboard"}}
	}
	panic("unknown pattern: " + pattern)
}

func newTestServer(t *testing.T, routes []router.Route) *Server {
	t.Helper()
	srv, err := New(Config{
		Routes:   routes,
		Matchers: params.DefaultMatchers(),
		Shell:    testShell,
		Logger:   quietLogger(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv
}

func TestNew_validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "no_routes",
			cfg:  Config{Shell: testShell},
			want: "Routes is empty",
		},
		{
			name: "no_shell",
			cfg:  Config{Routes: []router.Route{{Pattern: "/", Page: staticPage("x")}}},
			want: "Shell is empty",
		},
		{
			name: "bad_shell",
			cfg:  Config{Routes: []router.Route{{Pattern: "/", Page: staticPage("x")}}, Shell: "no placeholders"},
			want: "shell missing",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestServeHTTP_indexPage(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page:     staticPage("<h1>home</h1>"),
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content-type: %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<h1>home</h1>") {
		t.Fatalf("body missing page content: %q", body)
	}
	if !strings.HasPrefix(string(body), "<!doctype html>") {
		t.Fatalf("body missing shell prefix: %q", body)
	}
	wantLen := strconv.Itoa(len(body))
	if got := resp.Header.Get("Content-Length"); got != wantLen {
		t.Fatalf("content-length: got %s want %s", got, wantLen)
	}
}

func TestServeHTTP_notFound(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page:     staticPage("home"),
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/nope")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d want 404", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content-type: %q", ct)
	}
}

func TestServeHTTP_serverRoute(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/api/data",
		Segments: segmentsFor("/api/data"),
		Server: router.ServerHandlers{
			http.MethodGet: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"ok":true}`))
			},
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	t.Run("get_ok", func(t *testing.T) {
		t.Parallel()
		resp, err := http.Get(ts.URL + "/api/data")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status: %d", resp.StatusCode)
		}
	})

	t.Run("post_405", func(t *testing.T) {
		t.Parallel()
		resp, err := http.Post(ts.URL+"/api/data", "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("status: got %d want 405", resp.StatusCode)
		}
		if got := resp.Header.Get("Allow"); got != http.MethodGet {
			t.Fatalf("allow: got %q want %q", got, http.MethodGet)
		}
	})
}

func TestServeHTTP_paramMatcher(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/post/[id=int]",
		Segments: segmentsFor("/post/[id=int]"),
		Page:     paramPage(),
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/post/42")
	if err != nil {
		t.Fatalf("GET 42: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "id=42") {
		t.Fatalf("got %d body=%q", resp.StatusCode, body)
	}

	resp, err = http.Get(ts.URL + "/post/abc")
	if err != nil {
		t.Fatalf("GET abc: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("non-int param: got %d want 404", resp.StatusCode)
	}
}

func TestServeHTTP_optionalSegment(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/[[lang]]/about",
		Segments: segmentsFor("/[[lang]]/about"),
		Page: func(w *render.Writer, ctx *kit.RenderCtx, _ any) error {
			w.WriteString("lang=")
			w.WriteString(ctx.Params["lang"])
			return nil
		},
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	for _, tc := range []struct {
		path string
		lang string
	}{
		{"/about", ""},
		{"/en/about", "en"},
	} {
		resp, err := http.Get(ts.URL + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		want := "lang=" + tc.lang
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), want) {
			t.Fatalf("path=%s status=%d body=%q want %q", tc.path, resp.StatusCode, body, want)
		}
	}
}

func TestServeHTTP_restSegment(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/docs/[...path]",
		Segments: segmentsFor("/docs/[...path]"),
		Page: func(w *render.Writer, ctx *kit.RenderCtx, _ any) error {
			w.WriteString("path=")
			w.WriteString(ctx.Params["path"])
			return nil
		},
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/docs/a/b")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "path=a/b") {
		t.Fatalf("got %d body=%q", resp.StatusCode, body)
	}
}

type loadFailErr struct{ msg string }

func (e *loadFailErr) Error() string { return e.msg }

type withStatus struct{ code int }

func (e *withStatus) Error() string   { return "boom" }
func (e *withStatus) HTTPStatus() int { return e.code }

func TestServeHTTP_loadError(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, []router.Route{
		{
			Pattern:  "/load",
			Segments: segmentsFor("/load"),
			Page:     loadingPage(),
			Load: func(_ *kit.LoadCtx) (any, error) {
				return nil, &loadFailErr{msg: "io broken"}
			},
		},
		{
			Pattern:  "/lang/[name]",
			Segments: segmentsFor("/lang/[name]"),
			Page:     loadingPage(),
			Load: func(_ *kit.LoadCtx) (any, error) {
				return nil, &withStatus{code: http.StatusTeapot}
			},
		},
	})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/load")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("plain error: got %d want 500", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/lang/en")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("status err: got %d want 418", resp.StatusCode)
	}
}

func TestServeHTTP_loadDataPropagates(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/load",
		Segments: segmentsFor("/load"),
		Page:     loadingPage(),
		Load:     func(_ *kit.LoadCtx) (any, error) { return "loaded", nil },
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/load")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "<h1>loaded</h1>") {
		t.Fatalf("body missing data: %q", body)
	}
}

func TestServeHTTP_renderError(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/render-fail",
		Segments: segmentsFor("/render-fail"),
		Page: func(_ *render.Writer, _ *kit.RenderCtx, _ any) error {
			return errors.New("template explosion")
		},
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/render-fail")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("got %d want 500", resp.StatusCode)
	}
}

func TestServeHTTP_orphanRoute(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{
		{Pattern: "/orphan-server", Segments: segmentsFor("/orphan-server")},
		{Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("home")},
	})
	ts := httptest.NewServer(srv)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/orphan-server")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("orphan route: got %d want 404", resp.StatusCode)
	}
}

func TestServeHTTP_layoutChainComposition(t *testing.T) {
	t.Parallel()
	page := func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
		w.WriteString("<page/>")
		return nil
	}
	makeLayout := func(tag string) router.LayoutHandler {
		return func(w *render.Writer, _ *kit.RenderCtx, _ any, children func(*render.Writer) error) error {
			w.WriteString("<" + tag + ">")
			if err := children(w); err != nil {
				return err
			}
			w.WriteString("</" + tag + ">")
			return nil
		}
	}
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page:     page,
		LayoutChain: []router.LayoutHandler{
			makeLayout("root"),
			makeLayout("app"),
			makeLayout("section"),
		},
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	want := "<root><app><section><page/></section></app></root>"
	if !strings.Contains(string(body), want) {
		t.Fatalf("expected layout composition %q in body:\n%s", want, body)
	}
}

// TestServeHTTP_layoutLoadParentChain covers Phase 0k-B: layout loaders
// run outer→inner, each pushing onto the LoadCtx parent stack so the
// next layer (and the page Load) read their direct parent through
// ctx.Parent. The render side receives each layout's own data, not the
// page's.
func TestServeHTTP_layoutLoadParentChain(t *testing.T) {
	t.Parallel()
	type rootData struct{ User string }
	type sectionData struct {
		User string
		Org  string
	}

	rootLoad := func(_ *kit.LoadCtx) (any, error) {
		return rootData{User: "alice"}, nil
	}
	sectionLoad := func(ctx *kit.LoadCtx) (any, error) {
		parent, ok := ctx.Parent().(rootData)
		if !ok {
			return nil, errors.New("section: missing root parent")
		}
		return sectionData{User: parent.User, Org: "acme"}, nil
	}
	pageLoad := func(ctx *kit.LoadCtx) (any, error) {
		parent, ok := ctx.Parent().(sectionData)
		if !ok {
			return nil, errors.New("page: missing section parent")
		}
		return parent.User + "@" + parent.Org, nil
	}

	rootLayout := func(w *render.Writer, _ *kit.RenderCtx, data any, children func(*render.Writer) error) error {
		d, _ := data.(rootData)
		w.WriteString("<root user=" + d.User + ">")
		if err := children(w); err != nil {
			return err
		}
		w.WriteString("</root>")
		return nil
	}
	sectionLayout := func(w *render.Writer, _ *kit.RenderCtx, data any, children func(*render.Writer) error) error {
		d, _ := data.(sectionData)
		w.WriteString("<section org=" + d.Org + ">")
		if err := children(w); err != nil {
			return err
		}
		w.WriteString("</section>")
		return nil
	}
	page := func(w *render.Writer, _ *kit.RenderCtx, data any) error {
		s, _ := data.(string)
		w.WriteString("<p>" + s + "</p>")
		return nil
	}

	srv := newTestServer(t, []router.Route{{
		Pattern:     "/",
		Segments:    segmentsFor("/"),
		Page:        page,
		Load:        pageLoad,
		LayoutChain: []router.LayoutHandler{rootLayout, sectionLayout},
		LayoutLoaders: []router.LayoutLoadHandler{
			rootLoad,
			sectionLoad,
		},
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", resp.StatusCode, body)
	}
	want := "<root user=alice><section org=acme><p>alice@acme</p></section></root>"
	if !strings.Contains(string(body), want) {
		t.Fatalf("expected layered render %q in body:\n%s", want, body)
	}
}

// TestServeHTTP_layoutLoadErrorShortCircuits asserts that a layout
// loader's error propagates through handleLoadError exactly like a page
// Load error. The page Load and any inner layout Load must not run.
func TestServeHTTP_layoutLoadErrorShortCircuits(t *testing.T) {
	t.Parallel()
	pageRan := false
	page := func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
		pageRan = true
		w.WriteString("page")
		return nil
	}
	failing := func(_ *kit.LoadCtx) (any, error) {
		return nil, errors.New("layout load boom")
	}
	srv := newTestServer(t, []router.Route{{
		Pattern:     "/",
		Segments:    segmentsFor("/"),
		Page:        page,
		LayoutChain: []router.LayoutHandler{func(w *render.Writer, _ *kit.RenderCtx, _ any, c func(*render.Writer) error) error { return c(w) }},
		LayoutLoaders: []router.LayoutLoadHandler{
			failing,
		},
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("layout load error: got %d want 500", resp.StatusCode)
	}
	if pageRan {
		t.Fatal("page rendered despite layout load failure")
	}
}

func TestServeHTTP_layoutErrorAborts(t *testing.T) {
	t.Parallel()
	page := func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
		w.WriteString("page")
		return nil
	}
	failing := func(_ *render.Writer, _ *kit.RenderCtx, _ any, _ func(*render.Writer) error) error {
		return errors.New("layout boom")
	}
	srv := newTestServer(t, []router.Route{{
		Pattern:     "/",
		Segments:    segmentsFor("/"),
		Page:        page,
		LayoutChain: []router.LayoutHandler{failing},
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("layout error: got %d want 500", resp.StatusCode)
	}
}

func TestServeHTTP_concurrent(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/post/[id=int]",
		Segments: segmentsFor("/post/[id=int]"),
		Page:     paramPage(),
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	const goroutines = 100
	const iters = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	errCh := make(chan error, goroutines*iters)
	for g := range goroutines {
		go func(gid int) {
			defer wg.Done()
			client := &http.Client{Timeout: 5 * time.Second}
			for i := range iters {
				id := gid*iters + i
				url := fmt.Sprintf("%s/post/%d", ts.URL, id)
				resp, err := client.Get(url)
				if err != nil {
					errCh <- err
					return
				}
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					errCh <- fmt.Errorf("status=%d", resp.StatusCode)
					return
				}
				want := fmt.Sprintf("id=%d", id)
				if !strings.Contains(string(body), want) {
					errCh <- fmt.Errorf("missing %s", want)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent: %v", err)
	}
}

func TestServeHTTP_contentLengthMatchesBody(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page:     staticPage("<p>" + strings.Repeat("a", 1024) + "</p>"),
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	cl := resp.Header.Get("Content-Length")
	if cl != strconv.Itoa(len(body)) {
		t.Fatalf("content-length=%s body-len=%d", cl, len(body))
	}
}

func TestServer_renderPoolReuse(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page:     staticPage("<h1>pooltest</h1>"),
	}})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	srv.ServeHTTP(w, r)
	first := w.Body.String()

	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	second := w.Body.String()
	if first != second {
		t.Fatalf("pool reuse corruption: %q vs %q", first, second)
	}

	// After ServeHTTP the writer was Released back to the pool; a fresh
	// Acquire returns a Writer with Len() == 0 (pool guarantees Reset).
	buf := render.Acquire()
	defer render.Release(buf)
	if buf.Len() != 0 {
		t.Fatalf("acquired writer not reset: len=%d", buf.Len())
	}
}

func TestServer_gracefulShutdown(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page:     staticPage("<h1>home</h1>"),
	}})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe(addr) }()

	// Poll the listener until it is up to avoid a brittle sleep.
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := http.Get("http://" + addr + "/")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not start: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("ListenAndServe: %v", err)
	}

	// New connection after shutdown should fail.
	if resp, err := http.Get("http://" + addr + "/"); err == nil {
		resp.Body.Close()
		t.Fatalf("expected refused connection after shutdown")
	}
}

// TestInitPending verifies that a request arriving while Init is still
// running waits up to InitTimeout and then receives 503 with the pending
// fallback body.
func TestInitPending(t *testing.T) {
	t.Parallel()

	ready := make(chan struct{})
	hooks := kit.Hooks{
		Init: func(_ context.Context) error {
			<-ready // block until the test releases it
			return nil
		},
	}
	srv, err := New(Config{
		Routes:          []router.Route{{Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("home")}},
		Shell:           testShell,
		Logger:          quietLogger(),
		Hooks:           hooks,
		InitTimeout:     50 * time.Millisecond,
		InitPendingHTML: "<p>pending</p>",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	srv.RunInitAsync(context.Background())

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	t.Cleanup(func() { close(ready) })

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
	if !strings.Contains(string(body), "pending") {
		t.Errorf("body = %q, want pending HTML", body)
	}
}

// TestInitError verifies that after Init returns an error every subsequent
// request receives 500 with the error fallback body.
func TestInitError(t *testing.T) {
	t.Parallel()

	hooks := kit.Hooks{
		Init: func(_ context.Context) error { return errors.New("startup broken") },
	}
	srv, err := New(Config{
		Routes:        []router.Route{{Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("home")}},
		Shell:         testShell,
		Logger:        quietLogger(),
		Hooks:         hooks,
		InitErrorHTML: "<p>init error</p>",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if initErr := srv.Init(context.Background()); initErr == nil {
		t.Fatal("expected Init to return an error")
	}

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if !strings.Contains(string(body), "init error") {
		t.Errorf("body = %q, want init error HTML", body)
	}
}

// TestInitSuccess verifies that after Init completes without error the
// normal pipeline runs and requests receive 200 with the page body.
func TestInitSuccess(t *testing.T) {
	t.Parallel()

	hooks := kit.Hooks{
		Init: func(_ context.Context) error { return nil },
	}
	srv, err := New(Config{
		Routes: []router.Route{{Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("<h1>ok</h1>")}},
		Shell:  testShell,
		Logger: quietLogger(),
		Hooks:  hooks,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := srv.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), "<h1>ok</h1>") {
		t.Errorf("body = %q, want page content", body)
	}
}

func TestServer_methodsOf(t *testing.T) {
	t.Parallel()
	got := methodsOf(router.ServerHandlers{
		http.MethodPost: func(http.ResponseWriter, *http.Request) {},
		http.MethodGet:  func(http.ResponseWriter, *http.Request) {},
		"DELETE":        nil, // nil entries skipped
	})
	want := []string{http.MethodGet, http.MethodPost}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("methodsOf: got %v want %v", got, want)
	}
}

// TestServeHTTP_ssrOnlyRendersHTML asserts that a route with SSROnly=true
// still serves normal HTML requests. The __data.json enforcement guard lives
// in the pipeline but remains dormant until the __data.json endpoint lands
// (blocked by #38 / SPA router #37); once that work merges, a dedicated
// integration test covering the 404 rejection should be added here.
func TestServeHTTP_ssrOnlyRendersHTML(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/protected",
		Segments: segmentsFor("/protected"),
		Page:     staticPage("<h1>protected</h1>"),
		Options:  kit.PageOptions{SSR: true, CSR: true, SSROnly: true},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/protected")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SSROnly HTML: got %d want 200; body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "<h1>protected</h1>") {
		t.Fatalf("body missing page content: %q", body)
	}
}

// TestServeHTTP_localsReachableWithoutParent asserts that Locals written by
// the Handle hook are visible to every layout and page Load in the chain
// without any of them calling ctx.Parent(). This is the server-side
// guarantee from sveltejs/kit#11587: Handle populates Locals once before any
// Load runs; all Loads share the same map.
func TestServeHTTP_localsReachableWithoutParent(t *testing.T) {
	t.Parallel()

	const wantUser = "alice"

	// rootLoad reads Locals["user"] set by Handle. It deliberately never
	// calls ctx.Parent() to prove that Locals are pre-populated.
	rootLoad := func(ctx *kit.LoadCtx) (any, error) {
		u, _ := ctx.Locals["user"].(string)
		if u != wantUser {
			return nil, fmt.Errorf("rootLoad: Locals[user] = %q, want %q", u, wantUser)
		}
		return u, nil
	}

	// leafLoad also reads Locals["user"] without calling ctx.Parent(),
	// proving that even a nested layout loader receives the same map.
	leafLoad := func(ctx *kit.LoadCtx) (any, error) {
		u, _ := ctx.Locals["user"].(string)
		if u != wantUser {
			return nil, fmt.Errorf("leafLoad: Locals[user] = %q, want %q", u, wantUser)
		}
		return u, nil
	}

	pageLoad := func(ctx *kit.LoadCtx) (any, error) {
		u, _ := ctx.Locals["user"].(string)
		if u != wantUser {
			return nil, fmt.Errorf("pageLoad: Locals[user] = %q, want %q", u, wantUser)
		}
		return u, nil
	}

	identityLayout := func(w *render.Writer, _ *kit.RenderCtx, _ any, children func(*render.Writer) error) error {
		return children(w)
	}

	page := func(w *render.Writer, _ *kit.RenderCtx, data any) error {
		s, _ := data.(string)
		w.WriteString("<user>" + s + "</user>")
		return nil
	}

	srv, err := New(Config{
		Routes: []router.Route{{
			Pattern:     "/dashboard",
			Segments:    segmentsFor("/dashboard"),
			Page:        page,
			Load:        pageLoad,
			LayoutChain: []router.LayoutHandler{identityLayout, identityLayout},
			LayoutLoaders: []router.LayoutLoadHandler{
				rootLoad,
				leafLoad,
			},
		}},
		Shell:  testShell,
		Logger: quietLogger(),
		Hooks: kit.Hooks{
			Handle: func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
				ev.Locals["user"] = wantUser
				return resolve(ev)
			},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/dashboard")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "<user>"+wantUser+"</user>") {
		t.Fatalf("body = %q, want <user>alice</user>", body)
	}
}

// TestIsDataJSONRequest verifies the path suffix detector used by the
// SSROnly guard.
func TestIsDataJSONRequest(t *testing.T) {
	t.Parallel()
	cases := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodGet, "/foo/__data.json", true},
		{http.MethodGet, "/__data.json", true},
		{http.MethodGet, "/foo/bar/__data.json", true},
		{http.MethodPost, "/foo/__data.json", false}, // POST is not a data fetch
		{http.MethodGet, "/foo/data.json", false},    // no leading __
		{http.MethodGet, "/foo", false},
		{http.MethodGet, "/foo/__data.jsonx", false}, // suffix must match exactly
	}
	for _, tc := range cases {
		r, _ := http.NewRequest(tc.method, tc.path, nil)
		if got := isDataJSONRequest(r); got != tc.want {
			t.Errorf("isDataJSONRequest(%s %s) = %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}
}
