// Package svelterender drives the Phase 3 hybrid-SSG sidecar for
// pure-Svelte routes (RFC #379, ADR 0008) and the Phase 2 SSR-AST
// extension (ADR 0009). When a route opts into Templates: "svelte" AND
// Prerender: true, the build pipeline must render the Svelte component
// to static HTML at build time so the runtime stays JS-free. When SSR
// Option B is engaged, the same sidecar additionally compiles each
// .svelte route via `svelte/compiler generate:'server'`, parses the
// resulting JS via vendored Acorn, and writes ESTree JSON ASTs that the
// Go-side pattern-match emitter (Phase 3, #425) consumes.
//
// The sidecar is a one-shot Node process. This package owns:
//
//   - Detecting whether any route in the manifest needs the sidecar.
//   - Locating Node on $PATH and emitting a clear error when required
//     but missing.
//   - Locating the sidecar source tree (vendored alongside this Go
//     package) and dispatching one of its modes.
//   - Building the JSON manifest the sidecar reads on stdin/disk and
//     parsing the JSON summary it writes to stdout.
//
// Runtime is unaffected; the deployed Go binary plus static/ is the
// entire deployable.
package svelterender

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// errNodeMissing is returned by EnsureNode when Phase-3 SSG would have
// to invoke Node but the binary is not on $PATH. The build pipeline
// surfaces it with a hint about Node 18+.
var errNodeMissing = errors.New("svelterender: node binary not found on $PATH")

// errSidecarMissing is returned when the vendored sidecar source tree
// (package.json + index.mjs) cannot be located. Distinct from
// errNodeMissing so callers can give the user a different remediation
// hint (reinstall vs install Node).
var errSidecarMissing = errors.New("svelterender: sidecar source tree not found")

// EnsureNode reports whether `node` is callable. It is a thin wrapper
// over exec.LookPath so callers can fail fast with a uniform message
// when prerender of Svelte-mode routes is requested but the toolchain
// is unavailable.
func EnsureNode() (string, error) {
	path, err := exec.LookPath("node")
	if err != nil {
		return "", fmt.Errorf("%w: install Node 18+ or remove Prerender: true from Svelte-mode routes", errNodeMissing)
	}
	return path, nil
}

// Job describes one Svelte-mode prerender unit. Path is the request
// path the sidecar materializes (the URL the resulting HTML will be
// served at); Pattern is the canonical route pattern for diagnostics;
// SSRBundle is the path to the compiled SSR module Vite produced for
// this route. The Phase-3 build wires the sidecar; Phase 4 fleshes out
// the data-loading bridge so the rendered HTML reflects Load() output.
type Job struct {
	Path      string
	Pattern   string
	SSRBundle string
}

// Plan filters the manifest to the subset of routes that need the
// sidecar: HasPage AND Templates: "svelte" AND Prerender: true.
// Returns nil when no routes qualify so the caller can skip Node
// invocation entirely.
func Plan(jobs []Job) []Job {
	out := make([]Job, 0, len(jobs))
	for _, j := range jobs {
		if j.Path == "" || j.Pattern == "" {
			continue
		}
		out = append(out, j)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// SSRJob describes one route the sidecar should compile to ESTree JSON
// AST in SSR mode (ADR 0009). Route is the canonical request path used
// for output slugging and diagnostics. Source is the .svelte path
// relative to Root the sidecar will read.
type SSRJob struct {
	Route  string `json:"route"`
	Source string `json:"source"`
}

// SSROptions configures BuildSSRAST. Root is the absolute project root
// (paths in SSRJob.Source resolve relative to it). OutDir is the
// directory the sidecar writes ast.json files into; when empty the
// sidecar defaults it to <Root>/.gen/svelte_js2go.
type SSROptions struct {
	Root       string
	OutDir     string
	Jobs       []SSRJob
	SidecarDir string // override for tests; resolves to vendored tree when empty
	NodePath   string // override for tests; falls back to exec.LookPath("node")
}

// SSRResult captures one route emitted by the sidecar.
type SSRResult struct {
	Route  string `json:"route"`
	Output string `json:"output"`
}

// ssrSummary mirrors the JSON summary the sidecar writes to stdout in
// SSR mode. It is consumed only by BuildSSRAST.
type ssrSummary struct {
	Schema  string      `json:"schema"`
	Results []SSRResult `json:"results"`
}

// SidecarRoot returns the absolute path to the vendored sidecar tree
// (sidecar/index.mjs and friends). Locating the directory at runtime
// depends on Go's runtime.Caller because the sidecar lives next to this
// Go file and ships with the module checkout — there is no install
// step. The returned path is empty when the tree cannot be found.
func SidecarRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("%w: runtime.Caller failed", errSidecarMissing)
	}
	dir := filepath.Join(filepath.Dir(thisFile), "sidecar")
	entry := filepath.Join(dir, "index.mjs")
	if _, err := os.Stat(entry); err != nil {
		return "", fmt.Errorf("%w at %s: %v", errSidecarMissing, entry, err)
	}
	return dir, nil
}

// BuildSSRAST runs the sidecar in SSR mode for the configured jobs and
// returns the per-route results. The sidecar is responsible for compile,
// parse, and JSON write. This function only marshals the manifest, runs
// the subprocess, and parses the result summary.
func BuildSSRAST(ctx context.Context, opts SSROptions) ([]SSRResult, error) {
	if opts.Root == "" {
		return nil, errors.New("svelterender: SSROptions.Root is required")
	}
	if len(opts.Jobs) == 0 {
		return nil, nil
	}
	nodePath := opts.NodePath
	if nodePath == "" {
		p, err := EnsureNode()
		if err != nil {
			return nil, err
		}
		nodePath = p
	}
	sidecarDir := opts.SidecarDir
	if sidecarDir == "" {
		p, err := SidecarRoot()
		if err != nil {
			return nil, err
		}
		sidecarDir = p
	}
	if _, err := os.Stat(filepath.Join(sidecarDir, "node_modules", "acorn")); err != nil {
		return nil, fmt.Errorf("svelterender: sidecar deps not installed at %s: run `npm install` in the sidecar tree", sidecarDir)
	}

	manifest := struct {
		Root   string   `json:"root"`
		OutDir string   `json:"outDir,omitempty"`
		Jobs   []SSRJob `json:"jobs"`
	}{
		Root:   opts.Root,
		OutDir: opts.OutDir,
		Jobs:   opts.Jobs,
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("svelterender: marshal manifest: %w", err)
	}

	tmp, err := os.CreateTemp("", "sveltego-ssr-manifest-*.json")
	if err != nil {
		return nil, fmt.Errorf("svelterender: temp manifest: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(manifestBytes); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("svelterender: write manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("svelterender: close manifest: %w", err)
	}

	entry := filepath.Join(sidecarDir, "index.mjs")
	cmd := exec.CommandContext(ctx, nodePath, entry, "--mode=ssr", "--manifest="+tmpPath)
	cmd.Dir = sidecarDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("svelterender: sidecar ssr: %w; stderr: %s", err, stderr.String())
	}
	var summary ssrSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		return nil, fmt.Errorf("svelterender: parse sidecar summary: %w; stdout: %s", err, stdout.String())
	}
	return summary.Results, nil
}
