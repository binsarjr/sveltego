package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/render"
	"github.com/binsarjr/sveltego/runtime/router"
)

func renderOK(text string) router.PageHandler {
	return func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
		w.WriteString("<main>" + text + "</main>")
		return nil
	}
}

func TestPrerender_StaticRoutes_WritesHTMLAndManifest(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cfg := Config{
		Routes: []router.Route{
			{
				Pattern:  "/",
				Segments: []router.Segment{},
				Page:     renderOK("home"),
				Options:  kit.PageOptions{Prerender: true, SSR: true, CSR: true, CSRF: true},
			},
			{
				Pattern: "/about",
				Segments: []router.Segment{
					{Kind: router.SegmentStatic, Value: "about"},
				},
				Page:    renderOK("about"),
				Options: kit.PageOptions{Prerender: true, SSR: true, CSR: true, CSRF: true},
			},
			{
				Pattern: "/dynamic/[id]",
				Segments: []router.Segment{
					{Kind: router.SegmentStatic, Value: "dynamic"},
					{Kind: router.SegmentParam, Name: "id"},
				},
				Page:    renderOK("dyn"),
				Options: kit.PageOptions{Prerender: false, SSR: true, CSR: true, CSRF: true},
			},
		},
		Shell: testShell,
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := s.Prerender(context.Background(), tmp, PrerenderOptions{})
	if err != nil {
		t.Fatalf("Prerender: %v", err)
	}
	if got, want := len(res.Entries), 2; got != want {
		t.Fatalf("entries = %d, want %d (%+v)", got, want, res.Entries)
	}
	homeFile := filepath.Join(tmp, "static", "_prerendered", "index", "index.html")
	if data, err := os.ReadFile(homeFile); err != nil {
		t.Fatalf("read %s: %v", homeFile, err)
	} else if !strings.Contains(string(data), "<main>home</main>") {
		t.Errorf("home file missing rendered body: %q", data)
	}
	aboutFile := filepath.Join(tmp, "static", "_prerendered", "about", "index.html")
	if _, err := os.Stat(aboutFile); err != nil {
		t.Fatalf("expected %s: %v", aboutFile, err)
	}
	manifestPath := filepath.Join(tmp, "static", "_prerendered", "manifest.json")
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m struct {
		Entries []PrerenderedEntry `json:"entries"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if got, want := len(m.Entries), 2; got != want {
		t.Errorf("manifest entries = %d, want %d", got, want)
	}
}

func TestPrerender_Auto_SkipsRoutesWithLoad(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	loader := func(_ *kit.LoadCtx) (any, error) { return nil, nil }
	cfg := Config{
		Routes: []router.Route{
			{
				Pattern:  "/auto-static",
				Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "auto-static"}},
				Page:     renderOK("static"),
				Options:  kit.PageOptions{PrerenderAuto: true, Prerender: true, SSR: true, CSR: true, CSRF: true},
			},
			{
				Pattern:  "/auto-load",
				Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "auto-load"}},
				Page:     renderOK("load"),
				Load:     loader,
				Options:  kit.PageOptions{PrerenderAuto: true, Prerender: true, SSR: true, CSR: true, CSRF: true},
			},
		},
		Shell: testShell,
	}
	s, _ := New(cfg)
	res, err := s.Prerender(context.Background(), tmp, PrerenderOptions{})
	if err != nil {
		t.Fatalf("Prerender: %v", err)
	}
	if len(res.Entries) != 1 || res.Entries[0].Path != "/auto-static" {
		t.Fatalf("auto entries = %+v, want only /auto-static", res.Entries)
	}
}

func TestPrerender_DynamicEntries(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cfg := Config{
		Routes: []router.Route{
			{
				Pattern: "/post/[slug]",
				Segments: []router.Segment{
					{Kind: router.SegmentStatic, Value: "post"},
					{Kind: router.SegmentParam, Name: "slug"},
				},
				Page:    renderOK("post"),
				Options: kit.PageOptions{Prerender: true, SSR: true, CSR: true, CSRF: true},
			},
		},
		Shell: testShell,
	}
	s, _ := New(cfg)
	res, err := s.Prerender(context.Background(), tmp, PrerenderOptions{
		Entries: map[string][]map[string]string{
			"/post/[slug]": {
				{"slug": "hello"},
				{"slug": "world"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Prerender: %v", err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("dynamic entries = %d, want 2", len(res.Entries))
	}
	if _, err := os.Stat(filepath.Join(tmp, "static", "_prerendered", "post", "hello", "index.html")); err != nil {
		t.Errorf("hello file missing: %v", err)
	}
}

func TestPrerender_ErrorAggregation(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	failPage := func(_ *render.Writer, _ *kit.RenderCtx, _ any) error {
		return kit.SafeError{Code: http.StatusInternalServerError, Message: "boom"}
	}
	cfg := Config{
		Routes: []router.Route{
			{Pattern: "/a", Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "a"}}, Page: failPage, Options: kit.PageOptions{Prerender: true, SSR: true, CSR: true, CSRF: true}},
			{Pattern: "/b", Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "b"}}, Page: failPage, Options: kit.PageOptions{Prerender: true, SSR: true, CSR: true, CSRF: true}},
			{Pattern: "/c", Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "c"}}, Page: renderOK("c"), Options: kit.PageOptions{Prerender: true, SSR: true, CSR: true, CSRF: true}},
		},
		Shell: testShell,
	}
	s, _ := New(cfg)
	res, err := s.Prerender(context.Background(), tmp, PrerenderOptions{Tolerate: 0})
	if err == nil {
		t.Fatalf("expected aggregated error, got nil")
	}
	var pe *PrerenderErrors
	if !errors.As(err, &pe) {
		t.Fatalf("error type %T, want *PrerenderErrors", err)
	}
	if len(pe.Errors) != 2 {
		t.Errorf("collected %d errors, want 2", len(pe.Errors))
	}
	if len(res.Entries) != 1 {
		t.Errorf("entries = %d, want 1", len(res.Entries))
	}

	// Tolerate -1 should not error.
	res2, err2 := s.Prerender(context.Background(), t.TempDir(), PrerenderOptions{Tolerate: -1})
	if err2 != nil {
		t.Fatalf("tolerate=-1 returned error: %v", err2)
	}
	if len(res2.Errors) != 2 {
		t.Errorf("tolerate=-1 errors = %d, want 2", len(res2.Errors))
	}
}

func TestPrerender_ErrorReportPath(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cfg := Config{
		Routes: []router.Route{
			{
				Pattern:  "/x",
				Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "x"}},
				Page: func(_ *render.Writer, _ *kit.RenderCtx, _ any) error {
					return kit.SafeError{Code: 404, Message: "nope"}
				},
				Options: kit.PageOptions{Prerender: true, SSR: true, CSR: true, CSRF: true},
			},
		},
		Shell: testShell,
	}
	s, _ := New(cfg)
	report := filepath.Join(tmp, "report.json")
	_, _ = s.Prerender(context.Background(), tmp, PrerenderOptions{ReportPath: report, Tolerate: -1})
	if _, err := os.Stat(report); err != nil {
		t.Fatalf("expected report file: %v", err)
	}
}

func TestServePrerendered_StaticHit(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cfg := Config{
		Routes: []router.Route{
			{
				Pattern:  "/cached",
				Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "cached"}},
				Page:     renderOK("cached"),
				Options:  kit.PageOptions{Prerender: true, SSR: true, CSR: true, CSRF: true},
			},
		},
		Shell: testShell,
	}
	s, _ := New(cfg)
	if _, err := s.Prerender(context.Background(), tmp, PrerenderOptions{}); err != nil {
		t.Fatalf("Prerender: %v", err)
	}
	table, err := LoadPrerenderManifest(tmp, "")
	if err != nil {
		t.Fatalf("LoadPrerenderManifest: %v", err)
	}
	if table == nil {
		t.Fatal("expected manifest, got nil")
	}

	cfg2 := cfg
	cfg2.Prerender = table
	s2, err := New(cfg2)
	if err != nil {
		t.Fatalf("New(prerender): %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/cached", nil)
	s2.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("X-Sveltego-Prerendered"); got != "1" {
		t.Errorf("X-Sveltego-Prerendered = %q, want 1", got)
	}
	if !strings.Contains(rec.Body.String(), "<main>cached</main>") {
		t.Errorf("body missing rendered html: %q", rec.Body.String())
	}
}

func TestServePrerendered_ProtectedDeniedFallsThrough(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cfg := Config{
		Routes: []router.Route{
			{
				Pattern:  "/dash",
				Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "dash"}},
				Page:     renderOK("live"),
				Options:  kit.PageOptions{Prerender: true, PrerenderProtected: true, SSR: true, CSR: true, CSRF: true},
			},
		},
		Shell: testShell,
	}
	s, _ := New(cfg)
	if _, err := s.Prerender(context.Background(), tmp, PrerenderOptions{}); err != nil {
		t.Fatalf("Prerender: %v", err)
	}
	table, _ := LoadPrerenderManifest(tmp, "")

	// Default (deny-all) gate: should NOT serve the prerendered HTML; falls
	// through to live SSR which renders <main>live</main>.
	cfg2 := cfg
	cfg2.Prerender = table
	s2, _ := New(cfg2)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/dash", nil)
	s2.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Sveltego-Prerendered"); got != "" {
		t.Errorf("expected fallthrough, got X-Sveltego-Prerendered=%q", got)
	}
	if !strings.Contains(rec.Body.String(), "<main>live</main>") {
		t.Errorf("expected live SSR body, got %q", rec.Body.String())
	}

	// Allow-all gate: should serve prerendered HTML.
	cfg3 := cfg2
	cfg3.PrerenderAuth = kit.PrerenderAuthGateFunc(func(*http.Request) bool { return true })
	s3, _ := New(cfg3)
	rec3 := httptest.NewRecorder()
	s3.ServeHTTP(rec3, req)
	if got := rec3.Header().Get("X-Sveltego-Prerendered"); got != "1" {
		t.Errorf("allow gate: X-Sveltego-Prerendered = %q, want 1", got)
	}
}

func TestPlanPrerenderJobs_DynamicWithoutEntriesSkipped(t *testing.T) {
	t.Parallel()
	routes := []router.Route{
		{
			Pattern: "/u/[id]",
			Segments: []router.Segment{
				{Kind: router.SegmentStatic, Value: "u"},
				{Kind: router.SegmentParam, Name: "id"},
			},
			Page:    renderOK("u"),
			Options: kit.PageOptions{Prerender: true},
		},
	}
	jobs, err := planPrerenderJobs(routes, nil)
	if err != nil {
		t.Fatalf("planPrerenderJobs: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("dynamic-without-entries should be empty, got %+v", jobs)
	}
}

func TestSubstituteParams(t *testing.T) {
	t.Parallel()
	got, err := substituteParams("/a/[id]/b/[slug=name]", map[string]string{"id": "42", "slug": "hi"})
	if err != nil {
		t.Fatalf("substituteParams: %v", err)
	}
	if got != "/a/42/b/hi" {
		t.Errorf("got %q", got)
	}
	if _, err := substituteParams("/a/[id]", map[string]string{}); err == nil {
		t.Errorf("expected missing-param error")
	}
}

func TestDenyAllPrerenderAuth(t *testing.T) {
	t.Parallel()
	if kit.DenyAllPrerenderAuth.Allow(httptest.NewRequest("GET", "/", nil)) {
		t.Error("DenyAllPrerenderAuth allowed a request")
	}
	var nilGate kit.PrerenderAuthGateFunc
	if nilGate.Allow(httptest.NewRequest("GET", "/", nil)) {
		t.Error("nil gate allowed")
	}
}
