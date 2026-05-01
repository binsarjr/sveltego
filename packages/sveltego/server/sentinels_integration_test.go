package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

type notFoundErr struct{ Resource string }

func (e *notFoundErr) Error() string  { return "not found: " + e.Resource }
func (e *notFoundErr) Status() int    { return http.StatusNotFound }
func (e *notFoundErr) Public() string { return e.Resource + " does not exist" }

func TestPipeline_LoadReturnsRedirect_303WithLocation(t *testing.T) {
	t.Parallel()

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, kit.Redirect(303, "/login")
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("<p>should not render</p>")
			return nil
		},
	}}
	srv := newTestServer(t, routes)
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
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 303 {
		t.Errorf("status = %d, want 303", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/login" {
		t.Errorf("Location = %q, want /login", got)
	}
	if strings.Contains(string(body), "should not render") {
		t.Errorf("body leaked page content: %s", body)
	}
}

func TestPipeline_LoadReturnsError_404WithMessage(t *testing.T) {
	t.Parallel()

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, kit.Error(404, "post not found")
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("<p>should not render</p>")
			return nil
		},
	}}
	srv := newTestServer(t, routes)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	if !strings.Contains(string(body), "post not found") {
		t.Errorf("body = %q, want to contain post not found", body)
	}
	if strings.Contains(string(body), "should not render") {
		t.Errorf("body leaked page content: %s", body)
	}
}

func TestPipeline_LoadReturnsFail_500OutsideAction(t *testing.T) {
	t.Parallel()

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, kit.Fail(400, map[string]string{"email": "required"})
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("<p>should not render</p>")
			return nil
		},
	}}
	srv := newTestServer(t, routes)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != 500 {
		t.Errorf("status = %d, want 500 (Fail outside action context)", resp.StatusCode)
	}
}

func TestPipeline_LoadReturnsRedirect_307PreservesMethod(t *testing.T) {
	t.Parallel()

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, kit.Redirect(307, "/elsewhere")
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("nope")
			return nil
		},
	}}
	srv := newTestServer(t, routes)
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
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != 307 {
		t.Errorf("status = %d, want 307", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/elsewhere" {
		t.Errorf("Location = %q, want /elsewhere", got)
	}
}

func TestPipeline_RedirectReload_SetsReloadHeader(t *testing.T) {
	t.Parallel()

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, kit.Redirect(303, "/login", kit.RedirectReload())
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("should not render")
			return nil
		},
	}}
	srv := newTestServer(t, routes)
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
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != 303 {
		t.Errorf("status = %d, want 303", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/login" {
		t.Errorf("Location = %q, want /login", got)
	}
	if got := resp.Header.Get("X-Sveltego-Reload"); got != "1" {
		t.Errorf("X-Sveltego-Reload = %q, want 1", got)
	}
}

func TestPipeline_PlainRedirect_NoReloadHeader(t *testing.T) {
	t.Parallel()

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, kit.Redirect(303, "/login")
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("should not render")
			return nil
		},
	}}
	srv := newTestServer(t, routes)
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
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != 303 {
		t.Errorf("status = %d, want 303", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Sveltego-Reload"); got != "" {
		t.Errorf("X-Sveltego-Reload = %q, want empty for plain Redirect", got)
	}
}

func TestPipeline_UserHTTPError_RendersStatus(t *testing.T) {
	t.Parallel()

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, &notFoundErr{Resource: "post"}
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("<p>should not render</p>")
			return nil
		},
		Error: errorBoundary("user-http-error"),
	}}
	srv := newTestServer(t, routes)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	body, _ := io.ReadAll(resp.Body)
	s := string(body)

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	if strings.Contains(s, "should not render") {
		t.Errorf("page content leaked into error response: %s", s)
	}
	if !strings.Contains(s, "code=404") {
		t.Errorf("error boundary missing code=404: %s", s)
	}
	if !strings.Contains(s, "post does not exist") {
		t.Errorf("error boundary missing public message: %s", s)
	}
}

func TestPipeline_UserHTTPError_WrappedDetected(t *testing.T) {
	t.Parallel()

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, fmt.Errorf("load: %w", &notFoundErr{Resource: "article"})
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("nope")
			return nil
		},
		Error: errorBoundary("wrapped"),
	}}
	srv := newTestServer(t, routes)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 through wrapping", resp.StatusCode)
	}
	if !strings.Contains(string(body), "code=404") {
		t.Errorf("error boundary missing code=404: %s", body)
	}
}

func TestPipeline_PlainError_DefaultsTo500(t *testing.T) {
	t.Parallel()

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, errors.New("something internal")
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("nope")
			return nil
		},
	}}
	srv := newTestServer(t, routes)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 for plain errors", resp.StatusCode)
	}
}

func TestPipeline_ExistingKitError_Regression(t *testing.T) {
	t.Parallel()

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, kit.Error(403, "forbidden")
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("nope")
			return nil
		},
	}}
	srv := newTestServer(t, routes)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (kit.Error regression)", resp.StatusCode)
	}
	if !strings.Contains(string(body), "forbidden") {
		t.Errorf("body missing message: %s", body)
	}
}
