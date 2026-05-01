package codegen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/routescan"
)

// TestGenerateManifest_SvelteMode_NoRenderAdapter verifies that a
// pure-Svelte route (the only mode after RFC #379 phase 5) skips the
// legacy render-adapter emission. The manifest still includes the
// route entry, the wire-imported Load, and the ClientKey — but no
// `Page: render__...` reference and no `func render__...` definition.
// Templates: "svelte" is the default, so the Options literal omits the
// field; the runtime treats empty as svelte mode.
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
		t.Errorf("expected Templates: `svelte` literal in manifest:\n%s", s)
	}
	if !bytes.Contains(out, []byte("ClientKey: `routes/_page`")) {
		t.Errorf("expected ClientKey wired for Svelte mode:\n%s", s)
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
		{"no-svelte", kit.PageOptions{Prerender: true}, false},
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
