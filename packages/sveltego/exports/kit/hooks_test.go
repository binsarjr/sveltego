package kit_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/exports/kit"
)

func newEvent(t *testing.T) *kit.RequestEvent {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "https://example.com/posts/42", nil)
	return kit.NewRequestEvent(r, map[string]string{"id": "42"})
}

func TestNewRequestEvent_initializesFields(t *testing.T) {
	t.Parallel()
	ev := newEvent(t)

	if ev.Request == nil {
		t.Fatal("Request = nil")
	}
	if ev.URL == nil || ev.URL.Path != "/posts/42" {
		t.Errorf("URL = %v, want /posts/42", ev.URL)
	}
	if ev.OriginalURL != ev.URL {
		t.Error("OriginalURL must alias URL on construction")
	}
	if ev.Locals == nil {
		t.Error("Locals = nil")
	}
	if ev.Cookies == nil {
		t.Error("Cookies = nil")
	}
	if ev.Params["id"] != "42" {
		t.Errorf("Params[id] = %q, want 42", ev.Params["id"])
	}
	if ev.MatchPath != "" {
		t.Errorf("MatchPath = %q, want empty", ev.MatchPath)
	}
}

func TestSequence_runsLeftToRight(t *testing.T) {
	t.Parallel()
	var trail []string

	h1 := func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
		trail = append(trail, "h1-pre")
		res, err := resolve(ev)
		trail = append(trail, "h1-post")
		return res, err
	}
	h2 := func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
		trail = append(trail, "h2-pre")
		res, err := resolve(ev)
		trail = append(trail, "h2-post")
		return res, err
	}
	h3 := func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
		trail = append(trail, "h3-pre")
		res, err := resolve(ev)
		trail = append(trail, "h3-post")
		return res, err
	}

	chain := kit.Sequence(h1, h2, h3)
	resolve := func(_ *kit.RequestEvent) (*kit.Response, error) {
		trail = append(trail, "route")
		return kit.NewResponse(http.StatusOK, []byte("ok")), nil
	}

	res, err := chain(newEvent(t), resolve)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if res == nil || string(res.Body) != "ok" {
		t.Fatalf("res = %#v", res)
	}
	want := []string{"h1-pre", "h2-pre", "h3-pre", "route", "h3-post", "h2-post", "h1-post"}
	if strings.Join(trail, ",") != strings.Join(want, ",") {
		t.Errorf("order:\n got %v\nwant %v", trail, want)
	}
}

func TestSequence_shortCircuit(t *testing.T) {
	t.Parallel()
	resolved := false

	short := func(_ *kit.RequestEvent, _ kit.ResolveFn) (*kit.Response, error) {
		return kit.NewResponse(http.StatusForbidden, []byte("nope")), nil
	}
	tail := func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
		t.Error("tail must not run after short-circuit")
		return resolve(ev)
	}
	resolve := func(_ *kit.RequestEvent) (*kit.Response, error) {
		resolved = true
		return kit.NewResponse(http.StatusOK, nil), nil
	}

	res, err := kit.Sequence(short, tail)(newEvent(t), resolve)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if res.Status != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.Status)
	}
	if resolved {
		t.Error("resolve fired despite short-circuit")
	}
}

func TestSequence_errorPropagates(t *testing.T) {
	t.Parallel()
	want := errors.New("boom")
	failing := func(_ *kit.RequestEvent, _ kit.ResolveFn) (*kit.Response, error) {
		return nil, want
	}
	resolve := func(_ *kit.RequestEvent) (*kit.Response, error) {
		t.Error("resolve must not run when handler errors before resolve")
		return nil, nil
	}

	_, err := kit.Sequence(failing)(newEvent(t), resolve)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestSequence_emptyChain(t *testing.T) {
	t.Parallel()
	resolved := false
	resolve := func(_ *kit.RequestEvent) (*kit.Response, error) {
		resolved = true
		return kit.NewResponse(http.StatusOK, nil), nil
	}
	if _, err := kit.Sequence()(newEvent(t), resolve); err != nil {
		t.Fatalf("empty chain: %v", err)
	}
	if !resolved {
		t.Error("empty chain must call resolve")
	}
}

func TestIdentityHandle_callsResolveOnce(t *testing.T) {
	t.Parallel()
	calls := 0
	resolve := func(_ *kit.RequestEvent) (*kit.Response, error) {
		calls++
		return kit.NewResponse(http.StatusOK, []byte("x")), nil
	}
	res, err := kit.IdentityHandle(newEvent(t), resolve)
	if err != nil {
		t.Fatalf("IdentityHandle: %v", err)
	}
	if calls != 1 {
		t.Errorf("resolve calls = %d, want 1", calls)
	}
	if string(res.Body) != "x" {
		t.Errorf("body = %q, want x", res.Body)
	}
}

func TestIdentityReroute_returnsEmpty(t *testing.T) {
	t.Parallel()
	u, _ := url.Parse("https://example.com/legacy/blog")
	if got := kit.IdentityReroute(u); got != "" {
		t.Errorf("IdentityReroute = %q, want empty", got)
	}
}

func TestIdentityHandleError_returns500(t *testing.T) {
	t.Parallel()
	se, err := kit.IdentityHandleError(newEvent(t), errors.New("x"))
	if err != nil {
		t.Fatalf("IdentityHandleError returned unexpected error: %v", err)
	}
	if se.Code != http.StatusInternalServerError {
		t.Errorf("Code = %d, want 500", se.Code)
	}
	if se.Message == "" {
		t.Error("Message empty")
	}
}

func TestIdentityInit_returnsNil(t *testing.T) {
	t.Parallel()
	if err := kit.IdentityInit(context.Background()); err != nil {
		t.Errorf("IdentityInit: %v", err)
	}
}

func TestSafeError_implementsErrorAndStatuser(t *testing.T) {
	t.Parallel()
	se := kit.SafeError{Code: 418, Message: "teapot"}
	var err error = se
	if err.Error() != "teapot" {
		t.Errorf("Error() = %q, want teapot", err.Error())
	}
	if se.HTTPStatus() != 418 {
		t.Errorf("HTTPStatus = %d, want 418", se.HTTPStatus())
	}

	zero := kit.SafeError{}
	if zero.HTTPStatus() != http.StatusInternalServerError {
		t.Errorf("zero HTTPStatus = %d, want 500", zero.HTTPStatus())
	}
	if zero.Error() == "" {
		t.Error("zero Error empty")
	}
}

func TestHooks_WithDefaults_fillsNil(t *testing.T) {
	t.Parallel()
	h := kit.Hooks{}.WithDefaults()
	if h.Handle == nil || h.HandleError == nil || h.HandleFetch == nil || h.HandleAction == nil || h.Reroute == nil || h.Init == nil {
		t.Fatalf("WithDefaults left nil fields: %+v", h)
	}
}

func TestIdentityHandleAction_callsNext(t *testing.T) {
	t.Parallel()
	ev := newEvent(t)
	called := false
	next := func(_ *kit.RequestEvent) kit.ActionResult {
		called = true
		return kit.ActionDataResult(200, "ok")
	}
	result := kit.IdentityHandleAction(ev, "default", next)
	if !called {
		t.Error("IdentityHandleAction did not call next")
	}
	ad, ok := result.(kit.ActionData)
	if !ok || ad.Data != "ok" {
		t.Errorf("unexpected result: %#v", result)
	}
}

func TestHandleActionFn_shortCircuit(t *testing.T) {
	t.Parallel()
	ev := newEvent(t)
	// Middleware that rejects without calling next (CSRF use-case).
	reject := func(_ *kit.RequestEvent, _ string, _ kit.ActionFn) kit.ActionResult {
		return kit.ActionFail(403, "forbidden")
	}
	called := false
	next := func(_ *kit.RequestEvent) kit.ActionResult {
		called = true
		return kit.ActionDataResult(200, "reached")
	}
	result := reject(ev, "default", next)
	if called {
		t.Error("next should not be called on short-circuit")
	}
	afd, ok := result.(kit.ActionFailData)
	if !ok || afd.Code != 403 {
		t.Errorf("unexpected result: %#v", result)
	}
}

func TestHandleActionFn_wraps(t *testing.T) {
	t.Parallel()
	ev := newEvent(t)
	var trail []string
	// Middleware that runs before and after next.
	wrap := func(ev *kit.RequestEvent, name string, next kit.ActionFn) kit.ActionResult {
		trail = append(trail, "before:"+name)
		r := next(ev)
		trail = append(trail, "after:"+name)
		return r
	}
	next := func(_ *kit.RequestEvent) kit.ActionResult {
		trail = append(trail, "action")
		return kit.ActionDataResult(200, "done")
	}
	result := wrap(ev, "submit", next)
	want := []string{"before:submit", "action", "after:submit"}
	if strings.Join(trail, ",") != strings.Join(want, ",") {
		t.Errorf("trail = %v, want %v", trail, want)
	}
	ad, ok := result.(kit.ActionData)
	if !ok || ad.Data != "done" {
		t.Errorf("unexpected result: %#v", result)
	}
}

func TestHooks_WithDefaults_preservesUserHooks(t *testing.T) {
	t.Parallel()
	called := false
	user := kit.Hooks{
		Handle: func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
			called = true
			return resolve(ev)
		},
	}
	filled := user.WithDefaults()
	if _, err := filled.Handle(newEvent(t), func(_ *kit.RequestEvent) (*kit.Response, error) {
		return kit.NewResponse(http.StatusOK, nil), nil
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !called {
		t.Error("user Handle not preserved")
	}
}

func TestRequestEvent_Fetch_fallbackToDefaultClient(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("origin"))
	}))
	t.Cleanup(srv.Close)

	ev := newEvent(t)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := ev.Fetch(req)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestRequestEvent_Fetch_routesThroughHook(t *testing.T) {
	t.Parallel()
	called := false
	ev := newEvent(t)
	ev.SetFetcher(func(_ *kit.RequestEvent, req *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{
			StatusCode: http.StatusTeapot,
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})
	req := httptest.NewRequest(http.MethodGet, "https://api.example.com/x", nil)
	resp, err := ev.Fetch(req)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer resp.Body.Close()
	if !called {
		t.Error("hook not invoked")
	}
	if resp.StatusCode != http.StatusTeapot {
		t.Errorf("status = %d, want 418", resp.StatusCode)
	}
}
