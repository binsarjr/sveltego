package adapterstatic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
)

// subprocessRunner is the default Runner. It compiles the user's main
// package to a throwaway binary and runs it with SVELTEGO_PRERENDER=1
// so the in-binary MaybePrerenderFromEnv hook drives Server.Prerender
// against the configured scratch dir.
//
// Tests do not exercise this runner directly: spawning a real Go build
// inside a unit test is slow and fragile. Tests inject an in-process
// Runner that calls Server.Prerender directly. The CLI integration
// path uses this runner in production.
type subprocessRunner struct {
	MainPackage string
	Stdout      io.Writer
	Stderr      io.Writer

	// Tolerate mirrors `sveltego prerender --tolerate`. Default 0
	// (fail on first error). -1 absorbs every error.
	Tolerate int
}

func (r *subprocessRunner) Prerender(ctx context.Context, projectRoot, scratchDir string) (RunInfo, error) {
	if err := ctx.Err(); err != nil {
		return RunInfo{}, err
	}
	if _, err := exec.LookPath("go"); err != nil {
		return RunInfo{}, fmt.Errorf("adapter-static: `go` toolchain not on PATH: %w", err)
	}

	tmpBin := filepath.Join(scratchDir, "_prerender.bin")
	if err := os.MkdirAll(filepath.Dir(tmpBin), 0o755); err != nil {
		return RunInfo{}, fmt.Errorf("adapter-static: mkdir scratch bin dir: %w", err)
	}
	defer os.Remove(tmpBin) //nolint:errcheck // best-effort

	build := exec.CommandContext(ctx, "go", "build", "-o", tmpBin, r.MainPackage) //nolint:gosec // args are adapter-controlled
	build.Dir = projectRoot
	build.Stdout = r.Stdout
	build.Stderr = r.Stderr
	if err := build.Run(); err != nil {
		return RunInfo{}, fmt.Errorf("adapter-static: go build %s: %w", r.MainPackage, err)
	}

	run := exec.CommandContext(ctx, tmpBin) //nolint:gosec // path is adapter-controlled
	run.Dir = projectRoot
	run.Stdout = r.Stdout
	run.Stderr = r.Stderr
	env := os.Environ()
	env = append(env,
		"SVELTEGO_PRERENDER=1",
		"SVELTEGO_PRERENDER_OUT="+scratchDir,
		"SVELTEGO_PRERENDER_TOLERATE="+strconv.Itoa(r.Tolerate),
	)
	run.Env = env
	if err := run.Run(); err != nil {
		return RunInfo{}, fmt.Errorf("adapter-static: prerender run: %w", err)
	}

	manifestPath := filepath.Join(scratchDir, "manifest.json")
	body, err := os.ReadFile(manifestPath) //nolint:gosec // path is adapter-controlled
	if err != nil {
		return RunInfo{}, fmt.Errorf("adapter-static: read scratch manifest: %w", err)
	}
	var m struct {
		Entries []struct {
			Route string `json:"route"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return RunInfo{}, fmt.Errorf("adapter-static: parse scratch manifest: %w", err)
	}
	seen := make(map[string]struct{}, len(m.Entries))
	prerendered := make([]string, 0, len(m.Entries))
	for _, e := range m.Entries {
		if _, ok := seen[e.Route]; ok {
			continue
		}
		seen[e.Route] = struct{}{}
		prerendered = append(prerendered, e.Route)
	}
	sort.Strings(prerendered)

	// The subprocess runner does not have direct access to the user
	// binary's full route table, so it cannot enumerate dynamic routes
	// here. FailOnDynamic via the subprocess path is therefore a no-op
	// today — see the package README for the in-process workaround.
	return RunInfo{PrerenderedRoutes: prerendered}, nil
}

// compile-time assertion: keep the runner satisfying the Runner contract.
var _ Runner = (*subprocessRunner)(nil)
