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

// TestGenerateManifest_SvelteMode_SSRRender verifies Phase 6 (#428):
// when a Svelte-mode route is in SSRRenderRoutes, the manifest emits
// the bridge adapter, wires Page to it, and pulls in the runtime
// /svelte/server import.
func TestGenerateManifest_SvelteMode_SSRRender(t *testing.T) {
	t.Parallel()
	scan := scanFixture(t, "simple")
	pattern := scan.Routes[0].Pattern
	pkgPath := scan.Routes[0].PackagePath

	routeOpts := map[string]kit.PageOptions{
		pattern: {SSR: true, CSR: true, CSRF: true, TrailingSlash: kit.TrailingSlashNever, Templates: kit.TemplatesSvelte},
	}
	out, err := GenerateManifest(scan, ManifestOptions{
		PackageName:  "gen",
		ModulePath:   "myapp",
		GenRoot:      ".gen",
		RouteOptions: routeOpts,
		ClientKeys: map[string]string{
			pkgPath: "routes/_page",
		},
		SSRRenderRoutes: map[string]string{
			pattern: "routes",
		},
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	s := string(out)
	if !bytes.Contains(out, []byte("server \"github.com/binsarjr/sveltego/packages/sveltego/runtime/svelte/server\"")) {
		t.Errorf("expected runtime/svelte/server import:\n%s", s)
	}
	if !bytes.Contains(out, []byte(".RenderSSR(&payload, data, pageState)")) {
		t.Errorf("expected RenderSSR call inside bridge with pageState:\n%s", s)
	}
	if !bytes.Contains(out, []byte("Page:")) || !bytes.Contains(out, []byte("render__page_routes,")) {
		t.Errorf("expected Page wired for SSR-mode Svelte route:\n%s", s)
	}
	if !bytes.Contains(out, []byte("payload.Body()")) {
		t.Errorf("expected bridge to copy payload.Body() into the writer:\n%s", s)
	}
}

// TestGenerateManifest_SvelteMode_SSRLayout verifies #456: when a
// layout package is in SSRRenderLayouts, the manifest swaps its
// `render__layout__<alias>` adapter from the legacy
// `Layout{}.Render(*render.Writer, ...)` call to the children-callback
// payload bridge that dispatches `<alias>.RenderLayoutSSR`.
func TestGenerateManifest_SvelteMode_SSRLayout(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite := func(path, body string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("routes/_layout.svelte", "{@render children()}")
	mustWrite("routes/_page.svelte", "<h1>home</h1>")

	scan, err := routescan.Scan(routescan.ScanInput{RoutesDir: filepath.Join(root, "routes")})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	pattern := scan.Routes[0].Pattern
	pkgPath := scan.Routes[0].PackagePath
	routeOpts := map[string]kit.PageOptions{
		pattern: {SSR: true, CSR: true, CSRF: true, TrailingSlash: kit.TrailingSlashNever, Templates: kit.TemplatesSvelte},
	}
	out, err := GenerateManifest(scan, ManifestOptions{
		PackageName:  "gen",
		ModulePath:   "myapp",
		GenRoot:      ".gen",
		RouteOptions: routeOpts,
		SSRRenderRoutes: map[string]string{
			pattern: "routes",
		},
		SSRRenderLayouts: map[string]struct{}{
			pkgPath: {},
		},
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	s := string(out)
	if !bytes.Contains(out, []byte(".RenderLayoutSSR(&payload, data, inner, pageState)")) {
		t.Errorf("expected RenderLayoutSSR call inside layout bridge with pageState:\n%s", s)
	}
	if bytes.Contains(out, []byte(".Layout{}.Render(w, ctx, typed, children)")) {
		t.Errorf("legacy Layout{}.Render call should not appear when layout is SSR-bridged:\n%s", s)
	}
	if !bytes.Contains(out, []byte("inner := func(p *server.Payload) {")) {
		t.Errorf("expected children-bridge closure:\n%s", s)
	}
}

// TestGenerateManifest_SvelteMode_SSRError verifies #412: when an
// error-boundary package is in SSRRenderErrors, the manifest swaps its
// `renderError__<alias>` adapter from the legacy
// `<alias>.ErrorPage{}.Render(...)` call to the payload bridge that
// dispatches `<alias>.RenderErrorSSR(&payload, safe)`.
func TestGenerateManifest_SvelteMode_SSRError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite := func(path, body string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("routes/_error.svelte", "<h1>boom</h1>")
	mustWrite("routes/_page.svelte", "<h1>home</h1>")

	scan, err := routescan.Scan(routescan.ScanInput{RoutesDir: filepath.Join(root, "routes")})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	pattern := scan.Routes[0].Pattern
	errorPkg := scan.Routes[0].ErrorBoundaryPackagePath
	if errorPkg == "" {
		t.Fatalf("expected ErrorBoundaryPackagePath to be set")
	}
	routeOpts := map[string]kit.PageOptions{
		pattern: {SSR: true, CSR: true, CSRF: true, TrailingSlash: kit.TrailingSlashNever, Templates: kit.TemplatesSvelte},
	}
	out, err := GenerateManifest(scan, ManifestOptions{
		PackageName:  "gen",
		ModulePath:   "myapp",
		GenRoot:      ".gen",
		RouteOptions: routeOpts,
		SSRRenderRoutes: map[string]string{
			pattern: "routes",
		},
		SSRRenderErrors: map[string]struct{}{
			errorPkg: {},
		},
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	s := string(out)
	if !bytes.Contains(out, []byte(".RenderErrorSSR(&payload, safe, pageState)")) {
		t.Errorf("expected RenderErrorSSR call inside error bridge with pageState:\n%s", s)
	}
	if bytes.Contains(out, []byte(".ErrorPage{}.Render(w, ctx, safe)")) {
		t.Errorf("legacy ErrorPage{}.Render call should not appear when boundary is SSR-bridged:\n%s", s)
	}
	if !bytes.Contains(out, []byte("var payload server.Payload")) {
		t.Errorf("expected payload allocation in error bridge:\n%s", s)
	}
	if !bytes.Contains(out, []byte("w.WriteString(payload.Body())")) {
		t.Errorf("expected payload.Body() copy in error bridge:\n%s", s)
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
