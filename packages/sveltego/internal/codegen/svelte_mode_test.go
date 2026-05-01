package codegen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/routescan"
)

// TestGenerateManifest_SvelteMode_NoRenderAdapter verifies that a route
// with kit.PageOptions{Templates: "svelte"} skips the Mustache-Go
// render adapter emission. The manifest still includes the route
// entry, the wire-imported Load, the ClientKey, and the Options
// literal — but no `Page: render__...` reference and no
// `func render__...` definition. Phase 3 of RFC #379.
func TestGenerateManifest_SvelteMode_NoRenderAdapter(t *testing.T) {
	t.Parallel()
	scan := scanFixture(t, "simple")
	routeOpts := map[string]kit.PageOptions{
		"/": {SSR: true, CSR: true, CSRF: true, TrailingSlash: kit.TrailingSlashNever, Templates: kit.TemplatesSvelte},
	}
	out, err := GenerateManifest(scan, ManifestOptions{
		PackageName:  "gen",
		ModulePath:   "myapp",
		GenRoot:      ".gen",
		RouteOptions: routeOpts,
		ClientKeys: map[string]string{
			scan.Routes[0].PackagePath: "routes/_page",
		},
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	s := string(out)

	if bytes.Contains(out, []byte("func render__")) {
		t.Errorf("Svelte-mode route emitted render adapter:\n%s", s)
	}
	if bytes.Contains(out, []byte("Page: render__")) {
		t.Errorf("Svelte-mode route emitted Page field:\n%s", s)
	}
	if !bytes.Contains(out, []byte("Templates: `svelte`")) {
		t.Errorf("expected Templates: `svelte` in Options literal:\n%s", s)
	}
	if !bytes.Contains(out, []byte("ClientKey: `routes/_page`")) {
		t.Errorf("expected ClientKey wired for Svelte mode:\n%s", s)
	}
}

// TestGenerateManifest_MixedMode confirms that a Mustache-Go route and
// a Svelte route in the same scan emit a render adapter for the
// former but skip it for the latter, exercising the mixed-mode
// codegen pass that ships in Phase 3 (the framework keeps both
// pipelines parallel until Phase 5 (#384) drops Mustache-Go).
func TestGenerateManifest_MixedMode(t *testing.T) {
	t.Parallel()
	scan := scanFixture(t, "nested")
	if len(scan.Routes) < 2 {
		t.Fatalf("need ≥2 routes in nested fixture, got %d", len(scan.Routes))
	}
	// Pick the first two patterns: one stays Mustache-Go, the other
	// flips to Svelte.
	first := scan.Routes[0].Pattern
	second := scan.Routes[1].Pattern
	routeOpts := map[string]kit.PageOptions{
		first:  {SSR: true, CSR: true, CSRF: true, TrailingSlash: kit.TrailingSlashNever, Templates: kit.TemplatesGoMustache},
		second: {SSR: true, CSR: true, CSRF: true, TrailingSlash: kit.TrailingSlashNever, Templates: kit.TemplatesSvelte},
	}
	out, err := GenerateManifest(scan, ManifestOptions{
		PackageName:  "gen",
		ModulePath:   "myapp",
		GenRoot:      ".gen",
		RouteOptions: routeOpts,
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	s := string(out)

	if !bytes.Contains(out, []byte("func render__")) {
		t.Errorf("expected at least one render adapter for the Mustache-Go route:\n%s", s)
	}
	if !bytes.Contains(out, []byte("Templates: `svelte`")) {
		t.Errorf("expected Svelte route to carry Templates literal:\n%s", s)
	}
}

// TestNeedsNodeForSvelteSSG verifies the SSG sidecar trigger fires
// only when at least one route combines Templates: "svelte" with
// Prerender: true.
func TestNeedsNodeForSvelteSSG(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	routesDir := filepath.Join(dir, "routes")
	if err := os.MkdirAll(routesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// One synthetic page so HasPage is true.
	if err := os.WriteFile(filepath.Join(routesDir, "_page.svelte"), []byte("<h1>x</h1>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	scan, err := routescan.Scan(routescan.ScanInput{RoutesDir: routesDir})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	pat := scan.Routes[0].Pattern

	cases := []struct {
		name string
		opts kit.PageOptions
		want bool
	}{
		{"no-svelte", kit.PageOptions{Templates: kit.TemplatesGoMustache, Prerender: true}, false},
		{"svelte-no-prerender", kit.PageOptions{Templates: kit.TemplatesSvelte}, false},
		{"svelte-prerender", kit.PageOptions{Templates: kit.TemplatesSvelte, Prerender: true}, true},
		{"svelte-auto", kit.PageOptions{Templates: kit.TemplatesSvelte, PrerenderAuto: true}, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ro := map[string]kit.PageOptions{pat: tc.opts}
			got := needsNodeForSvelteSSG(scan.Routes, ro)
			if got != tc.want {
				t.Fatalf("got %v, want %v for %+v", got, tc.want, tc.opts)
			}
		})
	}
}
