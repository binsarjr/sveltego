package server

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/render"
	"github.com/binsarjr/sveltego/runtime/router"
)

// errorBoundary returns a handler that writes a stable marker carrying
// the SafeError fields so integration tests can assert the boundary
// fired and observed the right values.
func errorBoundary(label string) router.ErrorHandler {
	return func(w *render.Writer, _ *kit.RenderCtx, safe kit.SafeError) error {
		w.WriteString("<section class=\"err ")
		w.WriteString(label)
		w.WriteString("\">code=")
		w.WriteEscape(safe.Code)
		w.WriteString(" msg=")
		w.WriteEscape(safe.Message)
		w.WriteString("</section>")
		return nil
	}
}

func wrappingLayout(label string) router.LayoutHandler {
	return func(w *render.Writer, _ *kit.RenderCtx, _ any, children func(*render.Writer) error) error {
		w.WriteString("<div class=\"layout ")
		w.WriteString(label)
		w.WriteString("\">")
		if err := children(w); err != nil {
			return err
		}
		w.WriteString("</div>")
		return nil
	}
}

func TestErrorBoundary_RouteLocalOverridesGlobal(t *testing.T) {
	t.Parallel()
	rootError := errorBoundary("root")
	adminError := errorBoundary("admin")

	srv := newTestServer(t, []router.Route{
		{
			Pattern:  "/",
			Segments: segmentsFor("/"),
			Page:     staticPage("home"),
			Load: func(_ *kit.LoadCtx) (any, error) {
				return nil, errors.New("home boom")
			},
			Error:            rootError,
			ErrorLayoutDepth: 0,
		},
		{
			Pattern:  "/about",
			Segments: segmentsFor("/about"),
			Page:     staticPage("about"),
			Load: func(_ *kit.LoadCtx) (any, error) {
				return nil, errors.New("about boom")
			},
			Error:            adminError,
			ErrorLayoutDepth: 0,
		},
	})

	hooks := kit.Hooks{
		HandleError: func(_ *kit.RequestEvent, err error) kit.SafeError {
			return kit.SafeError{Code: http.StatusInternalServerError, Message: err.Error()}
		},
	}
	srv.hooks = hooks.WithDefaults()

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/about")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if !strings.Contains(string(body), "err admin") {
		t.Errorf("body missing admin-local boundary marker: %s", body)
	}
	if strings.Contains(string(body), "err root") {
		t.Errorf("body unexpectedly contains root boundary: %s", body)
	}
}

func TestErrorBoundary_OuterLayoutsRetained(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, []router.Route{{
		Pattern:  "/about",
		Segments: segmentsFor("/about"),
		Page:     staticPage("about"),
		LayoutChain: []router.LayoutHandler{
			wrappingLayout("outer"),
			wrappingLayout("middle"),
			wrappingLayout("inner"),
		},
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, errors.New("kaboom")
		},
		Error:            errorBoundary("middle"),
		ErrorLayoutDepth: 2,
	}})

	hooks := kit.Hooks{
		HandleError: func(_ *kit.RequestEvent, err error) kit.SafeError {
			return kit.SafeError{Code: http.StatusBadGateway, Message: err.Error()}
		},
	}
	srv.hooks = hooks.WithDefaults()

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/about")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	body, _ := io.ReadAll(resp.Body)

	s := string(body)
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
	if !strings.Contains(s, "layout outer") {
		t.Errorf("missing outer layout: %s", s)
	}
	if !strings.Contains(s, "layout middle") {
		t.Errorf("missing middle layout: %s", s)
	}
	if strings.Contains(s, "layout inner") {
		t.Errorf("inner layout should have aborted: %s", s)
	}
	if !strings.Contains(s, "err middle") {
		t.Errorf("missing boundary marker: %s", s)
	}

	// outer/middle layouts must wrap the boundary, not appear after it.
	outerIdx := strings.Index(s, "layout outer")
	middleIdx := strings.Index(s, "layout middle")
	bndIdx := strings.Index(s, "err middle")
	if !(outerIdx < middleIdx && middleIdx < bndIdx) {
		t.Errorf("layouts must wrap boundary outer→middle→err, got order outer=%d middle=%d err=%d:\n%s",
			outerIdx, middleIdx, bndIdx, s)
	}
}

func TestErrorBoundary_StatusFollowsSafeError(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page:     staticPage("home"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, errors.New("nope")
		},
		Error: errorBoundary("root"),
	}})

	hooks := kit.Hooks{
		HandleError: func(_ *kit.RequestEvent, _ error) kit.SafeError {
			return kit.SafeError{Code: http.StatusTeapot, Message: "i am a teapot", ID: "rid-42"}
		},
	}
	srv.hooks = hooks.WithDefaults()

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != http.StatusTeapot {
		t.Errorf("status = %d, want 418", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "code=418") {
		t.Errorf("missing code=418 in body: %s", body)
	}
}

func TestErrorBoundary_NoBoundaryFallsBackToPlain(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page:     staticPage("home"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, errors.New("plain")
		},
	}})

	hooks := kit.Hooks{
		HandleError: func(_ *kit.RequestEvent, err error) kit.SafeError {
			return kit.SafeError{Code: http.StatusServiceUnavailable, Message: err.Error()}
		},
	}
	srv.hooks = hooks.WithDefaults()

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", got)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "plain") {
		t.Errorf("body missing message: %s", body)
	}
}

func TestErrorBoundary_BoundaryRenderFailureFallsBack(t *testing.T) {
	t.Parallel()
	failing := func(_ *render.Writer, _ *kit.RenderCtx, _ kit.SafeError) error {
		return errors.New("boundary template exploded")
	}
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page:     staticPage("home"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, errors.New("primary")
		},
		Error: failing,
	}})

	hooks := kit.Hooks{
		HandleError: func(_ *kit.RequestEvent, err error) kit.SafeError {
			return kit.SafeError{Code: http.StatusInternalServerError, Message: err.Error()}
		},
	}
	srv.hooks = hooks.WithDefaults()

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain fallback", got)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "primary") {
		t.Errorf("body missing original error message: %s", body)
	}
}

// TestErrorPreservesHeadersAndCookies_PlainPath verifies that cookies and
// user-set response headers survive the writeSafeError path (no error boundary).
// Covers the WWW-Authenticate + Set-Cookie pattern from sveltejs/kit#9188.
func TestErrorPreservesHeadersAndCookies_PlainPath(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page:     staticPage("home"),
		Load: func(lctx *kit.LoadCtx) (any, error) {
			lctx.Cookies.Delete("session", kit.CookieOpts{})
			lctx.Header().Set("WWW-Authenticate", "Bearer")
			return nil, kit.Error(http.StatusUnauthorized, "unauthorized")
		},
	}})
	srv.hooks = kit.DefaultHooks()

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if got := resp.Header.Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("WWW-Authenticate = %q, want %q", got, "Bearer")
	}
	cookies := resp.Header.Values("Set-Cookie")
	if len(cookies) == 0 {
		t.Error("no Set-Cookie header on error response, want session deletion cookie")
	}
	var foundDeletion bool
	for _, ck := range cookies {
		if strings.Contains(ck, "session=") && strings.Contains(ck, "Max-Age=0") {
			foundDeletion = true
		}
	}
	if !foundDeletion {
		t.Errorf("Set-Cookie headers %v do not contain a session deletion cookie", cookies)
	}
}

// TestErrorPreservesHeadersAndCookies_BoundaryPath verifies that cookies and
// user-set response headers survive the renderErrorBoundary path.
func TestErrorPreservesHeadersAndCookies_BoundaryPath(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page:     staticPage("home"),
		Load: func(lctx *kit.LoadCtx) (any, error) {
			lctx.Cookies.Delete("session", kit.CookieOpts{})
			lctx.Header().Set("WWW-Authenticate", "Bearer")
			return nil, errors.New("unauthorized internal")
		},
		Error: errorBoundary("root"),
	}})
	hooks := kit.Hooks{
		HandleError: func(_ *kit.RequestEvent, _ error) kit.SafeError {
			return kit.SafeError{Code: http.StatusUnauthorized, Message: "unauthorized"}
		},
	}
	srv.hooks = hooks.WithDefaults()

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if got := resp.Header.Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("WWW-Authenticate = %q, want %q", got, "Bearer")
	}
	cookies := resp.Header.Values("Set-Cookie")
	if len(cookies) == 0 {
		t.Error("no Set-Cookie header on error boundary response, want session deletion cookie")
	}
}

// TestErrorPreservesHeaders_HandleError verifies that headers set on
// RequestEvent.ResponseHeader() inside HandleError appear in the 500 response.
func TestErrorPreservesHeaders_HandleError(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page:     staticPage("home"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, errors.New("internal boom")
		},
	}})
	hooks := kit.Hooks{
		HandleError: func(ev *kit.RequestEvent, _ error) kit.SafeError {
			ev.ResponseHeader().Set("X-Error-ID", "err-42")
			return kit.SafeError{Code: http.StatusInternalServerError, Message: "internal server error"}
		},
	}
	srv.hooks = hooks.WithDefaults()

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Error-ID"); got != "err-42" {
		t.Errorf("X-Error-ID = %q, want %q", got, "err-42")
	}
}
