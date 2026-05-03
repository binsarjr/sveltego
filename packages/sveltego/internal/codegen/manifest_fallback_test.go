package codegen

import (
	"bytes"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/routescan"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// TestGenerateManifest_FallbackAdapter exercises Phase 8 (#430) wiring:
// a Svelte-mode route declared as fallback should produce a
// renderFallback__ adapter, an init() registration, the fallback
// import, and a Page wire on the route entry.
func TestGenerateManifest_FallbackAdapter(t *testing.T) {
	t.Parallel()
	posts := router.Segment{Kind: router.SegmentStatic, Value: "posts"}
	id := router.Segment{Kind: router.SegmentParam, Name: "id"}
	scan := &routescan.ScanResult{
		Routes: []routescan.ScannedRoute{
			{
				Pattern:     "/posts/[id]",
				Segments:    []router.Segment{posts, id},
				Dir:         "/tmp/routes/posts/[id]",
				PackageName: "_id_",
				PackagePath: ".gen/routes/posts/_id_",
				HasPage:     true,
				SSRFallback: true,
			},
		},
	}
	routeOptions := map[string]kit.PageOptions{
		"/posts/[id]": {Templates: kit.TemplatesSvelte, SSR: true},
	}
	out, err := GenerateManifest(scan, ManifestOptions{
		PackageName:  "gen",
		ModulePath:   "myapp",
		GenRoot:      ".gen",
		RouteOptions: routeOptions,
		SSRFallbackRoutes: []SSRFallbackRoute{
			{Pattern: "/posts/[id]", Source: "src/routes/posts/[id]/_page.svelte"},
		},
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"runtime/svelte/fallback",
		"renderFallback__PostsId",
		"fallback.Default()",
		"r.Register(`/posts/[id]`, `src/routes/posts/[id]/_page.svelte`)",
		"renderFallback__PostsId,",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in manifest output:\n%s", want, s)
		}
	}
}

// TestGenerateManifest_FallbackAdapter_CSRFInject covers issue #510:
// when a fallback-annotated route opts into CSRF, the generated
// renderFallback__ adapter must pipe the sidecar's HTML body through
// csrfinject.Rewrite so POST forms gain the hidden _csrf_token input.
// CSRF=false on the route should keep the import and the call out.
func TestGenerateManifest_FallbackAdapter_CSRFInject(t *testing.T) {
	t.Parallel()
	login := router.Segment{Kind: router.SegmentStatic, Value: "login"}
	scan := &routescan.ScanResult{
		Routes: []routescan.ScannedRoute{
			{
				Pattern:     "/login",
				Segments:    []router.Segment{login},
				Dir:         "/tmp/routes/login",
				PackageName: "login",
				PackagePath: ".gen/routes/login",
				HasPage:     true,
				SSRFallback: true,
			},
		},
	}
	routeOptions := map[string]kit.PageOptions{
		"/login": {Templates: kit.TemplatesSvelte, SSR: true, CSRF: true},
	}
	out, err := GenerateManifest(scan, ManifestOptions{
		PackageName:  "gen",
		ModulePath:   "myapp",
		GenRoot:      ".gen",
		RouteOptions: routeOptions,
		SSRFallbackRoutes: []SSRFallbackRoute{
			{Pattern: "/login", Source: "src/routes/login/_page.svelte"},
		},
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"runtime/svelte/csrfinject",
		"csrfinject.Rewrite(resp.Body, ctx.CSRFToken())",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in manifest output:\n%s", want, s)
		}
	}
}

// TestGenerateManifest_FallbackAdapter_NoCSRFNoInject is the negative
// twin: a fallback route with CSRF disabled must not import csrfinject
// nor call Rewrite, because the rewrite cost is wasted when the server
// won't validate the field anyway.
func TestGenerateManifest_FallbackAdapter_NoCSRFNoInject(t *testing.T) {
	t.Parallel()
	login := router.Segment{Kind: router.SegmentStatic, Value: "login"}
	scan := &routescan.ScanResult{
		Routes: []routescan.ScannedRoute{
			{
				Pattern:     "/login",
				Segments:    []router.Segment{login},
				Dir:         "/tmp/routes/login",
				PackageName: "login",
				PackagePath: ".gen/routes/login",
				HasPage:     true,
				SSRFallback: true,
			},
		},
	}
	routeOptions := map[string]kit.PageOptions{
		"/login": {Templates: kit.TemplatesSvelte, SSR: true, CSRF: false},
	}
	out, err := GenerateManifest(scan, ManifestOptions{
		PackageName:  "gen",
		ModulePath:   "myapp",
		GenRoot:      ".gen",
		RouteOptions: routeOptions,
		SSRFallbackRoutes: []SSRFallbackRoute{
			{Pattern: "/login", Source: "src/routes/login/_page.svelte"},
		},
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "csrfinject") {
		t.Errorf("CSRF=false route should not pull csrfinject into the manifest:\n%s", s)
	}
}

// TestGenerateManifest_NoFallbackNoImport ensures the fallback import
// and init() are omitted when no route opted in. We don't want a stale
// dependency dragging into builds that don't need it.
func TestGenerateManifest_NoFallbackNoImport(t *testing.T) {
	t.Parallel()
	scan := scanFixture(t, "simple")
	out, err := GenerateManifest(scan, ManifestOptions{
		PackageName: "gen",
		ModulePath:  "myapp",
		GenRoot:     ".gen",
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if bytes.Contains(out, []byte("runtime/svelte/fallback")) {
		t.Fatalf("manifest should not import fallback when no route opted in:\n%s", out)
	}
	if bytes.Contains(out, []byte("renderFallback__")) {
		t.Fatalf("manifest should not emit fallback adapter when no route opted in")
	}
}
