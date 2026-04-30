package codegen

import (
	"bufio"
	"errors"
	"fmt"
	"go/format"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/binsarjr/sveltego/internal/parser"
	"github.com/binsarjr/sveltego/internal/routescan"
)

// log attribute keys for sloglint compliance.
const (
	logKeyDiagnostic = "diagnostic"
	logKeyRoutes     = "routes"
	logKeyManifest   = "manifest"
	logKeyModule     = "module"
	logKeyElapsed    = "elapsed"
)

// genFileMode is the permission applied to every file written under
// OutDir. 0o600 satisfies gosec G306; the gen tree is consumed by the
// owning user's `go build` and never needs world-readable bits.
const genFileMode = 0o600

// BuildOptions configures [Build]. ProjectRoot must be an absolute path
// that contains a go.mod file and a src/routes/ directory; OutDir is the
// gen-output root relative to ProjectRoot and defaults to ".gen".
type BuildOptions struct {
	ProjectRoot string
	OutDir      string
	Verbose     bool
	Logger      *slog.Logger
}

// BuildResult summarizes one [Build] invocation. Routes counts every
// emitted page or server stub; ManifestPath is the absolute path to the
// generated manifest. Diagnostics holds non-fatal scanner warnings — fatal
// diagnostics return through the error channel instead.
type BuildResult struct {
	Routes       int
	ManifestPath string
	Diagnostics  []routescan.Diagnostic
	Elapsed      time.Duration
}

// Build orchestrates per-project codegen: it wipes OutDir, scans the
// routes tree, and emits one Go file per discovered route plus a
// cross-route manifest and an embed.go stub. The user's go.mod module
// path is read once and used to resolve `$lib` import literals in
// hoisted <script lang="go"> blocks.
func Build(opts BuildOptions) (*BuildResult, error) {
	start := time.Now()
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	if !filepath.IsAbs(opts.ProjectRoot) {
		return nil, fmt.Errorf("codegen: ProjectRoot must be absolute (got %q)", opts.ProjectRoot)
	}
	goModPath := filepath.Join(opts.ProjectRoot, "go.mod")
	if _, err := os.Stat(goModPath); err != nil {
		return nil, fmt.Errorf("codegen: go.mod not found at %s: %w", goModPath, err)
	}
	routesDir := filepath.Join(opts.ProjectRoot, "src", "routes")
	if info, err := os.Stat(routesDir); err != nil {
		return nil, fmt.Errorf("codegen: src/routes/ not found at %s: %w", routesDir, err)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("codegen: %s is not a directory", routesDir)
	}

	modulePath, err := readModulePath(goModPath)
	if err != nil {
		return nil, err
	}

	outDir := opts.OutDir
	if outDir == "" {
		outDir = ".gen"
	}
	outAbs := filepath.Join(opts.ProjectRoot, outDir)
	if err := os.RemoveAll(outAbs); err != nil {
		return nil, fmt.Errorf("codegen: clean %s: %w", outAbs, err)
	}
	if err := os.MkdirAll(outAbs, 0o755); err != nil {
		return nil, fmt.Errorf("codegen: mkdir %s: %w", outAbs, err)
	}

	paramsDir := filepath.Join(opts.ProjectRoot, "src", "params")
	if _, err := os.Stat(paramsDir); err != nil {
		paramsDir = ""
	}
	scan, err := routescan.Scan(routescan.ScanInput{RoutesDir: routesDir, ParamsDir: paramsDir})
	if err != nil {
		return nil, fmt.Errorf("codegen: scan routes: %w", err)
	}

	fatal, warnings := splitDiagnostics(scan.Diagnostics)
	if len(fatal) > 0 {
		return nil, fatalDiagnosticsError(fatal)
	}
	for _, d := range warnings {
		logger.Warn("routescan diagnostic", logKeyDiagnostic, d.String())
	}

	libDir := filepath.Join(opts.ProjectRoot, "lib")
	libRefs := 0
	routeCount := 0
	for _, route := range scan.Routes {
		switch {
		case route.HasPage:
			refs, err := emitPage(opts.ProjectRoot, outDir, modulePath, route)
			if err != nil {
				return nil, err
			}
			libRefs += refs
			routeCount++
		case route.HasServer:
			if err := emitServerStub(opts.ProjectRoot, outDir, route); err != nil {
				return nil, err
			}
			routeCount++
		}
		if route.HasPageServer {
			if err := emitMirrorAndWire(opts.ProjectRoot, outDir, modulePath, route); err != nil {
				return nil, err
			}
		}
	}
	libExists := dirExists(libDir)

	manifestBytes, err := GenerateManifest(scan, ManifestOptions{
		PackageName: "gen",
		ModulePath:  modulePath,
		GenRoot:     outDir,
	})
	if err != nil {
		return nil, fmt.Errorf("codegen: generate manifest: %w", err)
	}
	manifestPath := filepath.Join(outAbs, "manifest.gen.go")
	if err := os.WriteFile(manifestPath, manifestBytes, genFileMode); err != nil {
		return nil, fmt.Errorf("codegen: write manifest: %w", err)
	}

	if err := emitEmbedStub(opts.ProjectRoot, outAbs); err != nil {
		return nil, err
	}

	if libRefs > 0 && !libExists {
		warnings = append(warnings, routescan.Diagnostic{
			Path:    libDir,
			Message: fmt.Sprintf("$lib referenced %d time(s) but %s/ does not exist", libRefs, filepath.Base(libDir)),
			Hint:    "create lib/ at the project root for shared modules",
		})
	}

	if opts.Verbose {
		logger.Info("codegen done",
			logKeyRoutes, routeCount,
			logKeyManifest, manifestPath,
			logKeyModule, modulePath,
			logKeyElapsed, time.Since(start),
		)
	}

	return &BuildResult{
		Routes:       routeCount,
		ManifestPath: manifestPath,
		Diagnostics:  warnings,
		Elapsed:      time.Since(start),
	}, nil
}

// emitPage parses one +page.svelte, applies $lib import rewriting on the
// hoisted script body, runs the generator, and writes the result. The
// returned count is 1 when at least one $lib literal was rewritten in
// this file, 0 otherwise — the caller aggregates across routes to
// decide whether the missing-lib warning fires.
func emitPage(projectRoot, outDir, modulePath string, route routescan.ScannedRoute) (int, error) {
	pagePath := filepath.Join(route.Dir, "+page.svelte")
	src, err := os.ReadFile(pagePath) //nolint:gosec // path comes from scanner walk under projectRoot
	if err != nil {
		return 0, fmt.Errorf("codegen: read %s: %w", pagePath, err)
	}
	rewritten, hits := rewriteLibImports(string(src), modulePath)
	src = []byte(rewritten)

	frag, perrs := parser.Parse(src)
	if len(perrs) > 0 {
		return 0, fmt.Errorf("codegen: parse %s: %w", pagePath, perrs)
	}

	opts := Options{PackageName: route.PackageName}
	if route.HasPageServer {
		opts.ServerFilePath = filepath.Join(route.Dir, "page.server.go")
	}
	out, err := Generate(frag, opts)
	if err != nil {
		return 0, fmt.Errorf("codegen: generate %s: %w", pagePath, err)
	}

	relPkg := strings.TrimPrefix(route.PackagePath, ".gen/")
	target := filepath.Join(projectRoot, outDir, filepath.FromSlash(relPkg), "page.gen.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, fmt.Errorf("codegen: mkdir %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, out, genFileMode); err != nil {
		return 0, fmt.Errorf("codegen: write %s: %w", target, err)
	}
	if hits {
		return 1, nil
	}
	return 0, nil
}

// emitServerStub writes a placeholder file documenting the +server.go
// re-export point. Phase 0i ships pages only; Phase 0j wires the real
// re-export.
func emitServerStub(projectRoot, outDir string, route routescan.ScannedRoute) error {
	relPkg := strings.TrimPrefix(route.PackagePath, ".gen/")
	dir := filepath.Join(projectRoot, outDir, filepath.FromSlash(relPkg))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("codegen: mkdir %s: %w", dir, err)
	}
	body := fmt.Sprintf("// Code generated by sveltego. DO NOT EDIT.\npackage %s\n\n// +server.go user package re-export TODO Phase 0j.\n", route.PackageName)
	formatted, err := format.Source([]byte(body))
	if err != nil {
		return fmt.Errorf("codegen: format server stub: %w", err)
	}
	target := filepath.Join(dir, "server.gen.go")
	if err := os.WriteFile(target, formatted, genFileMode); err != nil {
		return fmt.Errorf("codegen: write %s: %w", target, err)
	}
	return nil
}

// emitEmbedStub writes <outAbs>/embed.go only when at least one of the
// expected `client/` or `static/` subdirectories sits under outAbs. The
// `go:embed` directive resolves paths relative to the file containing
// it, so both targets must live next to embed.go itself; project-root
// `static/` is staged into outAbs by the asset pipeline before this
// runs (Phase 0j onwards). Until that stage exists, projects without a
// client bundle skip embed.go entirely and the consuming binary does
// not declare ClientFS.
func emitEmbedStub(_, outAbs string) error {
	hasClient := dirExists(filepath.Join(outAbs, "client"))
	hasStatic := dirExists(filepath.Join(outAbs, "static"))
	if !hasClient && !hasStatic {
		return nil
	}
	var dirs []string
	if hasClient {
		dirs = append(dirs, "all:client")
	}
	if hasStatic {
		dirs = append(dirs, "all:static")
	}
	body := "// Code generated by sveltego. DO NOT EDIT.\npackage gen\n\nimport \"embed\"\n\n//go:embed " + strings.Join(dirs, " ") + "\nvar ClientFS embed.FS\n"
	formatted, err := format.Source([]byte(body))
	if err != nil {
		return fmt.Errorf("codegen: format embed stub: %w", err)
	}
	target := filepath.Join(outAbs, "embed.go")
	if err := os.WriteFile(target, formatted, genFileMode); err != nil {
		return fmt.Errorf("codegen: write embed.go: %w", err)
	}
	return nil
}

// readModulePath scans a go.mod file for the first non-blank,
// non-comment line beginning with `module ` and returns the path token.
// The implementation is stdlib-only; modfile-grade parsing is overkill
// for the MVP and adds a dependency.
func readModulePath(goModPath string) (string, error) {
	f, err := os.Open(goModPath) //nolint:gosec // path is caller-supplied
	if err != nil {
		return "", fmt.Errorf("codegen: open go.mod: %w", err)
	}
	defer f.Close()

	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if !strings.HasPrefix(line, "module ") && !strings.HasPrefix(line, "module\t") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "module"))
		rest = strings.TrimSpace(strings.Trim(rest, "\""))
		if rest == "" {
			return "", fmt.Errorf("codegen: %s: empty module path", goModPath)
		}
		return rest, nil
	}
	if err := scan.Err(); err != nil {
		return "", fmt.Errorf("codegen: read go.mod: %w", err)
	}
	return "", fmt.Errorf("codegen: %s: no module declaration", goModPath)
}

// splitDiagnostics partitions scanner diagnostics. Conflicts, orphans,
// and unknown matchers are fatal because they would yield a manifest
// that cannot compile or routes that never match. Everything else
// (currently misplaced hooks.server.go) is a warning.
func splitDiagnostics(ds []routescan.Diagnostic) (fatal, warnings []routescan.Diagnostic) {
	for _, d := range ds {
		if isFatalDiagnostic(d) {
			fatal = append(fatal, d)
			continue
		}
		warnings = append(warnings, d)
	}
	return fatal, warnings
}

func isFatalDiagnostic(d routescan.Diagnostic) bool {
	msg := d.Message
	switch {
	case strings.Contains(msg, "route conflict"):
		return true
	case strings.Contains(msg, "orphan page.server.go"):
		return true
	case strings.Contains(msg, "unknown matcher"):
		return true
	case strings.Contains(msg, "may not have both +page.svelte and server.go"):
		return true
	}
	return false
}

func fatalDiagnosticsError(fatal []routescan.Diagnostic) error {
	errs := make([]error, 0, len(fatal))
	for _, d := range fatal {
		errs = append(errs, errors.New(d.String()))
	}
	return fmt.Errorf("codegen: fatal scanner diagnostics:\n%w", errors.Join(errs...))
}

// emitMirrorAndWire writes the user-source mirror for one route and the
// adjacent wire.gen.go that the manifest references. The mirror lives at
// <projectRoot>/<outDir>/usersrc/<encodedSubpath>/<basename> and has its
// `//go:build sveltego` constraint stripped plus its package clause
// rewritten to <encodedPackageName>. The wire file lives at
// <projectRoot>/<outDir>/<encodedSubpath>/wire.gen.go and re-exports
// Load (always) and Actions (when the user file declares it) wrapped to
// satisfy router.LoadHandler / router.ActionsHandler.
func emitMirrorAndWire(projectRoot, outDir, modulePath string, route routescan.ScannedRoute) error {
	encodedSub := strings.TrimPrefix(route.PackagePath, ".gen/")

	usf := userSourceFile{
		UserPath:    filepath.Join(route.Dir, "page.server.go"),
		MirrorPath:  filepath.Join(projectRoot, outDir, "usersrc", filepath.FromSlash(encodedSub), "page_server.go"),
		PackageName: route.PackageName,
	}
	if err := mirrorUserSource(&usf); err != nil {
		return err
	}

	wireDir := filepath.Join(projectRoot, outDir, filepath.FromSlash(encodedSub))
	return emitWire(outDir, modulePath, mirrorRoute{
		encodedSubpath: encodedSub,
		packageName:    route.PackageName,
		wireDir:        wireDir,
		hasActions:     usf.HasActions,
	})
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
