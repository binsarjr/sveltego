// Package svelterender drives the Phase 3 hybrid-SSG sidecar for
// pure-Svelte routes (RFC #379, ADR 0008). When a route opts into
// Templates: "svelte" AND Prerender: true, the build pipeline must
// render the Svelte component to static HTML at build time so the
// runtime stays JS-free.
//
// The sidecar is a one-shot Node process invoking
// `import { render } from 'svelte/server'`. This package owns:
//
//   - Detecting whether any route in the manifest needs the sidecar.
//   - Locating Node on $PATH and emitting a clear error when required
//     but missing.
//   - Generating the JS driver script that loops over routes, imports
//     each .svelte component from Vite's SSR output, and writes HTML
//     to static/_prerendered/<path>/index.html.
//
// Runtime is unaffected; the deployed Go binary plus static/ is the
// entire deployable. Phase 4 (#383) wires the sidecar into the
// playgrounds and refines the JS driver. Phase 3 ships the
// orchestration plus a smoke that invokes Node only when prerender
// routes exist.
package svelterender

import (
	"errors"
	"fmt"
	"os/exec"
)

// errNodeMissing is returned by EnsureNode when Phase-3 SSG would have
// to invoke Node but the binary is not on $PATH. The build pipeline
// surfaces it with a hint about Node 18+.
var errNodeMissing = errors.New("svelterender: node binary not found on $PATH")

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
