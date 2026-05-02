package codegen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/routescan"
)

// TestPlanSSRPartitionsAnnotated exercises the Phase 8 (#430) split:
// annotated routes land on the fallback list and skip the transpile
// plan even when they have a sibling _page.server.go that would
// otherwise qualify.
func TestPlanSSRPartitionsAnnotated(t *testing.T) {
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
	mustWrite("src/routes/_page.svelte", "<h1>home</h1>")
	mustWrite("src/routes/_page.server.go", `package routes

type PageData struct{ Name string `+"`json:\"name\"`"+` }

func Load(ctx any) (PageData, error) { return PageData{}, nil }
`)
	mustWrite("src/routes/posts/[id]/_page.svelte", `<!-- sveltego:ssr-fallback -->
<h1>post</h1>`)
	mustWrite("src/routes/posts/[id]/_page.server.go", `package id

type PageData struct{ Title string `+"`json:\"title\"`"+` }

func Load(ctx any) (PageData, error) { return PageData{}, nil }
`)

	scan, err := routescan.Scan(routescan.ScanInput{RoutesDir: filepath.Join(root, "src", "routes")})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	routeOptions := map[string]kit.PageOptions{
		"/":           mkSvelteOpts(),
		"/posts/[id]": mkSvelteOpts(),
	}

	plans, fallback := planSSR(scan, routeOptions)
	if got, want := len(fallback), 1; got != want {
		t.Fatalf("fallback count = %d, want %d", got, want)
	}
	if fallback[0].Pattern != "/posts/[id]" {
		t.Fatalf("fallback[0].Pattern = %q, want /posts/[id]", fallback[0].Pattern)
	}
	for _, p := range plans {
		if p.route.Pattern == "/posts/[id]" {
			t.Fatalf("annotated route should not appear in transpile plan")
		}
	}
	// The root route has no SSRFallback annotation; it should be in the
	// transpile plan since it has a non-empty PageData.
	foundRoot := false
	for _, p := range plans {
		if p.route.Pattern == "/" {
			foundRoot = true
		}
	}
	if !foundRoot {
		t.Fatalf("root route should appear in transpile plan")
	}
}

func mkSvelteOpts() kit.PageOptions {
	o := kit.DefaultPageOptions()
	o.Templates = kit.TemplatesSvelte
	return o
}

// TestPlanSSRLayouts_DedupesAndSynthesisesShape verifies #456: the
// layout planner enumerates every layout dir reachable from a Svelte-
// SSR-eligible page route, deduplicates layouts shared between
// sibling routes, and synthesises an empty-LayoutData shape when no
// `_layout.server.go` is present so the wire helper still compiles
// against `usersrc.LayoutData`.
func TestPlanSSRLayouts_DedupesAndSynthesisesShape(t *testing.T) {
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
	mustWrite("src/routes/_layout.svelte", "{@render children()}")
	mustWrite("src/routes/foo/_layout.svelte", "<header>foo</header>{@render children()}")
	mustWrite("src/routes/foo/_page.svelte", "<h1>foo</h1>")
	mustWrite("src/routes/foo/_page.server.go", `package foo

type PageData struct{ Name string `+"`json:\"name\"`"+` }

func Load(ctx any) (PageData, error) { return PageData{}, nil }
`)
	mustWrite("src/routes/foo/bar/_page.svelte", "<h1>bar</h1>")
	mustWrite("src/routes/foo/bar/_page.server.go", `package bar

type PageData struct{ Slug string `+"`json:\"slug\"`"+` }

func Load(ctx any) (PageData, error) { return PageData{}, nil }
`)

	scan, err := routescan.Scan(routescan.ScanInput{RoutesDir: filepath.Join(root, "src", "routes")})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	routeOptions := map[string]kit.PageOptions{
		"/foo":     mkSvelteOpts(),
		"/foo/bar": mkSvelteOpts(),
	}
	pagePlans, _ := planSSR(scan, routeOptions)
	layoutPlans := planSSRLayouts(scan, pagePlans)

	// /foo and /foo/bar share two layouts (root + /foo); planSSRLayouts
	// dedupes by package path.
	if got, want := len(layoutPlans), 2; got != want {
		t.Fatalf("layoutPlans count = %d, want %d (paths: %v)", got, want, layoutPlanPaths(layoutPlans))
	}
	for _, lp := range layoutPlans {
		if lp.shape.RootType != "LayoutData" {
			t.Fatalf("layout %s: shape RootType = %q, want LayoutData", lp.pkgPath, lp.shape.RootType)
		}
		if _, ok := lp.shape.Types["LayoutData"]; !ok {
			t.Fatalf("layout %s: shape.Types missing LayoutData entry", lp.pkgPath)
		}
		if lp.serverFile != "" {
			t.Fatalf("layout %s: unexpected serverFile %q", lp.pkgPath, lp.serverFile)
		}
	}
}

func layoutPlanPaths(ps []layoutPlan) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.pkgPath)
	}
	return out
}
