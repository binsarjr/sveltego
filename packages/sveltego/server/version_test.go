package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit/params"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// TestComputeAppVersion_deterministic checks the same manifest input
// always produces the same digest — the whole feature collapses if a
// rebuild with identical bytes flips the hash.
func TestComputeAppVersion_deterministic(t *testing.T) {
	t.Parallel()

	src := `{"src/main.ts":{"file":"main.abc.js","isEntry":true}}`
	a := computeAppVersion(src)
	b := computeAppVersion(src)
	if a != b {
		t.Fatalf("computeAppVersion not deterministic: %q vs %q", a, b)
	}
	if a == "" {
		t.Fatalf("computeAppVersion returned empty for non-empty input")
	}
	if len(a) != 16 {
		t.Errorf("digest length = %d, want 16 hex chars", len(a))
	}
}

// TestComputeAppVersion_changesWithInput pins the drift signal: any
// manifest change must alter the digest, otherwise the client poller
// can never see a deploy.
func TestComputeAppVersion_changesWithInput(t *testing.T) {
	t.Parallel()

	a := computeAppVersion(`{"src/main.ts":{"file":"main.abc.js"}}`)
	b := computeAppVersion(`{"src/main.ts":{"file":"main.def.js"}}`)
	if a == b {
		t.Fatalf("digest should differ for different manifests, got %q", a)
	}
}

// TestComputeAppVersion_emptyInput documents the no-manifest path: an
// empty manifest yields no version, and the endpoint will 404 rather
// than serve a hash that no client can ever match.
func TestComputeAppVersion_emptyInput(t *testing.T) {
	t.Parallel()

	if got := computeAppVersion(""); got != "" {
		t.Fatalf("computeAppVersion(\"\") = %q, want empty", got)
	}
}

// TestServeVersion_okResponse exercises the happy path: GET against
// the canonical SvelteKit-shape endpoint returns 200 with the
// {"version":"<hash>"} body and no-cache headers.
func TestServeVersion_okResponse(t *testing.T) {
	t.Parallel()

	manifest := `{"src/main.ts":{"file":"main.abc.js","isEntry":true}}`
	srv := newTestServerWithManifest(t, manifest)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_app/version.json", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Version != computeAppVersion(manifest) {
		t.Errorf("body.version = %q, want %q", body.Version, computeAppVersion(manifest))
	}
}

// TestServeVersion_notFoundWithoutManifest covers the bootstrap path:
// when no Vite manifest is supplied the build version is unknown, so
// the endpoint reports 404 instead of serving a meaningless hash.
func TestServeVersion_notFoundWithoutManifest(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithManifest(t, "")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_app/version.json", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// TestServeVersion_methodNotAllowed pins the read-only contract: a
// POST against the endpoint must respond 405 with an Allow header
// listing the supported methods.
func TestServeVersion_methodNotAllowed(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithManifest(t, `{"src/main.ts":{"file":"x.js"}}`)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/_app/version.json", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if got := rec.Header().Get("Allow"); !strings.Contains(got, "GET") {
		t.Errorf("Allow = %q, want to include GET", got)
	}
}

// TestServeVersion_headRequest is a smoke for HEAD support — load
// balancers and uptime probes default to HEAD, so a 200 with no body
// is the right answer rather than 405.
func TestServeVersion_headRequest(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithManifest(t, `{"src/main.ts":{"file":"x.js"}}`)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/_app/version.json", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("HEAD body length = %d, want 0", rec.Body.Len())
	}
}

func newTestServerWithManifest(t *testing.T, manifest string) *Server {
	t.Helper()
	srv, err := New(Config{
		Routes: []router.Route{{
			Pattern:  "/",
			Segments: []router.Segment{},
			Page:     staticPage("hello"),
		}},
		Matchers:     params.DefaultMatchers(),
		Shell:        testShell,
		Logger:       quietLogger(),
		ViteManifest: manifest,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return srv
}

// TestServeVersion_payloadCarriesAppVersion exercises the hydration
// path end-to-end: the SSR script tag must carry both appVersion
// and the resolved versionPoll so the client poller boots without
// fetching auxiliary config. Asserting through the rendered HTML
// catches regressions where the field is added to the struct but
// dropped on JSON encode (omitempty / wire shape drift).
func TestServeVersion_payloadCarriesAppVersion(t *testing.T) {
	t.Parallel()

	manifest := `{"src/main.ts":{"file":"main.abc.js","isEntry":true}}`
	srv, err := New(Config{
		Routes: []router.Route{{
			Pattern:  "/",
			Segments: []router.Segment{},
			Page:     staticPage("hello"),
		}},
		Matchers:     params.DefaultMatchers(),
		Shell:        testShell,
		Logger:       quietLogger(),
		ViteManifest: manifest,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	const tag = `<script id="sveltego-data" type="application/json">`
	i := strings.Index(body, tag)
	if i < 0 {
		t.Fatalf("payload tag missing in body=%s", body)
	}
	raw := body[i+len(tag):]
	end := strings.Index(raw, "</script>")
	if end < 0 {
		t.Fatalf("payload tag unterminated")
	}
	var p struct {
		AppVersion  string `json:"appVersion"`
		VersionPoll struct {
			IntervalMS int64 `json:"intervalMs"`
			Disabled   bool  `json:"disabled"`
		} `json:"versionPoll"`
	}
	if err := json.Unmarshal([]byte(raw[:end]), &p); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if p.AppVersion != computeAppVersion(manifest) {
		t.Errorf("appVersion = %q, want %q", p.AppVersion, computeAppVersion(manifest))
	}
	wantInterval := kit.DefaultVersionPollInterval.Milliseconds()
	if p.VersionPoll.IntervalMS != wantInterval {
		t.Errorf("intervalMs = %d, want %d", p.VersionPoll.IntervalMS, wantInterval)
	}
}

// TestServeVersion_payloadOmitsWhenManifestEmpty pins the no-manifest
// case: with no Vite manifest the SSR payload must omit both
// appVersion and versionPoll so the client never starts a poller
// against a 404 endpoint.
func TestServeVersion_payloadOmitsWhenManifestEmpty(t *testing.T) {
	t.Parallel()

	srv, err := New(Config{
		Routes: []router.Route{{
			Pattern:  "/",
			Segments: []router.Segment{},
			Page:     staticPage("hello"),
		}},
		Matchers: params.DefaultMatchers(),
		Shell:    testShell,
		Logger:   quietLogger(),
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	if strings.Contains(body, `"appVersion"`) {
		t.Errorf("payload should omit appVersion when manifest is empty; body=%s", body)
	}
	if strings.Contains(body, `"versionPoll"`) {
		t.Errorf("payload should omit versionPoll when manifest is empty; body=%s", body)
	}
}
