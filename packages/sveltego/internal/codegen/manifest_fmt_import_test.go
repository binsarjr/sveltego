package codegen

import (
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/routescan"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// TestGenerateManifest_FmtImportGated locks in the gating from #485:
// the `"fmt"` import line lives behind `usesFmt` and only the legacy
// Mustache-Go page adapter (typed-data type-assert error path) emits
// `fmt.Errorf` into the manifest body. The Svelte-mode payload-bridge
// adapters introduced by Phase 6 (#428) and #456 do not. A trivial
// single-route Svelte project must compile to a manifest with NO `fmt`
// import — `goimports` post-pass becomes optional, not a hard
// requirement.
func TestGenerateManifest_FmtImportGated(t *testing.T) {
	t.Parallel()

	t.Run("svelte_single_route_no_fmt", func(t *testing.T) {
		t.Parallel()
		scan := &routescan.ScanResult{
			Routes: []routescan.ScannedRoute{
				{
					Pattern:       "/",
					Dir:           "/tmp/routes",
					PackageName:   "routes",
					PackagePath:   ".gen/routes",
					HasPage:       true,
					HasPageServer: true,
				},
			},
		}
		out, err := GenerateManifest(scan, ManifestOptions{
			PackageName: "gen",
			ModulePath:  "example.com/repro",
			GenRoot:     ".gen",
			RouteOptions: map[string]kit.PageOptions{
				"/": {Templates: kit.TemplatesSvelte, SSR: true, CSR: true},
			},
			SSRRenderRoutes: map[string]string{"/": "usersrc/routes"},
		})
		if err != nil {
			t.Fatalf("GenerateManifest: %v", err)
		}
		s := string(out)
		if strings.Contains(s, `"fmt"`) {
			t.Errorf("manifest imports fmt for a Svelte-only route; output:\n%s", s)
		}
		if strings.Contains(s, "fmt.") {
			t.Errorf("manifest body references fmt.; output:\n%s", s)
		}
	})

	t.Run("svelte_no_ssr_emit_no_fmt", func(t *testing.T) {
		t.Parallel()
		// Same Svelte route but the SSR transpile plan is empty (e.g.
		// PageData has no fields, so planSSR skipped it). The route
		// gets no Page wire and no render__ adapter; the manifest
		// must still leave fmt out.
		scan := &routescan.ScanResult{
			Routes: []routescan.ScannedRoute{
				{
					Pattern:       "/",
					Dir:           "/tmp/routes",
					PackageName:   "routes",
					PackagePath:   ".gen/routes",
					HasPage:       true,
					HasPageServer: true,
				},
			},
		}
		out, err := GenerateManifest(scan, ManifestOptions{
			PackageName: "gen",
			ModulePath:  "example.com/repro",
			GenRoot:     ".gen",
			RouteOptions: map[string]kit.PageOptions{
				"/": {Templates: kit.TemplatesSvelte, SSR: true, CSR: true},
			},
		})
		if err != nil {
			t.Fatalf("GenerateManifest: %v", err)
		}
		if strings.Contains(string(out), `"fmt"`) {
			t.Errorf("manifest imports fmt without an emit site; output:\n%s", out)
		}
	})

	t.Run("mustache_route_keeps_fmt", func(t *testing.T) {
		t.Parallel()
		// Regression check on the legacy path: a non-Svelte route DOES
		// emit `fmt.Errorf` for the typed-data assertion bridge, so the
		// `fmt` import must stay.
		scan := &routescan.ScanResult{
			Routes: []routescan.ScannedRoute{
				{
					Pattern:       "/",
					Dir:           "/tmp/routes",
					PackageName:   "routes",
					PackagePath:   ".gen/routes",
					HasPage:       true,
					HasPageServer: true,
				},
			},
		}
		out, err := GenerateManifest(scan, ManifestOptions{
			PackageName: "gen",
			ModulePath:  "example.com/repro",
			GenRoot:     ".gen",
		})
		if err != nil {
			t.Fatalf("GenerateManifest: %v", err)
		}
		s := string(out)
		if !strings.Contains(s, `"fmt"`) {
			t.Errorf("Mustache-Go route should keep fmt import; output:\n%s", s)
		}
		if !strings.Contains(s, `fmt.Errorf("sveltego: route`) {
			t.Errorf("Mustache-Go route should emit fmt.Errorf for typed-data assert; output:\n%s", s)
		}
	})

	t.Run("svelte_multi_route_no_fmt", func(t *testing.T) {
		t.Parallel()
		// Multi-route Svelte project: every route is the SSR Render
		// emit, so still no fmt anywhere. Confirms the gating scales
		// past a single route.
		about := router.Segment{Kind: router.SegmentStatic, Value: "about"}
		scan := &routescan.ScanResult{
			Routes: []routescan.ScannedRoute{
				{
					Pattern:       "/",
					Dir:           "/tmp/routes",
					PackageName:   "routes",
					PackagePath:   ".gen/routes",
					HasPage:       true,
					HasPageServer: true,
				},
				{
					Pattern:       "/about",
					Segments:      []router.Segment{about},
					Dir:           "/tmp/routes/about",
					PackageName:   "about",
					PackagePath:   ".gen/routes/about",
					HasPage:       true,
					HasPageServer: true,
				},
			},
		}
		out, err := GenerateManifest(scan, ManifestOptions{
			PackageName: "gen",
			ModulePath:  "example.com/repro",
			GenRoot:     ".gen",
			RouteOptions: map[string]kit.PageOptions{
				"/":      {Templates: kit.TemplatesSvelte, SSR: true, CSR: true},
				"/about": {Templates: kit.TemplatesSvelte, SSR: true, CSR: true},
			},
			SSRRenderRoutes: map[string]string{
				"/":      "usersrc/routes",
				"/about": "usersrc/routes/about",
			},
		})
		if err != nil {
			t.Fatalf("GenerateManifest: %v", err)
		}
		if strings.Contains(string(out), `"fmt"`) {
			t.Errorf("multi-route Svelte project should not import fmt; output:\n%s", out)
		}
	})

	t.Run("svelte_layout_no_ssr_marker_no_fmt", func(t *testing.T) {
		t.Parallel()
		// Svelte SSR page + a layout. emitLayoutAdapters always uses
		// the SSR payload-bridge form (#494 collapsed the always-true
		// hasSSR flag), so no fmt.Errorf lands in the emitted body.
		// The `usesFmt` accumulator must agree — otherwise the import
		// is "fmt imported and not used" again.
		scan := &routescan.ScanResult{
			Routes: []routescan.ScannedRoute{
				{
					Pattern:            "/",
					Dir:                "/tmp/routes",
					PackageName:        "routes",
					PackagePath:        ".gen/routes",
					HasPage:            true,
					HasPageServer:      true,
					LayoutPackagePaths: []string{".gen/layouts/_root_"},
					LayoutServerFiles:  []string{""},
				},
			},
		}
		out, err := GenerateManifest(scan, ManifestOptions{
			PackageName: "gen",
			ModulePath:  "example.com/repro",
			GenRoot:     ".gen",
			RouteOptions: map[string]kit.PageOptions{
				"/": {Templates: kit.TemplatesSvelte, SSR: true, CSR: true},
			},
			SSRRenderRoutes: map[string]string{"/": "usersrc/routes"},
			// SSRRenderLayouts intentionally omitted.
		})
		if err != nil {
			t.Fatalf("GenerateManifest: %v", err)
		}
		s := string(out)
		if strings.Contains(s, `"fmt"`) && !strings.Contains(s, "fmt.") {
			t.Errorf("manifest imports fmt but never references it; output:\n%s", s)
		}
	})

	t.Run("svelte_layout_chain_no_fmt", func(t *testing.T) {
		t.Parallel()
		// Layout chain on a Svelte SSR route: layout adapters now use
		// the SSR payload-bridge form unconditionally (#494 collapsed
		// the always-true hasSSR flag), and renderChain__ emit stays
		// fmt-free. Asserts the layout case from the #485 acceptance
		// criteria — fmt only when an emitted layout bridge actually
		// writes fmt.Errorf, which the SSR form never does.
		scan := &routescan.ScanResult{
			Routes: []routescan.ScannedRoute{
				{
					Pattern:            "/",
					Dir:                "/tmp/routes",
					PackageName:        "routes",
					PackagePath:        ".gen/routes",
					HasPage:            true,
					HasPageServer:      true,
					LayoutPackagePaths: []string{".gen/layouts/_root_"},
					LayoutServerFiles:  []string{""},
				},
			},
		}
		out, err := GenerateManifest(scan, ManifestOptions{
			PackageName: "gen",
			ModulePath:  "example.com/repro",
			GenRoot:     ".gen",
			RouteOptions: map[string]kit.PageOptions{
				"/": {Templates: kit.TemplatesSvelte, SSR: true, CSR: true},
			},
			SSRRenderRoutes: map[string]string{"/": "usersrc/routes"},
		})
		if err != nil {
			t.Fatalf("GenerateManifest: %v", err)
		}
		if strings.Contains(string(out), `"fmt"`) {
			t.Errorf("Svelte SSR layout-chain should not import fmt; output:\n%s", out)
		}
	})
}
