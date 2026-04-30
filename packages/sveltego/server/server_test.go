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

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/exports/kit/params"
	"github.com/binsarjr/sveltego/render"
	"github.com/binsarjr/sveltego/runtime/router"
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
