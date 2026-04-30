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
