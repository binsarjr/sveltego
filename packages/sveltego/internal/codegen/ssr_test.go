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

// TestPlanSSRErrors_DedupesAndSynthesisesShape verifies #412: the
// error planner enumerates every error-boundary dir reachable from a
// Svelte-SSR-eligible page route, deduplicates boundaries shared
// between sibling routes, and binds the synthetic ErrorData shape so
// the Lowerer can rewrite `data.code` → `data.Code` etc. against
// kit.SafeError.
func TestPlanSSRErrors_DedupesAndSynthesisesShape(t *testing.T) {
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
	mustWrite("src/routes/_error.svelte", "<h1>root error</h1>")
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
	errorPlans := planSSRErrors(scan, pagePlans)

	// Both /foo and /foo/bar resolve to the root _error.svelte boundary;
	// planSSRErrors dedupes by package path.
	if got, want := len(errorPlans), 1; got != want {
		t.Fatalf("errorPlans count = %d, want %d", got, want)
	}
	ep := errorPlans[0]
	if ep.shape.RootType != "ErrorData" {
		t.Fatalf("error shape RootType = %q, want ErrorData", ep.shape.RootType)
	}
	root_t, ok := ep.shape.Types["ErrorData"]
	if !ok {
		t.Fatalf("error shape.Types missing ErrorData entry")
	}
	wantFields := map[string]string{"code": "Code", "message": "Message", "id": "ID"}
	for jsonName, goName := range wantFields {
		f, found := root_t.Lookup(jsonName)
		if !found {
			t.Fatalf("ErrorData missing field %q", jsonName)
		}
		if f.GoName != goName {
			t.Fatalf("ErrorData[%q].GoName = %q, want %q", jsonName, f.GoName, goName)
		}
	}
}

// TestPlanSSR_PrerenderRouteFromTypedShape verifies #467: a Svelte+
// Prerender route with a sibling _page.server.go declaring a non-empty
// PageData lands in the transpile plan with the user shape, not the
// synthetic one.
func TestPlanSSR_PrerenderRouteFromTypedShape(t *testing.T) {
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
	mustWrite("src/routes/about/_page.svelte", "<h1>about</h1>")
	mustWrite("src/routes/about/_page.server.go", `package about

type PageData struct{ Title string `+"`json:\"title\"`"+` }

func Load(ctx any) (PageData, error) { return PageData{Title: "About"}, nil }
`)

	scan, err := routescan.Scan(routescan.ScanInput{RoutesDir: filepath.Join(root, "src", "routes")})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	opts := mkSvelteOpts()
	opts.Prerender = true
	routeOptions := map[string]kit.PageOptions{"/about": opts}

	plans, _ := planSSR(scan, routeOptions)
	if len(plans) != 1 {
		t.Fatalf("plans count = %d, want 1", len(plans))
	}
	if plans[0].synthetic {
		t.Fatalf("plan synthetic = true, want false (server file present)")
	}
	if plans[0].shape.RootType != "PageData" {
		t.Fatalf("shape.RootType = %q, want PageData", plans[0].shape.RootType)
	}
	if got := len(plans[0].shape.Types["PageData"].Fields); got == 0 {
		t.Fatalf("shape PageData fields = 0, want non-zero (user-authored shape)")
	}
}

// TestPlanSSR_PrerenderRouteSyntheticShape verifies #467: a Svelte+
// Prerender route without a sibling _page.server.go still lands in the
// transpile plan, marked synthetic so runSSRTranspile drops a
// page_synthetic.gen.go alongside the transpiled output.
func TestPlanSSR_PrerenderRouteSyntheticShape(t *testing.T) {
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
	mustWrite("src/routes/about/_page.svelte", "<h1>about</h1>")

	scan, err := routescan.Scan(routescan.ScanInput{RoutesDir: filepath.Join(root, "src", "routes")})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	opts := mkSvelteOpts()
	opts.Prerender = true
	routeOptions := map[string]kit.PageOptions{"/about": opts}

	plans, _ := planSSR(scan, routeOptions)
	if len(plans) != 1 {
		t.Fatalf("plans count = %d, want 1 (Prerender route gets synthetic shape when no _page.server.go)", len(plans))
	}
	if !plans[0].synthetic {
		t.Fatalf("plan synthetic = false, want true (no server file)")
	}
	if plans[0].shape.RootType != "PageData" {
		t.Fatalf("shape.RootType = %q, want PageData", plans[0].shape.RootType)
	}
	if got := len(plans[0].shape.Types["PageData"].Fields); got != 0 {
		t.Fatalf("synthetic shape PageData fields = %d, want 0", got)
	}
}

// TestPlanSSR_NonPrerenderNoServerFileSkipped re-asserts the legacy
// invariant kept by #467: live SSR (non-Prerender) routes still need a
// _page.server.go with a non-empty PageData. The synthetic-shape path
// is opt-in via Prerender:true only.
func TestPlanSSR_NonPrerenderNoServerFileSkipped(t *testing.T) {
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

	scan, err := routescan.Scan(routescan.ScanInput{RoutesDir: filepath.Join(root, "src", "routes")})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	routeOptions := map[string]kit.PageOptions{"/": mkSvelteOpts()}

	plans, _ := planSSR(scan, routeOptions)
	if len(plans) != 0 {
		t.Fatalf("plans count = %d, want 0 (live SSR still needs _page.server.go)", len(plans))
	}
}

// TestPlanSSRErrors_SkipsNonSvelteRoutes verifies that the error
// planner walks only error boundaries reachable from a Svelte-SSR-
// eligible page; routes that are pure-Svelte but lack a server-file
// (so they fall out of planSSR) carry no error transpile.
func TestPlanSSRErrors_SkipsNonSvelteRoutes(t *testing.T) {
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
	mustWrite("src/routes/_error.svelte", "<h1>root error</h1>")
	mustWrite("src/routes/_page.svelte", "<h1>home</h1>")
	// no _page.server.go; planSSR drops the route, so planSSRErrors
	// should also drop the boundary.

	scan, err := routescan.Scan(routescan.ScanInput{RoutesDir: filepath.Join(root, "src", "routes")})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	routeOptions := map[string]kit.PageOptions{"/": mkSvelteOpts()}
	pagePlans, _ := planSSR(scan, routeOptions)
	if len(pagePlans) != 0 {
		t.Fatalf("pagePlans count = %d, want 0 (no server file → planSSR drops)", len(pagePlans))
	}
	errorPlans := planSSRErrors(scan, pagePlans)
	if len(errorPlans) != 0 {
		t.Fatalf("errorPlans count = %d, want 0 (no eligible page route)", len(errorPlans))
	}
}
