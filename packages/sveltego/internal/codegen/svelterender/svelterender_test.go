package svelterender

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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
			{Route: "/named-import", Source: "named-import/_page.svelte"},
		},
	})
	if err != nil {
		t.Fatalf("BuildSSRAST: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
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
		// named-import is the regression case for the Acorn shared-Identifier
		// DAG that crashed the legacy WeakSet cycle detector. Source carries
		// `import { page } from '$app/state'` — the no-rename form that pins
		// `imported === local` on the ImportSpecifier (#460). The job MUST
		// produce a clean ast.json; absence of an error here is the test.
		{"named-import", filepath.Join(out, "named-import", "ast.json"), filepath.Join(absRoot, "named-import", "ast.golden.json")},
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

// TestSSRServe_AppAliases boots the real --mode=ssr-serve sidecar and
// renders a fixture that imports `$app/state` and `$app/navigation`.
// The fix for #460 wires server-side shims into the sidecar so the
// `$app/*` bare specifiers resolve at request time; before the fix Node
// would throw `Cannot find package '$app'` and the render would 5xx.
//
// The test only asserts a 200 response with non-empty body — it
// deliberately doesn't pin the exact rendered HTML, which depends on
// svelte/server output details and the shim's default page state.
func TestSSRServe_AppAliases(t *testing.T) {
	t.Parallel()
	sidecarDir, nodePath := requireSidecarReady(t)

	root := filepath.Join("testdata", "ssr-ast")
	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, nodePath, filepath.Join(sidecarDir, "index.mjs"),
		"--mode=ssr-serve",
		"--root="+absRoot,
		"--port=0",
	)
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sidecar: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	port, err := readSidecarPort(stderrPipe, 20*time.Second)
	if err != nil {
		t.Fatalf("read sidecar port: %v", err)
	}
	endpoint := "http://127.0.0.1:" + strconv.Itoa(port) + "/render"

	body, _ := json.Marshal(map[string]any{
		"route":  "/named-import",
		"source": "named-import/_page.svelte",
		"data":   map[string]any{"greeting": "hi"},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("post render: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, body=%s", resp.StatusCode, string(raw))
	}
	var out struct {
		Body  string `json:"body"`
		Head  string `json:"head"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v (raw=%s)", err, string(raw))
	}
	if out.Error != "" {
		t.Fatalf("sidecar error: %s", out.Error)
	}
	if out.Body == "" {
		t.Fatalf("empty body, raw=%s", string(raw))
	}
}

// readSidecarPort drains the sidecar's stderr until it sees the listen
// announcement, returning the port. Lines that don't match the prefix
// are discarded; a timeout is treated as boot failure.
func readSidecarPort(r io.Reader, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "SVELTEGO_SSR_FALLBACK_LISTEN port="); idx >= 0 {
			n, err := strconv.Atoi(strings.TrimSpace(line[idx+len("SVELTEGO_SSR_FALLBACK_LISTEN port="):]))
			if err != nil {
				return 0, err
			}
			return n, nil
		}
		if time.Now().After(deadline) {
			return 0, errors.New("timed out waiting for sidecar listen line")
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("sidecar exited before announcing port")
}
