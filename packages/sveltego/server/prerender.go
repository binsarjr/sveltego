package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

const (
	// PrerenderManifestFilename is the file written under
	// PrerenderOutDir holding the JSON list of prerendered routes.
	PrerenderManifestFilename = "manifest.json"

	// DefaultPrerenderOutDir is the path (relative to project root)
	// the prerender pipeline writes static HTML into. The runtime
	// staticPrerendered handler reads from the same root.
	DefaultPrerenderOutDir = "static/_prerendered"

	// PrerenderTriggerEnv is the env var the `sveltego prerender`
	// CLI sets before invoking the user binary. MaybePrerenderFromEnv
	// looks for it; absent means "boot normally".
	PrerenderTriggerEnv = "SVELTEGO_PRERENDER"

	// PrerenderOutDirEnv overrides the manifest output directory.
	PrerenderOutDirEnv = "SVELTEGO_PRERENDER_OUT"

	// PrerenderTolerateEnv mirrors --tolerate from the CLI.
	PrerenderTolerateEnv = "SVELTEGO_PRERENDER_TOLERATE"

	// PrerenderReportEnv mirrors --report=<path> from the CLI.
	PrerenderReportEnv = "SVELTEGO_PRERENDER_REPORT"
)

// PrerenderOptions configures a prerender run. OutDir is the directory
// (relative to the project root) the engine writes generated HTML and
// the manifest into; default DefaultPrerenderOutDir.
//
// Tolerate caps the number of HTTP errors the run will absorb before
// returning a non-nil error. -1 disables the cap; 0 fails on the first
// error. ReportPath, when non-empty, additionally writes a JSON copy of
// the error list (one object per line) to that path so CI can annotate
// individual failures (#185).
//
// Entries supplies parameter combinations for dynamic prerenderable
// routes keyed by canonical route pattern (e.g. "/post/[slug]" ->
// [{"slug": "hello"}]). Missing keys fall back to an empty entries list,
// meaning the route is skipped unless it has no dynamic params.
type PrerenderOptions struct {
	OutDir     string
	Tolerate   int
	ReportPath string
	Entries    map[string][]map[string]string
}

// PrerenderError carries one route-level prerender failure. Status is
// the HTTP status code captured from the in-memory pipeline; Message is
// the response body trimmed to a sensible width.
type PrerenderError struct {
	Route   string `json:"route"`
	Path    string `json:"path"`
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// PrerenderErrors is the aggregate error returned by Prerender when at
// least one route failed beyond the configured tolerance. Implements
// error so callers can treat it as a normal return; iterating Errors
// surfaces every captured failure.
type PrerenderErrors struct {
	Errors []PrerenderError
}

// Error implements error. The first failure becomes the headline; the
// remainder are summarized via "+N more" so log lines do not blow up.
func (p *PrerenderErrors) Error() string {
	if len(p.Errors) == 0 {
		return "prerender: no errors"
	}
	first := p.Errors[0]
	if len(p.Errors) == 1 {
		return fmt.Sprintf("prerender: %s -> %d %s", first.Path, first.Status, truncMessage(first.Message))
	}
	return fmt.Sprintf("prerender: %s -> %d %s (+%d more)",
		first.Path, first.Status, truncMessage(first.Message), len(p.Errors)-1)
}

// PrerenderResult summarizes a successful prerender pass.
type PrerenderResult struct {
	OutDir       string             `json:"outDir"`
	GeneratedAt  time.Time          `json:"generatedAt"`
	Entries      []PrerenderedEntry `json:"entries"`
	Errors       []PrerenderError   `json:"errors,omitempty"`
	ManifestPath string             `json:"-"`
}

// PrerenderedEntry is one row in the runtime manifest. Path is the URL
// path served (always begins with "/"); File is the relative file path
// under OutDir. Protected, when true, marks the entry as gated by the
// PrerenderProtected option so the runtime calls AuthGate before serving.
type PrerenderedEntry struct {
	Route     string `json:"route"`
	Path      string `json:"path"`
	File      string `json:"file"`
	Protected bool   `json:"protected,omitempty"`
}

// Prerender walks s.tree, picks routes opted into prerender, fetches
// each via the in-memory pipeline, and writes the rendered HTML to
// disk. It returns a manifest describing every entry written and a
// non-nil error when any failures exceed opts.Tolerate. projectRoot is
// the directory the OutDir is resolved against (typically the user's
// go.mod dir).
func (s *Server) Prerender(ctx context.Context, projectRoot string, opts PrerenderOptions) (*PrerenderResult, error) {
	if projectRoot == "" {
		return nil, errors.New("server: Prerender: empty project root")
	}
	if !filepath.IsAbs(projectRoot) {
		return nil, fmt.Errorf("server: Prerender: project root %q must be absolute", projectRoot)
	}
	outDir := opts.OutDir
	if outDir == "" {
		outDir = DefaultPrerenderOutDir
	}
	outAbs := filepath.Join(projectRoot, outDir)
	if err := os.RemoveAll(outAbs); err != nil {
		return nil, fmt.Errorf("server: clean prerender out dir: %w", err)
	}
	if err := os.MkdirAll(outAbs, 0o755); err != nil {
		return nil, fmt.Errorf("server: mkdir prerender out dir: %w", err)
	}

	if err := s.Init(ctx); err != nil {
		// Init may legitimately fail if the user wired a strict Init hook;
		// surface the failure rather than silently producing empty pages.
		return nil, fmt.Errorf("server: prerender init: %w", err)
	}

	jobs, err := planPrerenderJobs(s.tree.Routes(), opts.Entries)
	if err != nil {
		return nil, err
	}

	res := &PrerenderResult{
		OutDir:      outDir,
		GeneratedAt: time.Now().UTC(),
	}

	// We always run every job and aggregate failures (#185); Tolerate
	// only affects whether the surrounding return wraps them in an
	// error after the loop completes.
	tolerate := opts.Tolerate
	for _, job := range jobs {
		entry, perr := s.runOnePrerenderJob(ctx, job, outAbs, outDir)
		if perr != nil {
			res.Errors = append(res.Errors, *perr)
			continue
		}
		res.Entries = append(res.Entries, entry)
	}

	sort.Slice(res.Entries, func(i, j int) bool { return res.Entries[i].Path < res.Entries[j].Path })
	sort.Slice(res.Errors, func(i, j int) bool { return res.Errors[i].Path < res.Errors[j].Path })

	manifestPath, err := writePrerenderManifest(outAbs, res)
	if err != nil {
		return nil, err
	}
	res.ManifestPath = manifestPath

	if opts.ReportPath != "" && len(res.Errors) > 0 {
		if err := writePrerenderReport(opts.ReportPath, res.Errors); err != nil {
			return nil, err
		}
	}

	if tolerate >= 0 && len(res.Errors) > tolerate {
		return res, &PrerenderErrors{Errors: res.Errors}
	}
	return res, nil
}

// prerenderJob is one URL the engine fetches against the in-memory
// pipeline. Path is the request path; Route is the canonical pattern
// for diagnostics. Protected mirrors the route's Options.PrerenderProtected
// so the manifest carries the flag.
type prerenderJob struct {
	Path      string
	Route     string
	Protected bool
}

// planPrerenderJobs filters routes to those opted into prerender and
// expands dynamic patterns via opts.Entries. Auto routes with no
// dynamic params and no Load are included; auto routes with dynamic
// params or a server Load are skipped (they SSR at request time).
func planPrerenderJobs(routes []router.Route, entriesMap map[string][]map[string]string) ([]prerenderJob, error) {
	var out []prerenderJob
	for i := range routes {
		r := &routes[i]
		if r.Page == nil {
			continue
		}
		opts := r.Options
		if !opts.Prerender && !opts.PrerenderAuto {
			continue
		}
		dynamic := routeHasDynamicSegments(r.Segments)
		if opts.PrerenderAuto && (dynamic || r.Load != nil) {
			continue
		}
		if !dynamic {
			out = append(out, prerenderJob{
				Path:      patternToPath(r.Pattern),
				Route:     r.Pattern,
				Protected: opts.PrerenderProtected,
			})
			continue
		}
		entries := entriesMap[r.Pattern]
		for _, e := range entries {
			path, err := substituteParams(r.Pattern, e)
			if err != nil {
				return nil, fmt.Errorf("prerender: route %s: %w", r.Pattern, err)
			}
			out = append(out, prerenderJob{
				Path:      path,
				Route:     r.Pattern,
				Protected: opts.PrerenderProtected,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func routeHasDynamicSegments(segs []router.Segment) bool {
	for _, s := range segs {
		switch s.Kind {
		case router.SegmentParam, router.SegmentOptional, router.SegmentRest:
			return true
		}
	}
	return false
}

// patternToPath turns a static route pattern into the URL path the
// engine fetches. Routes with dynamic segments are not handled here;
// callers must supply concrete params via Entries.
func patternToPath(pattern string) string {
	if pattern == "" {
		return "/"
	}
	if pattern == "/" {
		return "/"
	}
	return pattern
}

// substituteParams replaces [name] / [name=matcher] / [[name]] / [...rest]
// segments in pattern with the matching key from params. Keys missing
// from params return an error so the run fails loudly rather than
// emitting a literal "[slug]" path.
func substituteParams(pattern string, params map[string]string) (string, error) {
	parts := strings.Split(pattern, "/")
	for i, seg := range parts {
		if !strings.HasPrefix(seg, "[") || !strings.HasSuffix(seg, "]") {
			continue
		}
		inner := seg[1 : len(seg)-1]
		// strip leading "..."
		inner = strings.TrimPrefix(inner, "...")
		// strip optional brackets
		inner = strings.TrimPrefix(inner, "[")
		inner = strings.TrimSuffix(inner, "]")
		// strip matcher suffix "=foo"
		if eq := strings.IndexByte(inner, '='); eq >= 0 {
			inner = inner[:eq]
		}
		val, ok := params[inner]
		if !ok {
			return "", fmt.Errorf("missing entry for param %q", inner)
		}
		parts[i] = val
	}
	return strings.Join(parts, "/"), nil
}

func (s *Server) runOnePrerenderJob(ctx context.Context, job prerenderJob, outAbs, _ string) (PrerenderedEntry, *PrerenderError) {
	req := httptest.NewRequestWithContext(ctx, "GET", job.Path, nil)
	// Hint to Locals warning + handler that this is a prerender pass.
	req.Header.Set(prerenderProbeHeader, "1")
	rec := httptest.NewRecorder()
	s.handle(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return PrerenderedEntry{}, &PrerenderError{
			Route:   job.Route,
			Path:    job.Path,
			Status:  resp.StatusCode,
			Message: strings.TrimSpace(string(body)),
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return PrerenderedEntry{}, &PrerenderError{
			Route:   job.Route,
			Path:    job.Path,
			Status:  500,
			Message: err.Error(),
		}
	}

	relFile := filepath.FromSlash(strings.Trim(job.Path, "/"))
	if relFile == "" {
		relFile = "index"
	}
	target := filepath.Join(outAbs, relFile, "index.html")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return PrerenderedEntry{}, &PrerenderError{
			Route:   job.Route,
			Path:    job.Path,
			Status:  500,
			Message: fmt.Sprintf("mkdir: %v", err),
		}
	}
	if err := os.WriteFile(target, body, 0o644); err != nil { //nolint:gosec // static asset: world-readable is intentional
		return PrerenderedEntry{}, &PrerenderError{
			Route:   job.Route,
			Path:    job.Path,
			Status:  500,
			Message: fmt.Sprintf("write: %v", err),
		}
	}

	relFileFromOut := filepath.ToSlash(filepath.Join(relFile, "index.html"))
	return PrerenderedEntry{
		Route:     job.Route,
		Path:      job.Path,
		File:      relFileFromOut,
		Protected: job.Protected,
	}, nil
}

func writePrerenderManifest(outAbs string, res *PrerenderResult) (string, error) {
	manifest := struct {
		GeneratedAt time.Time          `json:"generatedAt"`
		Entries     []PrerenderedEntry `json:"entries"`
	}{
		GeneratedAt: res.GeneratedAt,
		Entries:     res.Entries,
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("server: marshal prerender manifest: %w", err)
	}
	body = append(body, '\n')
	target := filepath.Join(outAbs, PrerenderManifestFilename)
	if err := os.WriteFile(target, body, 0o644); err != nil { //nolint:gosec
		return "", fmt.Errorf("server: write prerender manifest: %w", err)
	}
	return target, nil
}

func writePrerenderReport(path string, errs []PrerenderError) error {
	body, err := json.MarshalIndent(errs, "", "  ")
	if err != nil {
		return fmt.Errorf("server: marshal prerender report: %w", err)
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("server: mkdir prerender report: %w", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil { //nolint:gosec
		return fmt.Errorf("server: write prerender report: %w", err)
	}
	return nil
}

func truncMessage(msg string) string {
	const max = 160
	msg = strings.ReplaceAll(strings.ReplaceAll(msg, "\n", " "), "\r", " ")
	if len(msg) <= max {
		return msg
	}
	return msg[:max] + "…"
}

// MaybePrerenderFromEnv inspects PrerenderTriggerEnv. When set, it
// runs Prerender against s using projectRoot, prints a one-line
// summary, and returns (true, error). When the env var is absent it
// returns (false, nil) so the caller can proceed with the normal
// ListenAndServe path. The intended call site is the user's main
// function, immediately after server.New:
//
//	if done, err := server.MaybePrerenderFromEnv(ctx, root, s); err != nil {
//	    log.Fatal(err)
//	} else if done {
//	    return
//	}
//	log.Fatal(s.ListenAndServe(":3000"))
func MaybePrerenderFromEnv(ctx context.Context, projectRoot string, s *Server) (bool, error) {
	if os.Getenv(PrerenderTriggerEnv) == "" {
		return false, nil
	}
	tol := 0
	if raw := os.Getenv(PrerenderTolerateEnv); raw != "" {
		if v, err := strconvAtoi(raw); err == nil {
			tol = v
		}
	}
	opts := PrerenderOptions{
		OutDir:     os.Getenv(PrerenderOutDirEnv),
		Tolerate:   tol,
		ReportPath: os.Getenv(PrerenderReportEnv),
	}
	res, err := s.Prerender(ctx, projectRoot, opts)
	if res != nil {
		fmt.Fprintf(os.Stderr, "prerender: wrote %d entries, %d errors\n", len(res.Entries), len(res.Errors))
		for _, e := range res.Errors {
			fmt.Fprintf(os.Stderr, "prerender error: %d %s — %s\n", e.Status, e.Path, truncMessage(e.Message))
		}
	}
	if err != nil {
		return true, err
	}
	return true, nil
}

// strconvAtoi is a tiny helper used by MaybePrerenderFromEnv that
// avoids importing strconv at the top of the file (kept local for
// readability — the import block stays minimal).
func strconvAtoi(s string) (int, error) {
	var n int
	sign := 1
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		if s[0] == '-' {
			sign = -1
		}
		s = s[1:]
	}
	if s == "" {
		return 0, errors.New("invalid int")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("invalid int")
		}
		n = n*10 + int(c-'0')
	}
	return n * sign, nil
}

// ensure kit package import is used.
var _ = kit.DefaultPageOptions
