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
	if got := diagsContaining(res.Diagnostics, "may not have both +page.svelte and server.go"); got == 0 {
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
	if got := diagsContaining(res.Diagnostics, "orphan page.server.go"); got == 0 {
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

func TestScanErrorChainNearest(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "error-chain", "")
	if len(res.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", res.Diagnostics)
	}

	root := findRoute(res.Routes, "/")
	if root == nil {
		t.Fatal("missing / route")
	}
	if !strings.HasSuffix(root.ErrorBoundaryDir, "/routes") {
		t.Fatalf("root boundary dir = %q, want suffix /routes", root.ErrorBoundaryDir)
	}
	if root.ErrorBoundaryPackagePath != ".gen/routes" {
		t.Fatalf("root boundary pkg = %q", root.ErrorBoundaryPackagePath)
	}
	if root.ErrorBoundaryLayoutDepth != 1 {
		t.Fatalf("root boundary depth = %d, want 1", root.ErrorBoundaryLayoutDepth)
	}

	admin := findRoute(res.Routes, "/admin")
	if admin == nil {
		t.Fatal("missing /admin route")
	}
	if !strings.HasSuffix(admin.ErrorBoundaryDir, "/admin") {
		t.Fatalf("admin boundary dir = %q, want suffix /admin", admin.ErrorBoundaryDir)
	}
	if admin.ErrorBoundaryLayoutDepth != 2 {
		t.Fatalf("admin boundary depth = %d, want 2", admin.ErrorBoundaryLayoutDepth)
	}

	users := findRoute(res.Routes, "/admin/users")
	if users == nil {
		t.Fatal("missing /admin/users route")
	}
	if !strings.HasSuffix(users.ErrorBoundaryDir, "/admin") {
		t.Fatalf("users boundary dir = %q, want suffix /admin", users.ErrorBoundaryDir)
	}
	if users.ErrorBoundaryLayoutDepth != 2 {
		t.Fatalf("users boundary depth = %d, want 2", users.ErrorBoundaryLayoutDepth)
	}
}

func TestScanNoErrorBoundary(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "simple", "")
	if len(res.Routes) != 1 {
		t.Fatalf("want one route, got %d", len(res.Routes))
	}
	r := res.Routes[0]
	if r.ErrorBoundaryDir != "" || r.ErrorBoundaryPackagePath != "" || r.ErrorBoundaryLayoutDepth != 0 {
		t.Fatalf("expected no boundary, got %+v", r)
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

func TestScanMissingBuildTag(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "missing-buildtag", "")
	if got := diagsContaining(res.Diagnostics, "missing //go:build sveltego"); got == 0 {
		t.Fatalf("want missing-buildtag diagnostic, got %v", res.Diagnostics)
	}
	// The route still scans cleanly otherwise (HasPage + HasPageServer set).
	if len(res.Routes) != 1 || !res.Routes[0].HasPageServer {
		t.Fatalf("want one route with HasPageServer, got %+v", res.Routes)
	}
}

func TestScanReset(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "reset", "")
	if len(res.Routes) != 1 || !res.Routes[0].HasReset {
		t.Fatalf("want one route with HasReset, got %+v", res.Routes)
	}
	if res.Routes[0].ResetTarget != "" {
		t.Fatalf("want empty ResetTarget for root reset, got %q", res.Routes[0].ResetTarget)
	}
	if !res.Routes[0].HasPage {
		t.Fatal("want HasPage true for +page@.svelte")
	}
}

func TestScanGroupsDeep(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "groups-deep", "")
	if len(res.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", res.Diagnostics)
	}
	leaf := findRoute(res.Routes, "/users")
	if leaf == nil {
		t.Fatalf("missing /users route, got %v", patterns(res.Routes))
	}
	if leaf.PackagePath != ".gen/routes/_g_app/_g_admin/users" {
		t.Fatalf("want package .gen/routes/_g_app/_g_admin/users, got %q", leaf.PackagePath)
	}
	if got := len(leaf.LayoutChain); got != 3 {
		t.Fatalf("want LayoutChain length 3 (root, app, admin), got %d (%v)", got, leaf.LayoutChain)
	}
	for i := 1; i < len(leaf.LayoutChain); i++ {
		if !strings.HasPrefix(leaf.LayoutChain[i], leaf.LayoutChain[i-1]) {
			t.Fatalf("layout chain not ancestor->self: %v", leaf.LayoutChain)
		}
	}
}

func TestScanLayoutResetRoot(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "layout-reset", "")
	embed := findRoute(res.Routes, "/level1/level2/embed")
	if embed == nil {
		t.Fatalf("missing /level1/level2/embed route, got %v", patterns(res.Routes))
	}
	if !embed.HasReset || embed.ResetTarget != "" {
		t.Fatalf("want root reset on embed route, got HasReset=%v ResetTarget=%q", embed.HasReset, embed.ResetTarget)
	}
	if len(embed.LayoutChain) != 0 {
		t.Fatalf("want empty LayoutChain after root reset, got %v", embed.LayoutChain)
	}
	if len(embed.LayoutPackagePaths) != 0 {
		t.Fatalf("want empty LayoutPackagePaths after root reset, got %v", embed.LayoutPackagePaths)
	}
}

func TestScanLayoutResetGroup(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "layout-reset", "")
	dash := findRoute(res.Routes, "/dash")
	if dash == nil {
		t.Fatalf("missing /dash route, got %v", patterns(res.Routes))
	}
	if !dash.HasReset || dash.ResetTarget != "(app)" {
		t.Fatalf("want ResetTarget=(app), got HasReset=%v ResetTarget=%q", dash.HasReset, dash.ResetTarget)
	}
	// chain truncates at the (app) ancestor inclusive: root layout dropped,
	// (app)/+layout.svelte kept.
	if len(dash.LayoutChain) != 1 {
		t.Fatalf("want LayoutChain length 1 after (app) reset, got %v", dash.LayoutChain)
	}
	if !strings.HasSuffix(dash.LayoutChain[0], "(app)") {
		t.Fatalf("want chain leaf to be (app), got %v", dash.LayoutChain)
	}
}

func TestScanGroupsConflict(t *testing.T) {
	t.Parallel()
	res := mustScan(t, "groups-conflict", "")
	if got := diagsContaining(res.Diagnostics, "route conflict"); got == 0 {
		t.Fatalf("want route-conflict diagnostic for duplicate /users across groups, got %v", res.Diagnostics)
	}
}

func TestParseResetFilename(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in     string
		base   string
		target string
		ok     bool
	}{
		{"+page@.svelte", "+page", "", true},
		{"+page@(app).svelte", "+page", "(app)", true},
		{"+layout@.svelte", "+layout", "", true},
		{"+layout@(admin).svelte", "+layout", "(admin)", true},
		{"+error@.svelte", "+error", "", true},
		{"+page.svelte", "", "", false},
		{"+page@.html", "", "", false},
		{"+page@(.svelte", "", "", false},
		{"+page@(123).svelte", "", "", false},
		{"+other@.svelte", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			base, target, ok := ParseResetFilename(tc.in)
			if ok != tc.ok || base != tc.base || target != tc.target {
				t.Fatalf("got (%q,%q,%v), want (%q,%q,%v)", base, target, ok, tc.base, tc.target, tc.ok)
			}
		})
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
