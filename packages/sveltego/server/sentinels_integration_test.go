package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/render"
	"github.com/binsarjr/sveltego/runtime/router"
)

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
