package codegen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/binsarjr/sveltego/internal/routescan"
)

// manifestFixtures lists the routescan testdata trees re-used as inputs for
// GenerateManifest. Each entry produces one golden under
// testdata/golden/manifest/<name>.golden. Run with -update or
// GOLDEN_UPDATE=1 to regenerate.
var manifestFixtures = []string{
	"simple",
	"nested",
	"groups",
	"optional",
	"rest",
	"layout-chain",
	"error-chain",
}

// pageOptionsFixture lives under testdata/page-options/ rather than
// the routescan testdata tree because the option scanner needs paired
// layout.server.go / page.server.go files; replicating them under
// routescan would distort the routescan goldens.
var pageOptionsFixture = "page-options"

func TestGenerateManifest_Goldens(t *testing.T) {
	t.Parallel()
	for _, name := range manifestFixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			scan := scanFixture(t, name)
			got, err := GenerateManifest(scan, ManifestOptions{
				PackageName: "gen",
				ModulePath:  "myapp",
				GenRoot:     ".gen",
			})
			if err != nil {
				t.Fatalf("GenerateManifest: %v", err)
			}
			assertManifestGolden(t, name, got)
		})
	}
}

func TestGenerateManifest_PageOptionsGolden(t *testing.T) {
	t.Parallel()
	abs, err := filepath.Abs(filepath.Join("testdata", pageOptionsFixture))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	scan, err := routescan.Scan(routescan.ScanInput{RoutesDir: filepath.Join(abs, "routes")})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	routeOpts, err := resolvePageOptions(scan)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got, err := GenerateManifest(scan, ManifestOptions{
		PackageName:  "gen",
		ModulePath:   "myapp",
		GenRoot:      ".gen",
		RouteOptions: routeOpts,
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	assertManifestGolden(t, pageOptionsFixture, got)
}

func TestGenerateManifest_Deterministic(t *testing.T) {
	t.Parallel()
	scan := scanFixture(t, "nested")
	opts := ManifestOptions{PackageName: "gen", ModulePath: "myapp", GenRoot: ".gen"}
	a, err := GenerateManifest(scan, opts)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	b, err := GenerateManifest(scan, opts)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("non-deterministic output:\n--- a:\n%s\n--- b:\n%s", a, b)
	}
}

func TestGenerateManifest_NilScan(t *testing.T) {
	t.Parallel()
	if _, err := GenerateManifest(nil, ManifestOptions{ModulePath: "myapp"}); err == nil {
		t.Fatal("expected error on nil scan")
	}
}

func TestGenerateManifest_EmptyModulePath(t *testing.T) {
	t.Parallel()
	scan := scanFixture(t, "simple")
	if _, err := GenerateManifest(scan, ManifestOptions{}); err == nil {
		t.Fatal("expected error on empty module path")
	}
}

func TestGenerateManifest_DiagnosticsSurfaced(t *testing.T) {
	t.Parallel()
	scan := scanFixture(t, "conflict-pattern")
	out, err := GenerateManifest(scan, ManifestOptions{ModulePath: "myapp", GenRoot: ".gen"})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if !bytes.Contains(out, []byte("// SCANNER DIAGNOSTIC")) {
		t.Fatalf("expected SCANNER DIAGNOSTIC line, got:\n%s", out)
	}
}

// TestGenerateManifest_PageHead exercises the head adapter + Head field
// emission when a page route declares a Head method via PageHeads.
func TestGenerateManifest_PageHead(t *testing.T) {
	t.Parallel()
	scan := scanFixture(t, "simple")
	pageHeads := map[string]bool{}
	for _, r := range scan.Routes {
		if r.HasPage {
			pageHeads[r.PackagePath] = true
		}
	}
	out, err := GenerateManifest(scan, ManifestOptions{
		PackageName: "gen",
		ModulePath:  "myapp",
		GenRoot:     ".gen",
		PageHeads:   pageHeads,
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"func head__",
		".Page{}.Head(",
		"Head:",
		"head__page_routes",
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// TestGenerateManifest_LayoutHead exercises the layout-head adapter +
// LayoutHeads field emission when a layout package declares a Head
// method via LayoutHeads.
func TestGenerateManifest_LayoutHead(t *testing.T) {
	t.Parallel()
	scan := scanFixture(t, "layout-chain")
	layoutHeads := map[string]bool{}
	seen := map[string]struct{}{}
	for _, r := range scan.Routes {
		for _, p := range r.LayoutPackagePaths {
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			layoutHeads[p] = true
		}
	}
	out, err := GenerateManifest(scan, ManifestOptions{
		PackageName: "gen",
		ModulePath:  "myapp",
		GenRoot:     ".gen",
		LayoutHeads: layoutHeads,
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"func head__layout__",
		".Layout{}.Head(",
		"LayoutHeads: []router.LayoutHeadHandler{",
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// TestGenerateManifest_RouteIDConsts verifies that the manifest contains the
// expected RouteID constants for the nested fixture.
func TestGenerateManifest_RouteIDConsts(t *testing.T) {
	t.Parallel()
	scan := scanFixture(t, "nested")
	out, err := GenerateManifest(scan, ManifestOptions{
		PackageName: "gen",
		ModulePath:  "myapp",
		GenRoot:     ".gen",
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"RouteIDIndex",
		"RouteIDAbout",
		"RouteIDPostsSlug",
		`RouteIDIndex     = ` + "`/`",
		`RouteIDAbout     = ` + "`/about`",
		`RouteIDPostsSlug = ` + "`/posts/[slug]`",
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// TestGenerateManifest_HasServiceWorker_True verifies that ManifestOptions.HasServiceWorker = true
// emits `const HasServiceWorker = true` so the runtime can gate the auto-registration
// <script> tag (#89).
func TestGenerateManifest_HasServiceWorker_True(t *testing.T) {
	t.Parallel()
	scan := scanFixture(t, "simple")
	out, err := GenerateManifest(scan, ManifestOptions{
		PackageName:      "gen",
		ModulePath:       "myapp",
		GenRoot:          ".gen",
		HasServiceWorker: true,
	})
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if !bytes.Contains(out, []byte("const HasServiceWorker = true")) {
		t.Fatalf("expected `const HasServiceWorker = true` in:\n%s", out)
	}
}

// TestGenerateManifest_HasServiceWorker_False verifies the constant defaults to
// false so server code can read it unconditionally without a build-tag dance.
func TestGenerateManifest_HasServiceWorker_False(t *testing.T) {
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
	if !bytes.Contains(out, []byte("const HasServiceWorker = false")) {
		t.Fatalf("expected `const HasServiceWorker = false` in:\n%s", out)
	}
}

// TestRouteIDCollision verifies that emitRouteIDConsts errors when two entries
// produce the same Go identifier. Two routes with nil Segments both lower to
// "Index" via routeIdent, triggering the collision guard.
func TestRouteIDCollision(t *testing.T) {
	t.Parallel()
	// Both entries have nil Segments → routeIdent returns "Index" for both.
	entries := []entry{
		{route: routescan.ScannedRoute{Pattern: "/", HasPage: true}, alias: "a"},
		{route: routescan.ScannedRoute{Pattern: "/mirror", HasPage: true}, alias: "b"},
	}
	var b Builder
	err := emitRouteIDConsts(&b, entries)
	if err == nil {
		t.Fatal("expected collision error, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("RouteID collision")) {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func scanFixture(t *testing.T, name string) *routescan.ScanResult {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "routescan", "testdata", name))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	in := routescan.ScanInput{RoutesDir: filepath.Join(abs, "routes")}
	if _, err := os.Stat(filepath.Join(abs, "params")); err == nil {
		in.ParamsDir = filepath.Join(abs, "params")
	}
	res, err := routescan.Scan(in)
	if err != nil {
		t.Fatalf("scan %s: %v", name, err)
	}
	return res
}

func assertManifestGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "golden", "manifest", name+".golden")
	if os.Getenv("GOLDEN_UPDATE") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with GOLDEN_UPDATE=1): %v", path, err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("golden mismatch in %s; run GOLDEN_UPDATE=1\n--- want:\n%s\n--- got:\n%s", path, want, got)
	}
}
