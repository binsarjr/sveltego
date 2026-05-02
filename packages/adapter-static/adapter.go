// Package adapterstatic provides a build-time adapter that produces
// prerendered static-site output (SSG). It drives sveltego's existing
// Server.Prerender engine by spawning the user binary with
// SVELTEGO_PRERENDER=1, then packages the resulting tree into a flat
// deploy directory ready for any static host (S3, GitHub Pages,
// Cloudflare Pages, Netlify static, etc.).
//
// The adapter is dependency-light: it does not import the sveltego
// runtime at compile time. Production callers go through the
// subprocess Runner; tests inject a custom Runner so they can drive
// Server.Prerender in-process without spawning a child binary.
package adapterstatic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Name is the canonical target name for this adapter.
const Name = "static"

// PrerenderManifestFilename is the file the adapter writes at the root
// of OutputDir summarizing the deploy tree. It is distinct from
// sveltego's internal manifest.json (which lives inside the scratch
// dir during the run) so a static host that serves the deploy tree
// verbatim does not accidentally expose the runtime artifact.
const PrerenderManifestFilename = "_prerender_manifest.json"

// DefaultMainPackage is the user main package compiled when
// BuildContext.MainPackage is empty. Mirrors `sveltego prerender`'s
// default.
const DefaultMainPackage = "./cmd/app"

// BuildContext is the input contract for Build.
type BuildContext struct {
	// ProjectRoot is the absolute path of the user's project root (the
	// directory containing go.mod and src/routes/).
	ProjectRoot string

	// OutputDir is the absolute path where the flat deploy tree is
	// written. Created if missing. Existing contents are not deleted —
	// callers that want a clean tree should remove the directory first.
	OutputDir string

	// MainPackage is the user main package import path or directory
	// passed to `go build`. Defaults to DefaultMainPackage when empty.
	MainPackage string

	// FailOnDynamic, when true, returns ErrDynamicRoutes if any route in
	// the user's project is not opted into prerender. The set of
	// dynamic routes is reported via the Runner's RunInfo.DynamicRoutes
	// field.
	FailOnDynamic bool

	// ScratchDir overrides the working directory used by the runner
	// while the prerender pass runs. Default: a sibling of OutputDir
	// named ".sveltego-static-scratch". The directory is removed at the
	// end of a successful Build.
	ScratchDir string

	// Stdout and Stderr capture build/runner output. Default os.Stdout
	// / os.Stderr.
	Stdout, Stderr io.Writer

	// Runner overrides the default subprocess runner. Production callers
	// leave this nil; tests inject an in-process runner that calls
	// Server.Prerender directly so they do not have to compile a real
	// user binary.
	Runner Runner
}

// Runner drives the prerender pass and writes its output into
// scratchDir using sveltego's Server.Prerender layout
// (scratchDir/manifest.json + scratchDir/<route>/index.html).
//
// RunInfo reports the route patterns the runner knows about so the
// adapter can implement BuildContext.FailOnDynamic.
type Runner interface {
	Prerender(ctx context.Context, projectRoot, scratchDir string) (RunInfo, error)
}

// RunInfo summarizes the route plan a Runner observed.
type RunInfo struct {
	// PrerenderedRoutes is the canonical pattern of every route the
	// runner wrote HTML for.
	PrerenderedRoutes []string

	// DynamicRoutes is the canonical pattern of every route in the
	// project that did not produce HTML (no Prerender flag, or dynamic
	// param without an entries supplier). May be empty even when the
	// runner can produce a list — only populated by runners that have
	// access to the full route table.
	DynamicRoutes []string
}

// ErrDynamicRoutes is returned by Build when BuildContext.FailOnDynamic
// is set and the runner reported one or more non-prerenderable routes.
type ErrDynamicRoutes struct {
	Routes []string
}

// Error implements error. The message lists every offending pattern in
// stable order so log output is grep-friendly.
func (e *ErrDynamicRoutes) Error() string {
	if e == nil || len(e.Routes) == 0 {
		return "adapter-static: dynamic routes present (none reported)"
	}
	return fmt.Sprintf("adapter-static: %d dynamic route(s) cannot be prerendered: %s",
		len(e.Routes), strings.Join(e.Routes, ", "))
}

// scratchManifest mirrors the JSON shape Server.Prerender writes. The
// adapter only needs the entries field to walk the produced tree.
type scratchManifest struct {
	Entries []scratchEntry `json:"entries"`
}

type scratchEntry struct {
	Route     string `json:"route"`
	Path      string `json:"path"`
	File      string `json:"file"`
	Protected bool   `json:"protected,omitempty"`
}

// outputManifest is the JSON payload written to
// OutputDir/_prerender_manifest.json. It intentionally omits
// wall-clock timestamps so two consecutive Build calls produce
// byte-identical output (#447 idempotency criterion).
type outputManifest struct {
	Version       int             `json:"version"`
	SourceSHA     string          `json:"sourceSHA"`
	Entries       []manifestEntry `json:"entries"`
	DynamicRoutes []string        `json:"dynamicRoutes,omitempty"`
}

type manifestEntry struct {
	Route     string `json:"route"`
	Path      string `json:"path"`
	File      string `json:"file"`
	SHA256    string `json:"sha256"`
	Protected bool   `json:"protected,omitempty"`
}

// Build runs the prerender pass for bc.ProjectRoot, packages the
// rendered HTML into bc.OutputDir as a flat tree of <route>/index.html
// files, copies the project's static/ directory verbatim (minus the
// runtime _prerendered scratch sibling) to OutputDir/static/, and
// writes an OutputDir-rooted manifest summarizing the result.
//
// Two consecutive calls with identical inputs produce byte-identical
// output trees so downstream `rsync`/`aws s3 sync` commands do not
// thrash on no-op rebuilds.
func Build(ctx context.Context, bc BuildContext) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if bc.ProjectRoot == "" {
		return errors.New("adapter-static: ProjectRoot is required")
	}
	if !filepath.IsAbs(bc.ProjectRoot) {
		return fmt.Errorf("adapter-static: ProjectRoot %q must be absolute", bc.ProjectRoot)
	}
	if bc.OutputDir == "" {
		return errors.New("adapter-static: OutputDir is required")
	}
	if !filepath.IsAbs(bc.OutputDir) {
		return fmt.Errorf("adapter-static: OutputDir %q must be absolute", bc.OutputDir)
	}
	if _, err := os.Stat(bc.ProjectRoot); err != nil {
		return fmt.Errorf("adapter-static: project root: %w", err)
	}

	stdout := bc.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := bc.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	scratchDir := bc.ScratchDir
	if scratchDir == "" {
		scratchDir = filepath.Join(filepath.Dir(bc.OutputDir), ".sveltego-static-scratch")
	}
	if err := os.RemoveAll(scratchDir); err != nil {
		return fmt.Errorf("adapter-static: clean scratch dir: %w", err)
	}
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return fmt.Errorf("adapter-static: mkdir scratch dir: %w", err)
	}
	defer os.RemoveAll(scratchDir) //nolint:errcheck // best-effort cleanup

	runner := bc.Runner
	if runner == nil {
		mainPkg := bc.MainPackage
		if mainPkg == "" {
			mainPkg = DefaultMainPackage
		}
		runner = &subprocessRunner{
			MainPackage: mainPkg,
			Stdout:      stdout,
			Stderr:      stderr,
		}
	}

	info, err := runner.Prerender(ctx, bc.ProjectRoot, scratchDir)
	if err != nil {
		return fmt.Errorf("adapter-static: prerender: %w", err)
	}

	if bc.FailOnDynamic && len(info.DynamicRoutes) > 0 {
		sorted := append([]string(nil), info.DynamicRoutes...)
		sort.Strings(sorted)
		return &ErrDynamicRoutes{Routes: sorted}
	}

	if err := os.MkdirAll(bc.OutputDir, 0o755); err != nil {
		return fmt.Errorf("adapter-static: mkdir output dir: %w", err)
	}

	scratchEntries, err := readScratchManifest(scratchDir)
	if err != nil {
		return err
	}

	manifestEntries, err := copyPrerenderedTree(scratchDir, bc.OutputDir, scratchEntries)
	if err != nil {
		return err
	}

	staticSrc := filepath.Join(bc.ProjectRoot, "static")
	if _, err := os.Stat(staticSrc); err == nil {
		if err := copyStaticDir(staticSrc, filepath.Join(bc.OutputDir, "static")); err != nil {
			return fmt.Errorf("adapter-static: copy static/: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("adapter-static: stat static/: %w", err)
	}

	dynamic := append([]string(nil), info.DynamicRoutes...)
	sort.Strings(dynamic)
	if err := writeOutputManifest(bc.OutputDir, manifestEntries, dynamic); err != nil {
		return err
	}

	return nil
}

// readScratchManifest parses the manifest.json Server.Prerender wrote
// into scratchDir.
func readScratchManifest(scratchDir string) ([]scratchEntry, error) {
	manifestPath := filepath.Join(scratchDir, "manifest.json")
	body, err := os.ReadFile(manifestPath) //nolint:gosec // path is adapter-controlled
	if err != nil {
		return nil, fmt.Errorf("adapter-static: read scratch manifest: %w", err)
	}
	var m scratchManifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("adapter-static: parse scratch manifest: %w", err)
	}
	sort.Slice(m.Entries, func(i, j int) bool { return m.Entries[i].Path < m.Entries[j].Path })
	return m.Entries, nil
}

// copyPrerenderedTree copies every entry from scratchDir to outDir at
// the same relative path. Returns the per-entry manifest rows used by
// the OutputDir manifest.
func copyPrerenderedTree(scratchDir, outDir string, entries []scratchEntry) ([]manifestEntry, error) {
	out := make([]manifestEntry, 0, len(entries))
	for _, e := range entries {
		rel := filepath.FromSlash(e.File)
		src := filepath.Join(scratchDir, rel)
		dst := filepath.Join(outDir, rel)
		body, err := os.ReadFile(src) //nolint:gosec // path is adapter-controlled
		if err != nil {
			return nil, fmt.Errorf("adapter-static: read prerendered %s: %w", e.File, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, fmt.Errorf("adapter-static: mkdir for %s: %w", e.File, err)
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil { //nolint:gosec
			return nil, fmt.Errorf("adapter-static: write prerendered %s: %w", e.File, err)
		}
		sum := sha256.Sum256(body)
		out = append(out, manifestEntry{
			Route:     e.Route,
			Path:      e.Path,
			File:      e.File,
			SHA256:    hex.EncodeToString(sum[:]),
			Protected: e.Protected,
		})
	}
	return out, nil
}

// copyStaticDir mirrors src into dst, skipping the runtime
// `_prerendered` scratch directory (Server.Prerender's default
// OutDir). Preserves regular files only.
func copyStaticDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		// Skip the prerender runtime artifact dir if the user binary
		// happened to write into static/_prerendered/. The deploy tree
		// already contains the prerendered HTML at top level; we do not
		// want a duplicate copy under static/.
		if rel == "_prerendered" || strings.HasPrefix(rel, "_prerendered"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

// copyFile is a tiny CopyFile mirroring adapter-server/internal/fsutil
// without a dependency on that internal package.
func copyFile(src, dst string, perm os.FileMode) (err error) {
	if mkErr := os.MkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
		return mkErr
	}
	in, err := os.Open(src) //nolint:gosec // adapter-controlled
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, in.Close())
	}()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, out.Close())
	}()
	_, err = io.Copy(out, in)
	return err
}

// writeOutputManifest writes the deterministic OutputDir manifest.
// SourceSHA hashes the route patterns + per-file SHA256s so callers can
// detect when nothing has changed without comparing every file by hand.
func writeOutputManifest(outDir string, entries []manifestEntry, dynamic []string) error {
	hasher := sha256.New()
	for _, e := range entries {
		_, _ = hasher.Write([]byte(e.Route))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(e.SHA256))
		_, _ = hasher.Write([]byte{0})
	}
	for _, r := range dynamic {
		_, _ = hasher.Write([]byte(r))
		_, _ = hasher.Write([]byte{0})
	}

	m := outputManifest{
		Version:       1,
		SourceSHA:     hex.EncodeToString(hasher.Sum(nil)),
		Entries:       entries,
		DynamicRoutes: dynamic,
	}
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("adapter-static: marshal manifest: %w", err)
	}
	body = append(body, '\n')
	target := filepath.Join(outDir, PrerenderManifestFilename)
	if err := os.WriteFile(target, body, 0o644); err != nil { //nolint:gosec
		return fmt.Errorf("adapter-static: write manifest: %w", err)
	}
	return nil
}

// Doc returns a short deploy guide for the static target.
func Doc() string {
	return `Static target — SSG (prerendered HTML)

  1. sveltego build --target=static --out dist/
  2. dist/ contains <route>/index.html for every prerendered route,
     a static/ subtree mirrored from the project's static directory,
     and a _prerender_manifest.json summary at the root.
  3. Upload the tree to any static host (S3, GitHub Pages,
     Cloudflare Pages, Netlify static, your own nginx).

Dynamic routes (no Prerender flag, or dynamic params without an
entries supplier) are not written. Pass FailOnDynamic: true to make
their presence a hard build failure.

Idempotent: re-running Build on unchanged input produces a
byte-identical tree.`
}
