package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/render"
	"github.com/binsarjr/sveltego/runtime/router"
)

func newHookServer(t *testing.T, hooks kit.Hooks, routes []router.Route) *Server {
	t.Helper()
	srv, err := New(Config{
		Routes: routes,
		Shell:  testShell,
		Logger: quietLogger(),
		Hooks:  hooks,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv
}

func TestHooks_HandleAddsHeader(t *testing.T) {
	t.Parallel()
	hooks := kit.Hooks{
		Handle: func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
			ev.Locals["rid"] = "abc-123"
			res, err := resolve(ev)
			if err != nil || res == nil {
				return res, err
			}
			res.Headers.Set("X-Request-ID", "abc-123")
			return res, nil
		},
	}
	srv := newHookServer(t, hooks, []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page:     staticPage("ok"),
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if got := resp.Header.Get("X-Request-ID"); got != "abc-123" {
		t.Errorf("X-Request-ID = %q, want abc-123", got)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestHooks_SequenceOrdering(t *testing.T) {
	t.Parallel()
	first := func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
		res, err := resolve(ev)
		if res != nil {
			res.Headers.Add("X-Trail", "first")
		}
		return res, err
	}
	second := func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
		res, err := resolve(ev)
		if res != nil {
			res.Headers.Add("X-Trail", "second")
		}
		return res, err
	}
	hooks := kit.Hooks{Handle: kit.Sequence(first, second)}
	srv := newHookServer(t, hooks, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("x"),
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	got := resp.Header.Values("X-Trail")
	want := []string{"second", "first"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("trail = %v, want %v", got, want)
	}
}

func TestHooks_ShortCircuitSkipsRoute(t *testing.T) {
	t.Parallel()
	pageHit := false
	hooks := kit.Hooks{
		Handle: func(_ *kit.RequestEvent, _ kit.ResolveFn) (*kit.Response, error) {
			res := kit.NewResponse(http.StatusForbidden, []byte("nope"))
			res.Headers.Set("Content-Type", "text/plain; charset=utf-8")
			return res, nil
		},
	}
	srv := newHookServer(t, hooks, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"),
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			pageHit = true
			w.WriteString("never")
			return nil
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if pageHit {
		t.Error("page invoked despite short-circuit")
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
	if string(body) != "nope" {
		t.Errorf("body = %q, want nope", body)
	}
}

func TestHooks_HandleErrorTransformsLoadError(t *testing.T) {
	t.Parallel()
	hooks := kit.Hooks{
		HandleError: func(_ *kit.RequestEvent, _ error) (kit.SafeError, error) {
			return kit.SafeError{Code: http.StatusBadGateway, Message: "upstream", ID: "rid-1"}, nil
		},
	}
	srv := newHookServer(t, hooks, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"),
		Page: staticPage("x"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, errors.New("internal")
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "upstream") {
		t.Errorf("body = %q, want upstream", body)
	}
}

func TestHooks_RerouteAffectsMatching(t *testing.T) {
	t.Parallel()
	hooks := kit.Hooks{
		Reroute: func(u *url.URL) string {
			if strings.HasPrefix(u.Path, "/legacy/") {
				return strings.Replace(u.Path, "/legacy/", "/", 1)
			}
			return ""
		},
		Handle: func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
			res, err := resolve(ev)
			if err != nil || res == nil {
				return res, err
			}
			res.Headers.Set("X-Original", ev.OriginalURL.Path)
			res.Headers.Set("X-Match", ev.MatchPath)
			return res, nil
		},
	}
	srv := newHookServer(t, hooks, []router.Route{{
		Pattern: "/about", Segments: segmentsFor("/about"),
		Page: staticPage("about"),
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/legacy/about")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), "about") {
		t.Errorf("body = %q, want about", body)
	}
	if got := resp.Header.Get("X-Original"); got != "/legacy/about" {
		t.Errorf("X-Original = %q, want /legacy/about", got)
	}
	if got := resp.Header.Get("X-Match"); got != "/about" {
		t.Errorf("X-Match = %q, want /about", got)
	}
}

func TestHooks_FetchRoutesThroughHandleFetch(t *testing.T) {
	t.Parallel()
	calls := 0
	hooks := kit.Hooks{
		HandleFetch: func(_ *kit.RequestEvent, req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{
				StatusCode: http.StatusTeapot,
				Body:       http.NoBody,
				Request:    req,
				Header:     http.Header{},
			}, nil
		},
		Handle: func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
			req, _ := http.NewRequestWithContext(ev.Request.Context(), http.MethodGet, "https://api.example.com/x", nil)
			resp, err := ev.Fetch(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			res, err := resolve(ev)
			if err != nil || res == nil {
				return res, err
			}
			res.Headers.Set("X-Upstream-Status", http.StatusText(resp.StatusCode))
			return res, nil
		},
	}
	srv := newHookServer(t, hooks, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("ok"),
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if calls != 1 {
		t.Errorf("HandleFetch calls = %d, want 1", calls)
	}
	if got := resp.Header.Get("X-Upstream-Status"); got != http.StatusText(http.StatusTeapot) {
		t.Errorf("X-Upstream-Status = %q", got)
	}
}

func TestHooks_InitRunsBeforeFirstRequest(t *testing.T) {
	t.Parallel()
	called := false
	hooks := kit.Hooks{
		Init: func(_ context.Context) error {
			called = true
			return nil
		},
	}
	srv := newHookServer(t, hooks, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("x"),
	}})
	if err := srv.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !called {
		t.Error("Init not called")
	}
}

func TestHooks_InitErrorServesFallback(t *testing.T) {
	t.Parallel()
	hooks := kit.Hooks{
		Init: func(_ context.Context) error { return errors.New("init boom") },
	}
	srv, err := New(Config{
		Routes:        []router.Route{{Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("x")}},
		Shell:         testShell,
		Logger:        quietLogger(),
		Hooks:         hooks,
		InitErrorHTML: "<p>failed</p>",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := srv.Init(context.Background()); err == nil {
		t.Fatal("expected Init to return error")
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err2 := http.Get(ts.URL + "/")
	if err2 != nil {
		t.Fatalf("GET: %v", err2)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if !strings.Contains(string(body), "failed") {
		t.Errorf("body = %q, want init error HTML", body)
	}
}

func TestHooks_HandleErrorIDPropagates(t *testing.T) {
	t.Parallel()
	hooks := kit.Hooks{
		HandleError: func(_ *kit.RequestEvent, _ error) (kit.SafeError, error) {
			return kit.SafeError{Code: 503, Message: "down", ID: "rid-x"}, nil
		},
	}
	srv := newHookServer(t, hooks, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"),
		Page: staticPage("x"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, errors.New("oops")
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}

func TestHooks_HandleErrorCatchesHandleErrors(t *testing.T) {
	t.Parallel()
	hooks := kit.Hooks{
		Handle: func(_ *kit.RequestEvent, _ kit.ResolveFn) (*kit.Response, error) {
			return nil, errors.New("handle exploded")
		},
		HandleError: func(_ *kit.RequestEvent, _ error) (kit.SafeError, error) {
			return kit.SafeError{Code: 502, Message: "handle bad"}, nil
		},
	}
	srv := newHookServer(t, hooks, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("x"),
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "handle bad") {
		t.Errorf("body = %q, want handle bad", body)
	}
}

func TestHooks_PanicInLoadFlowsToHandleError(t *testing.T) {
	t.Parallel()
	hooks := kit.Hooks{
		HandleError: func(_ *kit.RequestEvent, err error) (kit.SafeError, error) {
			return kit.SafeError{Code: http.StatusInternalServerError, Message: "sanitized: " + err.Error(), ID: "rid-9"}, nil
		},
	}
	srv := newHookServer(t, hooks, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"),
		Page: staticPage("x"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			panic("kaboom")
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "kaboom") {
		t.Errorf("body missing panic message: %q", body)
	}
}

func TestHooks_DefaultHooks_passthrough(t *testing.T) {
	t.Parallel()
	srv := newHookServer(t, kit.Hooks{}, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("home"),
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "home") {
		t.Errorf("status=%d body=%q", resp.StatusCode, body)
	}
}

func TestHandleError_RedirectShortCircuit(t *testing.T) {
	t.Parallel()
	hooks := kit.Hooks{
		HandleError: func(_ *kit.RequestEvent, _ error) (kit.SafeError, error) {
			return kit.SafeError{}, kit.Redirect(302, "/login")
		},
	}
	srv := newHookServer(t, hooks, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"),
		Page: staticPage("secret"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, errors.New("unauthenticated")
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("status = %d, want 302", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/login" {
		t.Errorf("Location = %q, want /login", got)
	}
}

func TestHandleError_HTTPErrShortCircuit(t *testing.T) {
	t.Parallel()
	hooks := kit.Hooks{
		HandleError: func(_ *kit.RequestEvent, _ error) (kit.SafeError, error) {
			return kit.SafeError{}, kit.Error(http.StatusPaymentRequired, "subscribe to unlock")
		},
	}
	srv := newHookServer(t, hooks, []router.Route{{
		Pattern: "/about", Segments: segmentsFor("/about"),
		Page: staticPage("premium"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, errors.New("paywall")
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/about")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("status = %d, want 402", resp.StatusCode)
	}
	if !strings.Contains(string(body), "subscribe to unlock") {
		t.Errorf("body = %q, want subscribe to unlock", body)
	}
}

func TestHandleError_PlainErrorFallsThroughToBoundary(t *testing.T) {
	t.Parallel()
	hooks := kit.Hooks{
		HandleError: func(_ *kit.RequestEvent, _ error) (kit.SafeError, error) {
			return kit.SafeError{Code: http.StatusInternalServerError, Message: "boundary reached"}, nil
		},
	}
	srv := newHookServer(t, hooks, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"),
		Page: staticPage("x"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, errors.New("plain error")
		},
	}})
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
	if !strings.Contains(string(body), "boundary reached") {
		t.Errorf("body = %q, want boundary reached", body)
	}
}
