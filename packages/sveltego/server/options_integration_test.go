package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

func helloPage() router.PageHandler {
	return func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
		w.WriteString("<h1>hello</h1>")
		return nil
	}
}

// noRedirectClient prevents the std client from following 308s so we
// can assert on the redirect response itself.
func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func TestServeHTTP_trailingSlashAlways_redirects(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/about",
		Segments: segmentsFor("/about"),
		Page:     helloPage(),
		Options:  kit.PageOptions{SSR: true, CSR: true, TrailingSlash: kit.TrailingSlashAlways},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := noRedirectClient().Get(ts.URL + "/about")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPermanentRedirect {
		t.Fatalf("status: got %d want 308", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/about/" {
		t.Fatalf("Location: got %q want /about/", loc)
	}
}

func TestServeHTTP_trailingSlashNever_redirects(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/about",
		Segments: segmentsFor("/about"),
		Page:     helloPage(),
		Options:  kit.PageOptions{SSR: true, CSR: true, TrailingSlash: kit.TrailingSlashNever},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	// /about/ should normalize to /about.
	resp, err := noRedirectClient().Get(ts.URL + "/about/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPermanentRedirect {
		t.Fatalf("status: got %d want 308", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/about" {
		t.Fatalf("Location: got %q want /about", loc)
	}
}

func TestServeHTTP_trailingSlashIgnore_noRedirect(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/about",
		Segments: segmentsFor("/about"),
		Page:     helloPage(),
		Options:  kit.PageOptions{SSR: true, CSR: true, TrailingSlash: kit.TrailingSlashIgnore},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := noRedirectClient().Get(ts.URL + "/about")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
}

func TestServeHTTP_ssrFalse_emitsEmptyShell(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/about",
		Segments: segmentsFor("/about"),
		Page:     helloPage(),
		Options:  kit.PageOptions{SSR: false, CSR: true, TrailingSlash: kit.TrailingSlashIgnore},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/about")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d", resp.StatusCode)
	}
	body := readAll(t, resp)
	if !strings.Contains(body, `<div id="app"></div>`) {
		t.Fatalf("expected empty mount; got:\n%s", body)
	}
	if strings.Contains(body, "<h1>hello</h1>") {
		t.Fatalf("page renderer ran for SSR=false route; got:\n%s", body)
	}
}

func TestServeHTTP_zeroOptions_keepsLegacyRender(t *testing.T) {
	t.Parallel()
	// A route with the zero-value PageOptions (no codegen Options
	// emitted) must still render normally — older manifests do not
	// populate the Options field.
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/about",
		Segments: segmentsFor("/about"),
		Page:     helloPage(),
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/about")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body := readAll(t, resp)
	if !strings.Contains(body, "<h1>hello</h1>") {
		t.Fatalf("legacy render skipped; got:\n%s", body)
	}
}

func TestServeHTTP_restDispatch(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/api/data",
		Segments: segmentsFor("/api/data"),
		Server: router.ServerHandlers{
			http.MethodGet: dispatchKitVerb(func(_ *kit.RequestEvent) *kit.Response {
				return kit.JSON(http.StatusOK, kit.M{"ok": true})
			}),
			http.MethodPost: dispatchKitVerb(func(_ *kit.RequestEvent) *kit.Response {
				return kit.NoContent()
			}),
		},
	}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	t.Run("get_json", func(t *testing.T) {
		t.Parallel()
		resp, err := http.Get(ts.URL + "/api/data")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status: %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("content-type: %q", ct)
		}
		body := readAll(t, resp)
		if !strings.Contains(body, `"ok":true`) {
			t.Fatalf("body: %q", body)
		}
	})

	t.Run("post_204", func(t *testing.T) {
		t.Parallel()
		resp, err := http.Post(ts.URL+"/api/data", "", strings.NewReader(""))
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("status: %d", resp.StatusCode)
		}
	})

	t.Run("delete_405_with_allow", func(t *testing.T) {
		t.Parallel()
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/data", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DELETE: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("status: got %d want 405", resp.StatusCode)
		}
		if got := resp.Header.Get("Allow"); got != "GET, POST" {
			t.Fatalf("Allow: got %q want GET, POST", got)
		}
	})
}

// dispatchKitVerb mirrors the wrapper the codegen-emitted
// server.gen.go uses; declared inline here so the integration test
// exercises the same translation surface without invoking the build
// pipeline. The test for the codegen output lives in
// internal/codegen/rest_emit_test.go.
func dispatchKitVerb(verb func(*kit.RequestEvent) *kit.Response) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ev := kit.NewRequestEvent(r, nil)
		res := verb(ev)
		if res == nil {
			res = kit.NewResponse(http.StatusNoContent, nil)
		}
		if ev.Cookies != nil {
			ev.Cookies.Apply(w)
		}
		for k, vs := range res.Headers {
			w.Header()[k] = vs
		}
		status := res.Status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		if len(res.Body) > 0 {
			_, _ = w.Write(res.Body)
		}
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	buf, err := readAllBytes(resp)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(buf)
}

func readAllBytes(resp *http.Response) ([]byte, error) {
	const maxBody = 1 << 20
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if len(buf) > maxBody {
				return buf, nil
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			return buf, err
		}
	}
}
