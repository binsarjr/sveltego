package routescan

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestScanSimple(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "simple", "")
	if got := len(res.Routes); got != 1 {
		t.Fatalf("want 1 route, got %d", got)
	}
	r := res.Routes[0]
	if r.Pattern != "/" {
		t.Fatalf("want pattern /, got %q", r.Pattern)
	}
	if !r.HasPage {
		t.Fatal("expected HasPage")
	}
	if r.PackageName != "routes" {
		t.Fatalf("want package routes, got %q", r.PackageName)
	}
	if r.PackagePath != ".gen/routes" {
		t.Fatalf("want pkg path .gen/routes, got %q", r.PackagePath)
	}
	if len(res.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", res.Diagnostics)
	}
}

func TestScanNested(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "nested", "")
	if len(res.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", res.Diagnostics)
	}
	if len(res.Routes) != 3 {
		t.Fatalf("want 3 routes, got %d: %+v", len(res.Routes), patterns(res.Routes))
	}
	want := []string{"/", "/about", "/posts/[slug]"}
	for i, p := range want {
		if res.Routes[i].Pattern != p {
			t.Fatalf("route %d: want %q, got %q", i, p, res.Routes[i].Pattern)
		}
	}
	slug := res.Routes[2]
	if slug.PackagePath != ".gen/routes/posts/_slug_" {
		t.Fatalf("want .gen/routes/posts/_slug_, got %q", slug.PackagePath)
	}
	if !slug.HasPageServer {
		t.Fatal("expected slug route HasPageServer")
	}
}

func TestScanGroup(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "groups", "")
	if len(res.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", res.Diagnostics)
	}
	if len(res.Routes) != 1 {
		t.Fatalf("want 1 route, got %d", len(res.Routes))
	}
	r := res.Routes[0]
	if r.Pattern != "/about" {
		t.Fatalf("want /about, got %q", r.Pattern)
	}
	if r.PackagePath != ".gen/routes/_g_marketing/about" {
		t.Fatalf("want .gen/routes/_g_marketing/about, got %q", r.PackagePath)
	}
}

func TestScanOptional(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "optional", "")
	if len(res.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", res.Diagnostics)
	}
	if len(res.Routes) != 1 {
		t.Fatalf("want 1 route, got %d", len(res.Routes))
	}
	r := res.Routes[0]
	if r.Pattern != "/[[lang]]/about" {
		t.Fatalf("want /[[lang]]/about, got %q", r.Pattern)
	}
	if r.PackagePath != ".gen/routes/__lang__/about" {
		t.Fatalf("want .gen/routes/__lang__/about, got %q", r.PackagePath)
	}
}

func TestScanRest(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "rest", "")
	if len(res.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", res.Diagnostics)
	}
	if len(res.Routes) != 1 {
		t.Fatalf("want 1 route, got %d", len(res.Routes))
	}
	r := res.Routes[0]
	if r.Pattern != "/docs/[...path]" {
		t.Fatalf("want /docs/[...path], got %q", r.Pattern)
	}
	if r.PackagePath != ".gen/routes/docs/___path" {
		t.Fatalf("want .gen/routes/docs/___path, got %q", r.PackagePath)
	}
}

func TestScanMatchers(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "matchers", "params")
	if len(res.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", res.Diagnostics)
	}
	if len(res.Matchers) != 1 || res.Matchers[0].Name != "int" {
		t.Fatalf("want one int matcher, got %+v", res.Matchers)
	}
	if len(res.Routes) != 1 {
		t.Fatalf("want 1 route, got %d", len(res.Routes))
	}
	if res.Routes[0].Segments[1].Matcher != "int" {
		t.Fatalf("want matcher int on id, got %+v", res.Routes[0].Segments)
	}
}

func TestScanConflictPageServer(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "conflict-page-server", "")
	if got := diagsContaining(res.Diagnostics, "may not have both +page.svelte and +server.go"); got == 0 {
		t.Fatalf("want page+server conflict diagnostic, got %v", res.Diagnostics)
	}
}

func TestScanConflictPattern(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "conflict-pattern", "")
	if got := diagsContaining(res.Diagnostics, "route conflict"); got == 0 {
		t.Fatalf("want route-conflict diagnostic, got %v", res.Diagnostics)
	}
}

func TestScanOrphanPageServer(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "orphan-pageserver", "")
	if got := diagsContaining(res.Diagnostics, "orphan +page.server.go"); got == 0 {
		t.Fatalf("want orphan diagnostic, got %v", res.Diagnostics)
	}
}

func TestScanMissingMatcher(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "missing-matcher", "")
	if got := diagsContaining(res.Diagnostics, "unknown matcher \"missing\""); got == 0 {
		t.Fatalf("want missing-matcher diagnostic, got %v", res.Diagnostics)
	}
}

func TestScanErrorBoundary(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "error-boundary", "")
	if len(res.Routes) != 1 || !res.Routes[0].HasError {
		t.Fatalf("want one route with HasError, got %+v", res.Routes)
	}
}

func TestScanLayoutChain(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "layout-chain", "")
	leaf := findRoute(res.Routes, "/dash/team/billing")
	if leaf == nil {
		t.Fatalf("missing leaf route, got %v", patterns(res.Routes))
	}
	if got := len(leaf.LayoutChain); got != 3 {
		t.Fatalf("want LayoutChain length 3, got %d (%v)", got, leaf.LayoutChain)
	}
	// Order: ancestor -> self.
	for i := 1; i < len(leaf.LayoutChain); i++ {
		if !strings.HasPrefix(leaf.LayoutChain[i], leaf.LayoutChain[i-1]) {
			t.Fatalf("layout chain not ancestor->self: %v", leaf.LayoutChain)
		}
	}
}

func TestScanReset(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "reset", "")
	if len(res.Routes) != 1 || !res.Routes[0].HasReset {
		t.Fatalf("want one route with HasReset, got %+v", res.Routes)
	}
}

func TestScanRoutesSorted(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "nested", "")
	for i := 1; i < len(res.Routes); i++ {
		if res.Routes[i-1].Pattern > res.Routes[i].Pattern {
			t.Fatalf("routes not sorted: %v", patterns(res.Routes))
		}
	}
}

func mustScan(t *testing.T, fixture, paramsSubdir string) *ScanResult {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	in := ScanInput{RoutesDir: filepath.Join(abs, "routes")}
	if paramsSubdir != "" {
		in.ParamsDir = filepath.Join(abs, paramsSubdir)
	}
	res, err := Scan(in)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return res
}

func patterns(routes []ScannedRoute) []string {
	out := make([]string, len(routes))
	for i, r := range routes {
		out[i] = r.Pattern
	}
	return out
}

func diagsContaining(ds []Diagnostic, sub string) int {
	n := 0
	for _, d := range ds {
		if strings.Contains(d.Message, sub) {
			n++
		}
	}
	return n
}

func findRoute(routes []ScannedRoute, pattern string) *ScannedRoute {
	for i := range routes {
		if routes[i].Pattern == pattern {
			return &routes[i]
		}
	}
	return nil
}
