package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// pageDataWithForm mirrors the codegen-emitted PageData for routes that
// declare both Load and Actions: the anonymous struct has a `Form any`
// the dispatcher fills via reflection.
type pageDataWithForm = struct {
	Greeting string
	Form     any
}

func formAwarePage() router.PageHandler {
	return func(w *render.Writer, _ *kit.RenderCtx, data any) error {
		d, _ := data.(pageDataWithForm)
		w.WriteString("<h1>")
		w.WriteString(d.Greeting)
		w.WriteString("</h1>")
		if msg, ok := d.Form.(string); ok {
			w.WriteString("<p>form=")
			w.WriteString(msg)
			w.WriteString("</p>")
		}
		return nil
	}
}

func formAwareLoad() router.LoadHandler {
	return func(_ *kit.LoadCtx) (any, error) {
		return pageDataWithForm{Greeting: "hello"}, nil
	}
}

func TestActions_DefaultPostRendersWithFormData(t *testing.T) {
	t.Parallel()
	actions := kit.ActionMap{
		"default": func(_ *kit.RequestEvent) kit.ActionResult {
			return kit.ActionDataResult(200, "ok")
		},
	}
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/login",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "login"}},
		Page:     formAwarePage(),
		Load:     formAwareLoad(),
		Actions:  func() any { return actions },
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.PostForm(ts.URL+"/login", url.Values{"email": {"alice@example.com"}})
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<p>form=ok</p>") {
		t.Fatalf("body missing form data: %s", body)
	}
}

func TestActions_NamedAction(t *testing.T) {
	t.Parallel()
	actions := kit.ActionMap{
		"submit": func(_ *kit.RequestEvent) kit.ActionResult {
			return kit.ActionDataResult(200, "submitted")
		},
	}
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/login",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "login"}},
		Page:     formAwarePage(),
		Load:     formAwareLoad(),
		Actions:  func() any { return actions },
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/login?/submit", "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "form=submitted") {
		t.Fatalf("body missing submitted form: %s", body)
	}
}

func TestActions_MissingActionName(t *testing.T) {
	t.Parallel()
	actions := kit.ActionMap{
		"default": func(_ *kit.RequestEvent) kit.ActionResult {
			return kit.ActionDataResult(200, "ok")
		},
	}
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/login",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "login"}},
		Page:     formAwarePage(),
		Load:     formAwareLoad(),
		Actions:  func() any { return actions },
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/login?/missing", "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestActions_FailRendersWithCode(t *testing.T) {
	t.Parallel()
	actions := kit.ActionMap{
		"default": func(_ *kit.RequestEvent) kit.ActionResult {
			return kit.ActionFail(422, "validation failed")
		},
	}
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/login",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "login"}},
		Page:     formAwarePage(),
		Load:     formAwareLoad(),
		Actions:  func() any { return actions },
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/login", "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "form=validation failed") {
		t.Fatalf("body missing failure data: %s", body)
	}
}

func TestActions_RedirectShortCircuits(t *testing.T) {
	t.Parallel()
	actions := kit.ActionMap{
		"default": func(_ *kit.RequestEvent) kit.ActionResult {
			return kit.ActionRedirect(0, "/dashboard")
		},
	}
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/login",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "login"}},
		Page:     formAwarePage(),
		Load:     formAwareLoad(),
		Actions:  func() any { return actions },
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Post(ts.URL+"/login", "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/dashboard" {
		t.Errorf("Location = %q, want /dashboard", loc)
	}
}

func TestActions_PostWithoutActionsReturnsMethodNotAllowed(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/static",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "static"}},
		Page:     staticPage("<h1>static</h1>"),
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/static", "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	if got := resp.Header.Get("Allow"); got != "GET" {
		t.Errorf("Allow = %q, want GET", got)
	}
}

func newTestServerWithHooks(t *testing.T, routes []router.Route, hooks kit.Hooks) *Server {
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

func TestHandleAction_shortCircuitsAction(t *testing.T) {
	t.Parallel()
	actions := kit.ActionMap{
		"default": func(_ *kit.RequestEvent) kit.ActionResult {
			return kit.ActionDataResult(200, "reached")
		},
	}
	hooks := kit.Hooks{
		HandleAction: func(_ *kit.RequestEvent, _ string, _ kit.ActionFn) kit.ActionResult {
			return kit.ActionFail(403, "csrf check failed")
		},
	}
	srv := newTestServerWithHooks(t, []router.Route{{
		Pattern:  "/login",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "login"}},
		Page:     formAwarePage(),
		Load:     formAwareLoad(),
		Actions:  func() any { return actions },
	}}, hooks)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/login", "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	// HandleAction short-circuits with ActionFail(403) — pipeline re-renders
	// the page with status 403 and the failure data in Form.
	if resp.StatusCode != 403 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 403, body = %s", resp.StatusCode, body)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "csrf check failed") {
		t.Fatalf("expected csrf message in body, got: %s", body)
	}
}

func TestHandleAction_wrapsAction(t *testing.T) {
	t.Parallel()
	var trail []string
	actions := kit.ActionMap{
		"submit": func(_ *kit.RequestEvent) kit.ActionResult {
			trail = append(trail, "action")
			return kit.ActionDataResult(200, "ok")
		},
	}
	hooks := kit.Hooks{
		HandleAction: func(ev *kit.RequestEvent, name string, next kit.ActionFn) kit.ActionResult {
			trail = append(trail, "before:"+name)
			r := next(ev)
			trail = append(trail, "after:"+name)
			return r
		},
	}
	srv := newTestServerWithHooks(t, []router.Route{{
		Pattern:  "/form",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "form"}},
		Page:     formAwarePage(),
		Load:     formAwareLoad(),
		Actions:  func() any { return actions },
	}}, hooks)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/form?/submit", "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	want := []string{"before:submit", "action", "after:submit"}
	if strings.Join(trail, ",") != strings.Join(want, ",") {
		t.Errorf("trail = %v, want %v", trail, want)
	}
}

func TestHandleAction_receivesActionName(t *testing.T) {
	t.Parallel()
	var capturedName string
	actions := kit.ActionMap{
		"checkout": func(_ *kit.RequestEvent) kit.ActionResult {
			return kit.ActionDataResult(200, "done")
		},
	}
	hooks := kit.Hooks{
		HandleAction: func(ev *kit.RequestEvent, name string, next kit.ActionFn) kit.ActionResult {
			capturedName = name
			return next(ev)
		},
	}
	srv := newTestServerWithHooks(t, []router.Route{{
		Pattern:  "/cart",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "cart"}},
		Page:     formAwarePage(),
		Load:     formAwareLoad(),
		Actions:  func() any { return actions },
	}}, hooks)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/cart?/checkout", "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if capturedName != "checkout" {
		t.Errorf("capturedName = %q, want checkout", capturedName)
	}
}

func TestActions_BindFormInsideAction(t *testing.T) {
	t.Parallel()
	type loginForm struct {
		Email    string `form:"email"`
		Password string `form:"password"`
	}
	actions := kit.ActionMap{
		"default": func(ev *kit.RequestEvent) kit.ActionResult {
			var f loginForm
			if err := ev.BindForm(&f); err != nil {
				return kit.ActionFail(400, err.Error())
			}
			return kit.ActionDataResult(200, f.Email+":"+f.Password)
		},
	}
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/login",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "login"}},
		Page:     formAwarePage(),
		Load:     formAwareLoad(),
		Actions:  func() any { return actions },
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.PostForm(ts.URL+"/login", url.Values{
		"email":    {"u@e.com"},
		"password": {"hunter2"},
	})
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "form=u@e.com:hunter2") {
		t.Fatalf("body = %s", body)
	}
}
