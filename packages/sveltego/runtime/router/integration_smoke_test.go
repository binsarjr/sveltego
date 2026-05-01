// Build tag `integration` gates this end-to-end smoke test. Run with
// `go test -tags=integration ./runtime/router/...`. Default `go test` skips
// the file entirely.
//
// The test exercises the full Phase 0g pipeline:
//
//	testdata/smoke/routes/  →  routescan.Scan
//	                       →  codegen.GenerateManifest (formatted Go bytes)
//	                       →  written to a sandbox module
//	                       →  per-route stub Page packages emitted from the
//	                          same ScannedRoute data
//	                       →  go build ./... in sandbox with GOWORK=off
//	                       →  programmatically built []router.Route fed into
//	                          router.NewTree(...).WithMatchers(...)
//	                       →  match assertions across static, param,
//	                          matcher, optional, rest, and percent-encoded
//	                          slash inputs.
//
// We pick the subprocess path (vs go/types in-memory check) because the
// codegen package already ships a working subprocess pattern in
// internal/codegen/integration_test.go and the per-route stubs are cheap to
// generate from ScannedRoute.

//go:build integration

package router_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit/params"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/routescan"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

func TestRouterSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration smoke in -short mode")
	}

	sveltegoModuleDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs sveltego module: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sveltegoModuleDir, "go.mod")); err != nil {
		t.Fatalf("expected sveltego go.mod at %s: %v", sveltegoModuleDir, err)
	}

	fixtureRoot, err := filepath.Abs(filepath.Join("testdata", "smoke"))
	if err != nil {
		t.Fatalf("abs fixture: %v", err)
	}

	scan, err := routescan.Scan(routescan.ScanInput{
		RoutesDir: filepath.Join(fixtureRoot, "routes"),
		ParamsDir: filepath.Join(fixtureRoot, "params"),
	})
	if err != nil {
		t.Fatalf("routescan.Scan: %v", err)
	}
	if n := len(scan.Diagnostics); n != 0 {
		t.Fatalf("expected zero diagnostics, got %d:\n%v", n, scan.Diagnostics)
	}
	if len(scan.Routes) == 0 {
		t.Fatalf("scan returned no routes")
	}

	manifest, err := codegen.GenerateManifest(scan, codegen.ManifestOptions{
		PackageName: "gen",
		ModulePath:  "sveltegosmoke",
		GenRoot:     ".gen",
	})
	if err != nil {
		t.Fatalf("codegen.GenerateManifest: %v", err)
	}

	sandbox := t.TempDir()
	writeSandboxModule(t, sandbox, sveltegoModuleDir)
	writeManifest(t, sandbox, manifest)
	writeRouteStubs(t, sandbox, scan)

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = sandbox
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./... failed in %s (%s/%s):\n%s", sandbox, runtime.GOOS, runtime.GOARCH, output)
	}

	tree := buildTreeFromScan(t, scan)

	type want struct {
		path    string
		pattern string
		params  map[string]string
		nomatch bool
	}
	cases := []want{
		{path: "/", pattern: "/"},
		{path: "/about", pattern: "/about"},
		{path: "/post/42", pattern: "/post/[id=int]", params: map[string]string{"id": "42"}},
		{path: "/post/abc", nomatch: true},
		{path: "/en/about", pattern: "/[[lang]]/about", params: map[string]string{"lang": "en"}},
		{path: "/docs", pattern: "/docs/[...path]", params: map[string]string{"path": ""}},
		{path: "/docs/a/b", pattern: "/docs/[...path]", params: map[string]string{"path": "a/b"}},
		{path: "/docs/a%2Fb", pattern: "/docs/[...path]", params: map[string]string{"path": "a/b"}},
	}

	// Optional segment matching against the bare path "/about" is ambiguous
	// with the static /about route; static wins. For the optional-absent
	// assertion the brief calls out matching `/[[lang]]/about` against
	// `/about` with lang="" — but only when no static /about exists. We
	// already include /about, so cover the absent case via /xx/about? No,
	// that would resolve to the optional present arm. Use a separate
	// assertion: build a second tree without /about.
	t.Run("optional_absent_when_no_static_sibling", func(t *testing.T) {
		var routesNoStatic []router.Route
		for _, r := range tree.Routes() {
			if r.Pattern == "/about" {
				continue
			}
			routesNoStatic = append(routesNoStatic, router.Route{Pattern: r.Pattern, Segments: r.Segments})
		}
		alt, err := router.NewTree(routesNoStatic)
		if err != nil {
			t.Fatalf("NewTree(no-static): %v", err)
		}
		alt, err = alt.WithMatchers(params.DefaultMatchers())
		if err != nil {
			t.Fatalf("WithMatchers: %v", err)
		}
		r, ps, ok := alt.Match("/about")
		if !ok {
			t.Fatalf("expected match for /about against /[[lang]]/about")
		}
		if r.Pattern != "/[[lang]]/about" {
			t.Errorf("pattern = %q, want /[[lang]]/about", r.Pattern)
		}
		if got := ps["lang"]; got != "" {
			t.Errorf("lang = %q, want empty", got)
		}
	})

	for _, tc := range cases {
		t.Run(strings.ReplaceAll(tc.path, "/", "_"), func(t *testing.T) {
			r, ps, ok := tree.Match(tc.path)
			if tc.nomatch {
				if ok {
					t.Fatalf("expected no match for %q, got %q", tc.path, r.Pattern)
				}
				return
			}
			if !ok {
				t.Fatalf("no match for %q", tc.path)
			}
			if r.Pattern != tc.pattern {
				t.Fatalf("pattern = %q, want %q", r.Pattern, tc.pattern)
			}
			for k, v := range tc.params {
				if got := ps[k]; got != v {
					t.Errorf("param %q = %q, want %q", k, got, v)
				}
			}
			for k := range ps {
				if _, expected := tc.params[k]; !expected {
					t.Errorf("unexpected param %q = %q", k, ps[k])
				}
			}
		})
	}
}

func writeSandboxModule(t *testing.T, sandbox, sveltegoModuleDir string) {
	t.Helper()
	goMod := strings.Join([]string{
		"module sveltegosmoke",
		"",
		"go 1.22",
		"",
		"require github.com/binsarjr/sveltego/packages/sveltego v0.0.0-00010101000000-000000000000",
		"",
		"replace github.com/binsarjr/sveltego/packages/sveltego => " + sveltegoModuleDir,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(sandbox, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
}

func writeManifest(t *testing.T, sandbox string, manifest []byte) {
	t.Helper()
	manifestDir := filepath.Join(sandbox, ".gen")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("mkdir .gen: %v", err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "manifest.gen.go"), manifest, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// writeRouteStubs emits one stub package per ScannedRoute that the manifest
// imports. Each stub declares a no-op Page satisfying the PageHandler
// signature so go build can resolve the manifest's <alias>.Page{}.Render
// references without pulling in the full codegen output.
func writeRouteStubs(t *testing.T, sandbox string, scan *routescan.ScanResult) {
	t.Helper()
	for _, r := range scan.Routes {
		if !r.HasPage && !r.HasServer {
			continue
		}
		dir := filepath.Join(sandbox, filepath.FromSlash(r.PackagePath))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		var src string
		switch {
		case r.HasServer:
			src = `package ` + r.PackageName + `

import "net/http"

var Handlers = map[string]http.HandlerFunc{}
`
		case r.HasPage:
			src = `package ` + r.PackageName + `

import (
	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
)

type Page struct{}

func (Page) Render(w *render.Writer, ctx *kit.RenderCtx, data any) error {
	_ = w
	_ = ctx
	_ = data
	return nil
}
`
		}
		if r.HasPageServer {
			src += `
func Load(ctx *kit.LoadCtx) (any, error) { _ = ctx; return nil, nil }
func Actions() any                       { return nil }
`
		}
		if err := os.WriteFile(filepath.Join(dir, "page.gen.go"), []byte(src), 0o644); err != nil {
			t.Fatalf("write stub %s: %v", dir, err)
		}
	}
}

// buildTreeFromScan adapts ScannedRoute.Pattern + Segments into a router
// route table. PageHandler is left nil because the matcher only inspects
// the segment vector; the manifest carries the real handler refs.
func buildTreeFromScan(t *testing.T, scan *routescan.ScanResult) *router.Tree {
	t.Helper()
	routes := make([]router.Route, 0, len(scan.Routes))
	for _, r := range scan.Routes {
		if !r.HasPage && !r.HasServer {
			continue
		}
		routes = append(routes, router.Route{Pattern: r.Pattern, Segments: r.Segments})
	}
	tree, err := router.NewTree(routes)
	if err != nil {
		t.Fatalf("router.NewTree: %v", err)
	}
	tree, err = tree.WithMatchers(params.DefaultMatchers())
	if err != nil {
		t.Fatalf("router.WithMatchers: %v", err)
	}
	return tree
}
