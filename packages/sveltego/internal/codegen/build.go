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
	"github.com/binsarjr/sveltego/internal/vite"
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
//
// EnvLookup is called for each env.StaticPublic("X") call found in .svelte
// sources during codegen. The call is replaced with the Go string literal
// for the returned value, baking it into the binary at build time. A
// missing key is a fatal build error. When nil, os.LookupEnv is used.
//
// Provenance controls per-span // gen: comments in emitted files (default
// true). The file-level banner is always emitted regardless of this flag.
// Pass --no-provenance on the CLI to set Provenance: false.
type BuildOptions struct {
	ProjectRoot string
	OutDir      string
	Verbose     bool
	Release     bool
	Logger      *slog.Logger
	EnvLookup   EnvLookup
	// Provenance enables per-span // gen: source=<path>:<line> kind=<kind>
	// comments in generated files. The file-level DO NOT EDIT banner is
	// always emitted. Default true; set false with --no-provenance.
	Provenance bool
	// NoClient skips emitting Vite client entry files and vite.config.gen.js.
	NoClient bool
}

// BuildResult summarizes one [Build] invocation. Routes counts every
// emitted page or server stub; ManifestPath is the absolute path to the
// generated manifest. ViteConfigPath is the path to vite.config.gen.js
// (empty when NoClient is true or no page routes exist). ClientRouteKeys
// contains the Vite input key for each page route. Diagnostics holds
// non-fatal scanner warnings; fatal diagnostics are returned as errors.
type BuildResult struct {
	Routes          int
	ManifestPath    string
	ViteConfigPath  string
	ClientRouteKeys []string
	Diagnostics     []routescan.Diagnostic
	Elapsed         time.Duration
	// HasServiceWorker reports whether src/service-worker.ts was detected
	// at the project root. Drives both the Vite config (extra Rollup
	// input) and the generated manifest's HasServiceWorker constant the
	// runtime reads to gate the registration script (#89).
	HasServiceWorker bool
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
	if opts.EnvLookup == nil {
		opts.EnvLookup = os.LookupEnv
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
	// componentSeeds collects every .svelte file processed by the route/layout
	// passes so the component discovery pass can scan them for relative imports.
	var componentSeeds []string
	// clientRouteKeys collects the Vite input key for every page route.
	var clientRouteKeys []string
	clientKeysByPkg := make(map[string]string)
	// clientRouterMap maps each route's canonical pattern to the path of
	// its +page.svelte source relative to the SPA router module
	// (.gen/client/__router/router.ts). It feeds vite.GenerateRouter so
	// the SPA router can lazy-import the right component per navigation.
	clientRouterMap := make(map[string]string)
	// clientSnapshotRoutes records which patterns export a Snapshot from
	// `<script module>` so vite.GenerateRouter wires the snapshot capture
	// + restore hooks (#84). Empty when no route opts in.
	clientSnapshotRoutes := make(map[string]bool)
	for _, route := range scan.Routes {
		if route.HasError {
			if _, done := emittedErrors[route.Dir]; !done {
				if err := emitErrorPage(opts.ProjectRoot, outDir, route.Dir, route.PackagePath, route.PackageName, opts.Provenance, start); err != nil {
					return nil, err
				}
				emittedErrors[route.Dir] = struct{}{}
			}
		}
		switch {
		case route.HasPage:
			refs, hasHead, hasSnapshot, err := emitPage(opts.ProjectRoot, outDir, modulePath, route, opts.Release, opts.EnvLookup, opts.Provenance, start)
			if err != nil {
				return nil, err
			}
			libRefs += refs
			if hasHead {
				pageHeads[route.PackagePath] = true
			}
			routeCount++
			pageName := "+page.svelte"
			if route.HasReset {
				pageName = "+page@" + route.ResetTarget + ".svelte"
			}
			componentSeeds = append(componentSeeds, filepath.Join(route.Dir, pageName))

			if !opts.NoClient {
				ck, relSvelteFromRouter, cerr := emitClientEntry(opts.ProjectRoot, outDir, routesDir, route, pageName, hasSnapshot)
				if cerr != nil {
					return nil, cerr
				}
				clientRouteKeys = append(clientRouteKeys, ck)
				clientKeysByPkg[route.PackagePath] = ck
				clientRouterMap[route.Pattern] = relSvelteFromRouter
				if hasSnapshot {
					clientSnapshotRoutes[route.Pattern] = true
				}
			}
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
			hasHead, err := emitLayout(opts.ProjectRoot, outDir, layoutDir, pkgPath, pkgName, serverFile, opts.Release, opts.EnvLookup, opts.Provenance, start)
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
			layoutPath, lerr := resolveLayoutSource(layoutDir)
			if lerr == nil {
				componentSeeds = append(componentSeeds, layoutPath)
			}
		}
	}
	libExists := dirExists(libDir)

	if _, err := emitComponentTree(opts.ProjectRoot, outDir, componentSeeds); err != nil {
		return nil, fmt.Errorf("codegen: component tree: %w", err)
	}

	routeOptions, err := resolvePageOptions(scan)
	if err != nil {
		return nil, fmt.Errorf("codegen: resolve page options: %w", err)
	}
	localsDiags, err := collectLocalsPrerenderWarnings(scan, routeOptions)
	if err != nil {
		return nil, fmt.Errorf("codegen: locals/prerender scan: %w", err)
	}
	warnings = append(warnings, localsDiags...)
	hasServiceWorker := serviceWorkerEntry(opts.ProjectRoot) != ""
	manifestBytes, err := GenerateManifest(scan, ManifestOptions{
		PackageName:      "gen",
		ModulePath:       modulePath,
		GenRoot:          outDir,
		RouteOptions:     routeOptions,
		PageHeads:        pageHeads,
		LayoutHeads:      layoutHeads,
		ClientKeys:       clientKeysByPkg,
		HasServiceWorker: hasServiceWorker,
	})
	if err != nil {
		return nil, fmt.Errorf("codegen: generate manifest: %w", err)
	}
	manifestPath := filepath.Join(outAbs, "manifest.gen.go")
	if err := os.WriteFile(manifestPath, manifestBytes, genFileMode); err != nil {
		return nil, fmt.Errorf("codegen: write manifest: %w", err)
	}

	if err := emitAssetsManifest(opts.ProjectRoot, outDir, "gen"); err != nil {
		return nil, err
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

	var viteConfigPath string
	swEntry := ""
	if !opts.NoClient && hasServiceWorker {
		swEntry = "src/service-worker.ts"
	}
	if !opts.NoClient && len(clientRouteKeys) > 0 {
		if err := emitClientRouter(opts.ProjectRoot, outDir, clientRouterMap, clientSnapshotRoutes); err != nil {
			return nil, err
		}
		addons, derr := detectAddons(opts.ProjectRoot)
		if derr != nil {
			return nil, derr
		}
		var cssEntry string
		if len(addons) > 0 {
			cssEntry = resolveCSSEntry(opts.ProjectRoot)
		}
		viteConfigPath = filepath.Join(opts.ProjectRoot, "vite.config.gen.js")
		configSrc := vite.GenerateConfig(vite.ConfigOptions{
			OutDir:             "static/_app",
			RouteKeys:          clientRouteKeys,
			GenClientDir:       filepath.Join(outDir, "client"),
			Addons:             addons,
			CSSEntry:           cssEntry,
			ServiceWorkerEntry: swEntry,
		})
		if werr := os.WriteFile(viteConfigPath, []byte(configSrc), 0o644); werr != nil { //nolint:gosec // world-readable JS config is intentional
			return nil, fmt.Errorf("codegen: write vite.config.gen.js: %w", werr)
		}
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
		Routes:           routeCount,
		ManifestPath:     manifestPath,
		ViteConfigPath:   viteConfigPath,
		ClientRouteKeys:  clientRouteKeys,
		Diagnostics:      warnings,
		Elapsed:          time.Since(start),
		HasServiceWorker: hasServiceWorker,
	}, nil
}

// serviceWorkerEntry returns the absolute path to src/service-worker.ts
// when present at the project root, or "" when no service worker is
// declared. The detection is a single os.Stat — no parsing — so the
// only contract is the file's existence.
func serviceWorkerEntry(projectRoot string) string {
	path := filepath.Join(projectRoot, "src", "service-worker.ts")
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path
	}
	return ""
}

// emitPage parses one +page.svelte, applies $lib import rewriting on the
// hoisted script body, runs the generator, and writes the result. The
// first returned int is 1 when at least one $lib literal was rewritten
// in this file, 0 otherwise — the caller aggregates across routes to
// decide whether the missing-lib warning fires. The bool reports
// whether the page contributed a Head method (drives manifest
// HeadFn wiring). When release is true, any $lib/dev/** import is a
// fatal error. lookup is used to resolve env.Static* calls to literal
// values at build time.
func emitPage(projectRoot, outDir, modulePath string, route routescan.ScannedRoute, release bool, lookup EnvLookup, provenance bool, generatedAt time.Time) (int, bool, bool, error) {
	pageName := "+page.svelte"
	if route.HasReset {
		pageName = "+page@" + route.ResetTarget + ".svelte"
	}
	pagePath := filepath.Join(route.Dir, pageName)
	src, err := os.ReadFile(pagePath) //nolint:gosec // path comes from scanner walk under projectRoot
	if err != nil {
		return 0, false, false, fmt.Errorf("codegen: read %s: %w", pagePath, err)
	}
	if release {
		if err := checkLibDevImports(string(src), pagePath); err != nil {
			return 0, false, false, err
		}
	}
	srcForHash := make([]byte, len(src)) // capture before rewrite for deterministic hash
	copy(srcForHash, src)
	substituted, err := substituteStaticEnv(string(src), lookup)
	if err != nil {
		return 0, false, false, fmt.Errorf("codegen: %s: %w", pagePath, err)
	}
	rewritten, hits := rewriteLibImports(substituted, modulePath)
	src = []byte(rewritten)

	frag, perrs := parser.Parse(src)
	if len(perrs) > 0 {
		return 0, false, false, fmt.Errorf("codegen: parse %s: %w", pagePath, perrs)
	}

	opts := Options{
		PackageName:   route.PackageName,
		Filename:      pagePath,
		Provenance:    provenance,
		SourceContent: srcForHash,
		GeneratedAt:   generatedAt,
	}
	if route.HasPageServer {
		opts.ServerFilePath = filepath.Join(route.Dir, "page.server.go")
		actionInfo, err := scanActions(opts.ServerFilePath)
		if err != nil {
			return 0, false, false, err
		}
		opts.HasActions = actionInfo.HasActions
	}
	out, err := Generate(frag, opts)
	if err != nil {
		return 0, false, false, fmt.Errorf("codegen: generate %s: %w", pagePath, err)
	}

	hasHead, _ := extractHeadChildren(frag.Children)
	hasSnapshot := detectFragmentSnapshot(frag)

	relPkg := strings.TrimPrefix(route.PackagePath, ".gen/")
	target := filepath.Join(projectRoot, outDir, filepath.FromSlash(relPkg), "page.gen.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, false, false, fmt.Errorf("codegen: mkdir %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, out, genFileMode); err != nil {
		return 0, false, false, fmt.Errorf("codegen: write %s: %w", target, err)
	}
	libRefs := 0
	if hits {
		libRefs = 1
	}
	return libRefs, len(hasHead) > 0, hasSnapshot, nil
}

// emitClientEntry writes .gen/client/<routeKey>/entry.ts for one page route.
// routeKey is e.g. "routes/+page" or "routes/post/[id]/+page". Returns
// the routeKey, the .svelte source path relative to the SPA router
// directory (.gen/client/__router/), and any error. The caller collects
// the routeKey for vite.GenerateConfig and the relative source path for
// vite.GenerateRouter so the per-app router module imports each
// component lazily without re-deriving the path. hasSnapshot wires the
// snapshot capture/restore hooks into the initial-mount path so a
// reload that lands on a route with persisted state restores it.
func emitClientEntry(projectRoot, outDir, routesDir string, route routescan.ScannedRoute, pageName string, hasSnapshot bool) (string, string, error) {
	routesParent := filepath.Dir(routesDir)
	relDir, err := filepath.Rel(routesParent, route.Dir)
	if err != nil {
		return "", "", fmt.Errorf("codegen: client entry rel path: %w", err)
	}
	routeKey := filepath.ToSlash(filepath.Join(relDir, strings.TrimSuffix(pageName, ".svelte")))

	entryDir := filepath.Join(projectRoot, outDir, "client", filepath.FromSlash(routeKey))
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		return "", "", fmt.Errorf("codegen: mkdir client entry %s: %w", entryDir, err)
	}

	entryRelFromRoot := filepath.ToSlash(filepath.Join(outDir, "client", routeKey, "entry.ts"))
	depth := len(strings.Split(filepath.ToSlash(filepath.Dir(entryRelFromRoot)), "/"))
	routeDirFromRoot, err := filepath.Rel(projectRoot, route.Dir)
	if err != nil {
		return "", "", fmt.Errorf("codegen: client entry svelte rel path: %w", err)
	}
	svelteSrc := filepath.ToSlash(routeDirFromRoot) + "/" + pageName
	relSvelte := vite.RelativeSveltePath(svelteSrc, depth)

	// The per-route entry sits at .gen/client/<routeKey>/entry.ts; the
	// shared router module lives at .gen/client/__router/router.ts. The
	// relative path from the entry's directory walks up to .gen/client/
	// and into __router/router.
	relRouter := strings.Repeat("../", depth-2) + "__router/router"

	src := vite.GenerateClientEntry(vite.ClientEntryOptions{
		RelSveltePath: relSvelte,
		RelRouterPath: relRouter,
		HasSnapshot:   hasSnapshot,
	})
	entryAbs := filepath.Join(entryDir, "entry.ts")
	if err := os.WriteFile(entryAbs, []byte(src), genFileMode); err != nil {
		return "", "", fmt.Errorf("codegen: write client entry %s: %w", entryAbs, err)
	}

	enhanceAbs := filepath.Join(entryDir, "enhance.ts")
	if err := os.WriteFile(enhanceAbs, []byte(vite.GenerateEnhanceRuntime()), genFileMode); err != nil {
		return "", "", fmt.Errorf("codegen: write enhance runtime %s: %w", enhanceAbs, err)
	}

	// Path to the .svelte source from the SPA router directory
	// (.gen/client/__router/) — depth from routes parent to project root
	// is the same as for entry.ts, so router.ts at depth-3 ascends
	// correspondingly.
	routerDirFromRoot := filepath.ToSlash(filepath.Join(outDir, "client", "__router"))
	routerDepth := len(strings.Split(routerDirFromRoot, "/"))
	relSvelteFromRouter := vite.RelativeSveltePath(svelteSrc, routerDepth)
	return routeKey, relSvelteFromRouter, nil
}

// emitClientRouter writes .gen/client/__router/router.ts — the shared
// SPA router module imported by every per-route entry.ts. routeMap keys
// are canonical route patterns (matching server-side router.Route.Pattern)
// and values are paths to each route's +page.svelte source relative to
// the router.ts file. snapshotRoutes flags which patterns export a
// Snapshot so the generated router wires capture/restore hooks.
func emitClientRouter(projectRoot, outDir string, routeMap map[string]string, snapshotRoutes map[string]bool) error {
	dir := filepath.Join(projectRoot, outDir, "client", "__router")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("codegen: mkdir client router %s: %w", dir, err)
	}
	src := vite.GenerateRouter(vite.RouterOptions{
		Routes:         routeMap,
		SnapshotRoutes: snapshotRoutes,
	})
	target := filepath.Join(dir, "router.ts")
	if err := os.WriteFile(target, []byte(src), genFileMode); err != nil {
		return fmt.Errorf("codegen: write client router %s: %w", target, err)
	}
	navTarget := filepath.Join(dir, "navigation.ts")
	if err := os.WriteFile(navTarget, []byte(vite.GenerateNavigationModule()), genFileMode); err != nil {
		return fmt.Errorf("codegen: write client navigation %s: %w", navTarget, err)
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

// emitErrorPage parses one +error.svelte and writes the generated
// error.gen.go into the route's encoded gen package directory. The
// package may also host a layout.gen.go and/or page.gen.go from the
// same directory; the distinct filename keeps them separate.
func emitErrorPage(projectRoot, outDir, errorDir, pkgPath, pkgName string, provenance bool, generatedAt time.Time) error {
	errPath := filepath.Join(errorDir, "+error.svelte")
	src, err := os.ReadFile(errPath) //nolint:gosec // path comes from scanner walk under projectRoot
	if err != nil {
		return fmt.Errorf("codegen: read %s: %w", errPath, err)
	}
	frag, perrs := parser.Parse(src)
	if len(perrs) > 0 {
		return fmt.Errorf("codegen: parse %s: %w", errPath, perrs)
	}
	out, err := GenerateErrorPage(frag, ErrorPageOptions{
		PackageName:   pkgName,
		Filename:      errPath,
		Provenance:    provenance,
		SourceContent: src,
		GeneratedAt:   generatedAt,
	})
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
// any $lib/dev/** import is a fatal error. lookup resolves env.Static*
// calls to literal string values at build time.
func emitLayout(projectRoot, outDir, layoutDir, pkgPath, pkgName, serverFile string, release bool, lookup EnvLookup, provenance bool, generatedAt time.Time) (bool, error) {
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
	substituted, err := substituteStaticEnv(string(src), lookup)
	if err != nil {
		return false, fmt.Errorf("codegen: %s: %w", layoutPath, err)
	}
	frag, perrs := parser.Parse([]byte(substituted))
	if len(perrs) > 0 {
		return false, fmt.Errorf("codegen: parse %s: %w", layoutPath, perrs)
	}
	out, err := GenerateLayout(frag, LayoutOptions{
		PackageName:    pkgName,
		ServerFilePath: serverFile,
		Filename:       layoutPath,
		Provenance:     provenance,
		SourceContent:  src,
		GeneratedAt:    generatedAt,
	})
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
