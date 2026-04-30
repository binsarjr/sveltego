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

func TestPipeline_LoadSetsCookie_AppearsInResponse(t *testing.T) {
	t.Parallel()

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(ctx *kit.LoadCtx) (any, error) {
			ctx.Cookies.Set("session", "abc", kit.CookieOpts{})
			return "ok", nil
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("<p>hi</p>")
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

	hdrs := resp.Header.Values("Set-Cookie")
	if len(hdrs) != 1 {
		t.Fatalf("Set-Cookie count = %d, want 1; headers = %v", len(hdrs), hdrs)
	}
	got := hdrs[0]
	for _, want := range []string{"session=abc", "Path=/", "HttpOnly", "SameSite=Lax"} {
		if !strings.Contains(got, want) {
			t.Errorf("Set-Cookie %q missing %q", got, want)
		}
	}
}

func TestPipeline_LoadSetsMultipleCookies_SeparateHeaders(t *testing.T) {
	t.Parallel()

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(ctx *kit.LoadCtx) (any, error) {
			ctx.Cookies.Set("a", "1", kit.CookieOpts{})
			ctx.Cookies.Set("b", "2", kit.CookieOpts{})
			ctx.Cookies.SetExposed("c", "3", kit.CookieOpts{})
			return nil, nil
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("<p>hi</p>")
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

	hdrs := resp.Header.Values("Set-Cookie")
	if len(hdrs) != 3 {
		t.Fatalf("Set-Cookie count = %d, want 3; headers = %v", len(hdrs), hdrs)
	}

	combined := strings.Join(hdrs, "\n")
	for _, want := range []string{"a=1", "b=2", "c=3"} {
		if !strings.Contains(combined, want) {
			t.Errorf("missing %q in headers: %v", want, hdrs)
		}
	}

	// SetExposed must produce a cookie WITHOUT HttpOnly.
	var exposed string
	for _, h := range hdrs {
		if strings.HasPrefix(h, "c=3") {
			exposed = h
		}
	}
	if exposed == "" {
		t.Fatalf("could not find c=3 header in %v", hdrs)
	}
	if strings.Contains(exposed, "HttpOnly") {
		t.Errorf("SetExposed cookie %q should not have HttpOnly", exposed)
	}
}

func TestPipeline_LoadDeleteCookie_RoundTrip(t *testing.T) {
	t.Parallel()

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(ctx *kit.LoadCtx) (any, error) {
			if v, ok := ctx.Cookies.Get("session"); !ok || v != "abc" {
				t.Errorf("Load Get(session) = (%q,%v), want (abc,true)", v, ok)
			}
			ctx.Cookies.Delete("session", kit.CookieOpts{})
			return nil, nil
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("<p>bye</p>")
			return nil
		},
	}}
	srv := newTestServer(t, routes)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	hdrs := resp.Header.Values("Set-Cookie")
	if len(hdrs) != 1 {
		t.Fatalf("Set-Cookie count = %d, want 1; headers = %v", len(hdrs), hdrs)
	}
	got := hdrs[0]
	for _, want := range []string{"session=", "Max-Age=0"} {
		if !strings.Contains(got, want) {
			t.Errorf("Delete Set-Cookie %q missing %q", got, want)
		}
	}
}
