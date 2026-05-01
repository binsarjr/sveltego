package svelterender

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite *.golden.json fixtures from sidecar output")

func TestEnsureNode(t *testing.T) {
	t.Parallel()
	// Node availability depends on the host. The contract under test is:
	// either the lookup succeeds and the path is non-empty, or it fails
	// with errNodeMissing wrapped via %w. Both outcomes are healthy.
	path, err := EnsureNode()
	if err != nil {
		if !errors.Is(err, errNodeMissing) {
			t.Fatalf("expected errNodeMissing wrap, got %v", err)
		}
		return
	}
	if path == "" {
		t.Fatal("EnsureNode succeeded but returned empty path")
	}
}

func TestPlan(t *testing.T) {
	t.Parallel()
	if got := Plan(nil); got != nil {
		t.Fatalf("Plan(nil) = %v, want nil", got)
	}
	if got := Plan([]Job{{}}); got != nil {
		t.Fatalf("Plan(empty job) = %v, want nil", got)
	}
	jobs := []Job{
		{Path: "/", Pattern: "/", SSRBundle: ".gen/ssr/index.js"},
		{Path: "/about", Pattern: "/about", SSRBundle: ".gen/ssr/about.js"},
	}
	got := Plan(jobs)
	if len(got) != 2 {
		t.Fatalf("Plan(2 valid) length = %d, want 2", len(got))
	}
}

func TestSidecarRoot(t *testing.T) {
	t.Parallel()
	dir, err := SidecarRoot()
	if err != nil {
		t.Fatalf("SidecarRoot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
		t.Fatalf("sidecar package.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.mjs")); err != nil {
		t.Fatalf("sidecar index.mjs missing: %v", err)
	}
}

// requireSidecarReady skips the test when Node or the sidecar's
// node_modules are unavailable. CI installs both before the gate; local
// runs may skip if a developer has not installed sidecar deps yet.
func requireSidecarReady(t *testing.T) (sidecarDir, nodePath string) {
	t.Helper()
	p, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH; skipping sidecar test")
	}
	dir, err := SidecarRoot()
	if err != nil {
		t.Skipf("sidecar tree not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "acorn")); err != nil {
		t.Skipf("sidecar node_modules missing; run `npm install` in %s", dir)
	}
	return dir, p
}

func TestBuildSSRAST_HelloWorld(t *testing.T) {
	t.Parallel()
	sidecarDir, nodePath := requireSidecarReady(t)
	root := filepath.Join("testdata", "ssr-ast")
	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	out := t.TempDir()
	results, err := BuildSSRAST(context.Background(), SSROptions{
		Root:       absRoot,
		OutDir:     out,
		SidecarDir: sidecarDir,
		NodePath:   nodePath,
		Jobs: []SSRJob{
			{Route: "/hello-world", Source: "hello-world/_page.svelte"},
			{Route: "/each-list", Source: "each-list/_page.svelte"},
		},
	})
	if err != nil {
		t.Fatalf("BuildSSRAST: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}

	for _, r := range results {
		if _, err := os.Stat(r.Output); err != nil {
			t.Fatalf("output missing for %s: %v", r.Route, err)
		}
	}

	cases := []struct {
		name   string
		out    string
		golden string
	}{
		{"hello-world", filepath.Join(out, "hello-world", "ast.json"), filepath.Join(absRoot, "hello-world", "ast.golden.json")},
		{"each-list", filepath.Join(out, "each-list", "ast.json"), filepath.Join(absRoot, "each-list", "ast.golden.json")},
	}
	for _, tc := range cases {
		got, err := os.ReadFile(tc.out)
		if err != nil {
			t.Fatalf("%s: read output: %v", tc.name, err)
		}
		if *updateGolden {
			if err := os.WriteFile(tc.golden, got, 0o600); err != nil {
				t.Fatalf("%s: update golden: %v", tc.name, err)
			}
			continue
		}
		want, err := os.ReadFile(tc.golden)
		if err != nil {
			t.Fatalf("%s: read golden: %v (run with -update to create)", tc.name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s: ast.json drift; run with -update to refresh golden", tc.name)
		}
		// Sanity: the AST root must be a Program node and contain at
		// least one ExportDefaultDeclaration so downstream emitter
		// consumers have a stable entry point. This guards against an
		// upstream Acorn or Svelte change silently emitting a different
		// shape that the goldens then rubber-stamp.
		var payload struct {
			Schema string `json:"schema"`
			AST    struct {
				Type string `json:"type"`
				Body []struct {
					Type string `json:"type"`
				} `json:"body"`
			} `json:"ast"`
		}
		if err := json.Unmarshal(got, &payload); err != nil {
			t.Fatalf("%s: payload unmarshal: %v", tc.name, err)
		}
		if payload.Schema == "" {
			t.Fatalf("%s: schema field empty", tc.name)
		}
		if payload.AST.Type != "Program" {
			t.Fatalf("%s: ast.type = %q, want Program", tc.name, payload.AST.Type)
		}
		var sawExport bool
		for _, b := range payload.AST.Body {
			if b.Type == "ExportDefaultDeclaration" {
				sawExport = true
				break
			}
		}
		if !sawExport {
			t.Fatalf("%s: no ExportDefaultDeclaration in Program body", tc.name)
		}
	}
}

func TestBuildSSRAST_Determinism(t *testing.T) {
	t.Parallel()
	sidecarDir, nodePath := requireSidecarReady(t)
	root := filepath.Join("testdata", "ssr-ast")
	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	jobs := []SSRJob{{Route: "/hello-world", Source: "hello-world/_page.svelte"}}

	out1 := t.TempDir()
	if _, err := BuildSSRAST(context.Background(), SSROptions{
		Root: absRoot, OutDir: out1, SidecarDir: sidecarDir, NodePath: nodePath, Jobs: jobs,
	}); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	out2 := t.TempDir()
	if _, err := BuildSSRAST(context.Background(), SSROptions{
		Root: absRoot, OutDir: out2, SidecarDir: sidecarDir, NodePath: nodePath, Jobs: jobs,
	}); err != nil {
		t.Fatalf("run 2: %v", err)
	}
	a, err := os.ReadFile(filepath.Join(out1, "hello-world", "ast.json"))
	if err != nil {
		t.Fatalf("read run 1: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(out2, "hello-world", "ast.json"))
	if err != nil {
		t.Fatalf("read run 2: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("ast.json not byte-identical across runs (sidecar non-deterministic)")
	}
}
