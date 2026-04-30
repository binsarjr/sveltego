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
//
// Release, when true, activates production-build restrictions: imports of
// $lib/dev/** are rejected as fatal errors, mirroring sveltejs/kit#13078.
type BuildOptions struct {
	ProjectRoot string
	OutDir      string
	Verbose     bool
	Release     bool
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
	emittedLayouts := make(map[string]struct{})
	emittedErrors := make(map[string]struct{})
	pageHeads := make(map[string]bool)
	layoutHeads := make(map[string]bool)
	for _, route := range scan.Routes {
		if route.HasError {
			if _, done := emittedErrors[route.Dir]; !done {
				if err := emitErrorPage(opts.ProjectRoot, outDir, route.Dir, route.PackagePath, route.PackageName); err != nil {
					return nil, err
				}
				emittedErrors[route.Dir] = struct{}{}
			}
		}
		switch {
		case route.HasPage:
			refs, hasHead, err := emitPage(opts.ProjectRoot, outDir, modulePath, route, opts.Release)
			if err != nil {
				return nil, err
			}
			libRefs += refs
			if hasHead {
				pageHeads[route.PackagePath] = true
			}
			routeCount++
		case route.HasServer:
			if err := emitRESTRoute(opts.ProjectRoot, outDir, modulePath, route); err != nil {
				return nil, err
			}
			routeCount++
		}
		if route.HasPageServer {
			if err := emitMirrorAndWire(opts.ProjectRoot, outDir, modulePath, route); err != nil {
				return nil, err
			}
		}
		for i, layoutDir := range route.LayoutChain {
			if _, done := emittedLayouts[layoutDir]; done {
				continue
			}
			pkgPath := route.LayoutPackagePaths[i]
			pkgName := layoutPackageName(pkgPath)
			serverFile := ""
			if i < len(route.LayoutServerFiles) {
				serverFile = route.LayoutServerFiles[i]
			}
			hasHead, err := emitLayout(opts.ProjectRoot, outDir, layoutDir, pkgPath, pkgName, serverFile, opts.Release)
			if err != nil {
				return nil, err
			}
			if hasHead {
				layoutHeads[pkgPath] = true
			}
			if serverFile != "" {
				if err := emitLayoutMirrorAndWire(opts.ProjectRoot, outDir, modulePath, pkgPath, pkgName, serverFile); err != nil {
					return nil, err
				}
			}
			emittedLayouts[layoutDir] = struct{}{}
		}
	}
	libExists := dirExists(libDir)

	routeOptions, err := resolvePageOptions(scan)
	if err != nil {
		return nil, fmt.Errorf("codegen: resolve page options: %w", err)
	}
	manifestBytes, err := GenerateManifest(scan, ManifestOptions{
		PackageName:  "gen",
		ModulePath:   modulePath,
		GenRoot:      outDir,
		RouteOptions: routeOptions,
		PageHeads:    pageHeads,
		LayoutHeads:  layoutHeads,
	})
	if err != nil {
		return nil, fmt.Errorf("codegen: generate manifest: %w", err)
	}
	manifestPath := filepath.Join(outAbs, "manifest.gen.go")
	if err := os.WriteFile(manifestPath, manifestBytes, genFileMode); err != nil {
		return nil, fmt.Errorf("codegen: write manifest: %w", err)
	}

	hookSet, err := scanHooksServer(opts.ProjectRoot)
	if err != nil {
		return nil, err
	}
	if err := emitHooks(opts.ProjectRoot, outDir, modulePath, "gen", hookSet); err != nil {
		return nil, err
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
// first returned int is 1 when at least one $lib literal was rewritten
// in this file, 0 otherwise — the caller aggregates across routes to
// decide whether the missing-lib warning fires. The bool reports
// whether the page contributed a Head method (drives manifest
// HeadFn wiring). When release is true, any $lib/dev/** import is a
// fatal error.
func emitPage(projectRoot, outDir, modulePath string, route routescan.ScannedRoute, release bool) (int, bool, error) {
	pageName := "+page.svelte"
	if route.HasReset {
		pageName = "+page@" + route.ResetTarget + ".svelte"
	}
	pagePath := filepath.Join(route.Dir, pageName)
	src, err := os.ReadFile(pagePath) //nolint:gosec // path comes from scanner walk under projectRoot
	if err != nil {
		return 0, false, fmt.Errorf("codegen: read %s: %w", pagePath, err)
	}
	if release {
		if err := checkLibDevImports(string(src), pagePath); err != nil {
			return 0, false, err
		}
	}
	rewritten, hits := rewriteLibImports(string(src), modulePath)
	src = []byte(rewritten)

	frag, perrs := parser.Parse(src)
	if len(perrs) > 0 {
		return 0, false, fmt.Errorf("codegen: parse %s: %w", pagePath, perrs)
	}

	opts := Options{PackageName: route.PackageName}
	if route.HasPageServer {
		opts.ServerFilePath = filepath.Join(route.Dir, "page.server.go")
		actionInfo, err := scanActions(opts.ServerFilePath)
		if err != nil {
			return 0, false, err
		}
		opts.HasActions = actionInfo.HasActions
	}
	out, err := Generate(frag, opts)
	if err != nil {
		return 0, false, fmt.Errorf("codegen: generate %s: %w", pagePath, err)
	}

	hasHead, _ := extractHeadChildren(frag.Children)

	relPkg := strings.TrimPrefix(route.PackagePath, ".gen/")
	target := filepath.Join(projectRoot, outDir, filepath.FromSlash(relPkg), "page.gen.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, false, fmt.Errorf("codegen: mkdir %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, out, genFileMode); err != nil {
		return 0, false, fmt.Errorf("codegen: write %s: %w", target, err)
	}
	libRefs := 0
	if hits {
		libRefs = 1
	}
	return libRefs, len(hasHead) > 0, nil
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

// emitErrorPage parses one +error.svelte and writes the generated
// error.gen.go into the route's encoded gen package directory. The
// package may also host a layout.gen.go and/or page.gen.go from the
// same directory; the distinct filename keeps them separate.
func emitErrorPage(projectRoot, outDir, errorDir, pkgPath, pkgName string) error {
	errPath := filepath.Join(errorDir, "+error.svelte")
	src, err := os.ReadFile(errPath) //nolint:gosec // path comes from scanner walk under projectRoot
	if err != nil {
		return fmt.Errorf("codegen: read %s: %w", errPath, err)
	}
	frag, perrs := parser.Parse(src)
	if len(perrs) > 0 {
		return fmt.Errorf("codegen: parse %s: %w", errPath, perrs)
	}
	out, err := GenerateErrorPage(frag, ErrorPageOptions{PackageName: pkgName})
	if err != nil {
		return fmt.Errorf("codegen: generate %s: %w", errPath, err)
	}
	relPkg := strings.TrimPrefix(pkgPath, ".gen/")
	target := filepath.Join(projectRoot, outDir, filepath.FromSlash(relPkg), "error.gen.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("codegen: mkdir %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, out, genFileMode); err != nil {
		return fmt.Errorf("codegen: write %s: %w", target, err)
	}
	return nil
}

// emitLayout parses one +layout.svelte and writes the generated
// layout.gen.go into the encoded layout package directory. Layout files
// share the directory with any +page.svelte / wire.gen.go for the same
// dir; the distinct filename keeps them in separate generated artifacts.
// The leading character must not be "_" because Go's build system
// silently ignores files whose name starts with "_". serverFile, when
// non-empty, points at a sibling layout.server.go whose Load() inline
// struct return is used to infer LayoutData fields. When release is true,
// any $lib/dev/** import is a fatal error.
func emitLayout(projectRoot, outDir, layoutDir, pkgPath, pkgName, serverFile string, release bool) (bool, error) {
	layoutPath, err := resolveLayoutSource(layoutDir)
	if err != nil {
		return false, err
	}
	src, err := os.ReadFile(layoutPath) //nolint:gosec // path comes from scanner walk under projectRoot
	if err != nil {
		return false, fmt.Errorf("codegen: read %s: %w", layoutPath, err)
	}
	if release {
		if err := checkLibDevImports(string(src), layoutPath); err != nil {
			return false, err
		}
	}
	frag, perrs := parser.Parse(src)
	if len(perrs) > 0 {
		return false, fmt.Errorf("codegen: parse %s: %w", layoutPath, perrs)
	}
	out, err := GenerateLayout(frag, LayoutOptions{PackageName: pkgName, ServerFilePath: serverFile})
	if err != nil {
		return false, fmt.Errorf("codegen: generate %s: %w", layoutPath, err)
	}
	hasHead, _ := extractHeadChildren(frag.Children)
	relPkg := strings.TrimPrefix(pkgPath, ".gen/")
	target := filepath.Join(projectRoot, outDir, filepath.FromSlash(relPkg), "layout.gen.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return false, fmt.Errorf("codegen: mkdir %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, out, genFileMode); err != nil {
		return false, fmt.Errorf("codegen: write %s: %w", target, err)
	}
	return len(hasHead) > 0, nil
}

// emitLayoutMirrorAndWire writes the user-source mirror for one layout
// server file and the adjacent wire_layout.gen.go that the manifest
// references. The mirror lives at <projectRoot>/<outDir>/layoutsrc/
// <encodedSubpath>/layout_server.go with the build constraint stripped
// and the package clause rewritten to <encodedPackageName>. The wire
// file lives at <projectRoot>/<outDir>/<encodedSubpath>/
// wire_layout.gen.go and re-exports Load wrapped to satisfy
// router.LayoutLoadHandler.
func emitLayoutMirrorAndWire(projectRoot, outDir, modulePath, pkgPath, pkgName, serverFile string) error {
	encodedSub := strings.TrimPrefix(pkgPath, ".gen/")

	usf := userSourceFile{
		UserPath:    serverFile,
		MirrorPath:  filepath.Join(projectRoot, outDir, "layoutsrc", filepath.FromSlash(encodedSub), "layout_server.go"),
		PackageName: pkgName,
	}
	if err := mirrorUserSource(&usf); err != nil {
		return err
	}

	wireDir := filepath.Join(projectRoot, outDir, filepath.FromSlash(encodedSub))
	return emitLayoutWire(outDir, modulePath, mirrorRoute{
		encodedSubpath: encodedSub,
		packageName:    pkgName,
		wireDir:        wireDir,
	})
}

// resolveLayoutSource returns the path of the +layout.svelte (or its
// reset variant) inside layoutDir. The plain filename takes precedence;
// otherwise the first matching `+layout@*.svelte` entry wins. The
// scanner already guarantees the directory contains exactly one
// layout source, so the search is unambiguous.
func resolveLayoutSource(layoutDir string) (string, error) {
	plain := filepath.Join(layoutDir, "+layout.svelte")
	if _, err := os.Stat(plain); err == nil {
		return plain, nil
	}
	entries, err := os.ReadDir(layoutDir)
	if err != nil {
		return "", fmt.Errorf("codegen: read %s: %w", layoutDir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		base, _, ok := routescan.ParseResetFilename(e.Name())
		if !ok || base != "+layout" {
			continue
		}
		return filepath.Join(layoutDir, e.Name()), nil
	}
	return "", fmt.Errorf("codegen: %s contains no +layout.svelte", layoutDir)
}

// layoutPackageName extracts the directory's package name from a
// .gen/routes/... package path. Mirrors how routescan derives PackageName.
func layoutPackageName(pkgPath string) string {
	rel := strings.TrimPrefix(pkgPath, ".gen/")
	if rel == "" || rel == "routes" {
		return "routes"
	}
	parts := strings.Split(rel, "/")
	return parts[len(parts)-1]
}
