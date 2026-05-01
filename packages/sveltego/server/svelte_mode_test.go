package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// TestServeHTTP_svelteMode_servesShellWithPayload verifies the
// Phase-3 Svelte pipeline: the server returns the app.html shell with
// the JSON hydration payload injected and no Mustache-Go body. The
// route has Templates: "svelte" + a Load that returns a typed payload.
// No Page handler is wired — the manifest never emitted one because
// Vite + Svelte own the .svelte body.
func TestServeHTTP_svelteMode_servesShellWithPayload(t *testing.T) {
	t.Parallel()
	type pageData struct {
		Greeting string `json:"greeting"`
	}
	route := router.Route{
		Pattern:  "/",
		Segments: []router.Segment{},
		Load: func(_ *kit.LoadCtx) (any, error) {
			return pageData{Greeting: "hello svelte"}, nil
		},
		Options: kit.PageOptions{
			SSR:       true,
			CSR:       true,
			CSRF:      true,
			Templates: kit.TemplatesSvelte,
		},
	}
	srv := newTestServer(t, []router.Route{route})
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
	s := string(body)
	if !strings.Contains(s, `<div id="app"></div>`) {
		t.Errorf("body missing SPA mount node:\n%s", s)
	}
	if !strings.Contains(s, `<script id="sveltego-data" type="application/json">`) {
		t.Errorf("body missing hydration payload tag:\n%s", s)
	}
	// Pull the payload out and verify it round-trips Load.
	start := strings.Index(s, `>{`)
	end := strings.Index(s[start:], `</script>`)
	if start < 0 || end < 0 {
		t.Fatalf("payload script tag malformed:\n%s", s)
	}
	rawPayload := s[start+1 : start+end]
	var p map[string]any
	if err := json.Unmarshal([]byte(rawPayload), &p); err != nil {
		t.Fatalf("payload json: %v\nraw: %s", err, rawPayload)
	}
	data, ok := p["data"].(map[string]any)
	if !ok {
		t.Fatalf("payload.data missing or wrong type: %v", p)
	}
	if data["greeting"] != "hello svelte" {
		t.Errorf("payload.data.greeting = %v, want hello svelte", data["greeting"])
	}
}

// TestServeHTTP_svelteMode_noPageHandlerStillRenders confirms the
// shell path bypasses the legacy `route.Page == nil` 404 guard when
// Templates: "svelte" is set.
func TestServeHTTP_svelteMode_noPageHandlerStillRenders(t *testing.T) {
	t.Parallel()
	route := router.Route{
		Pattern:  "/",
		Segments: []router.Segment{},
		Options: kit.PageOptions{
			SSR:       true,
			CSR:       true,
			CSRF:      true,
			Templates: kit.TemplatesSvelte,
		},
	}
	srv := newTestServer(t, []router.Route{route})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200 (Svelte route should bypass nil-Page guard)", resp.StatusCode)
	}
}
