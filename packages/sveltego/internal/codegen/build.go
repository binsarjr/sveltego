package codegen

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"go/format"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen/csrffallback"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen/svelterender"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/routescan"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/vite"
)

// log attribute keys for sloglint compliance.
const (
	logKeyDiagnostic    = "diagnostic"
	logKeyRoutes        = "routes"
	logKeyManifest      = "manifest"
	logKeyModule        = "module"
	logKeyElapsed       = "elapsed"
	logKeyFallbackCount = "fallback_count"
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
//
// ctx is propagated into the Phase 6 (#428) SSR sidecar so a devserver
// hot rebuild or a CLI shutdown signal can tear down the Node child
// process promptly. Pass context.Background() when there is no
// surrounding cancellation source.
func Build(ctx context.Context, opts BuildOptions) (*BuildResult, error) {
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

	// Resolve page-options cascade up front so per-route emission sees
	// the resolved kit.PageOptions for manifest emission and SSG gating.
	// RFC #379 phase 5 made "svelte" the only template pipeline so the
	// cascade no longer branches the page-body emit decision.
	routeOptions, err := resolvePageOptions(scan)
	if err != nil {
		return nil, fmt.Errorf("codegen: resolve page options: %w", err)
	}

	routeCount := 0
	emittedLayouts := make(map[string]struct{})
	// clientRouteKeys collects the Vite input key for every page route.
	var clientRouteKeys []string
	clientKeysByPkg := make(map[string]string)
	// clientRouterMap maps each route's canonical pattern to the path of
	// its mount-target component (the per-route wrapper.svelte when the
	// route has a layout chain — see #508 — otherwise the bare
	// _page.svelte) relative to the SPA router module
	// (.gen/client/__router/router.ts). It feeds vite.GenerateRouter so
	// the SPA router can lazy-import the right component per navigation.
	clientRouterMap := make(map[string]string)
	// clientSnapshotRoutes records which patterns export a Snapshot from
	// `<script module>` so vite.GenerateRouter wires the snapshot capture
	// + restore hooks (#84). Empty when no route opts in.
	clientSnapshotRoutes := make(map[string]bool)
	// clientChainKeys maps each route's canonical pattern to its
	// layout-chain identifier (vite.LayoutChainKey). Routes with no
	// _layout.svelte map to the empty string. The SPA router uses
	// these keys to decide between an in-place page-slot swap and a
	// full unmount+mount on navigation (#518).
	clientChainKeys := make(map[string]string)
	// chainLayouts tracks the layout chain for every unique chainKey
	// seen during the walk so the post-loop emitter can write one
	// shared wrapper per chain (#518). Identical chains hash to the
	// same key, so the map collapses repeats.
	chainLayouts := make(map[string][]string)

	for _, route := range scan.Routes {
		switch {
		case route.HasPage:
			pageName := "_page.svelte"
			if route.HasReset {
				pageName = "_page@" + route.ResetTarget + ".svelte"
			}
			// Pure-Svelte page bodies are owned by Vite + svelte/server;
			// the Go side keeps Load + manifest entry only. The page
			// module's `<script module>` snapshot export (when present)
			// is imported directly by entry.ts and forwarded to the
			// router for per-route capture/restore (#84). Routes with a
			// layout chain mount the chainKey-shared wrapper instead of
			// the bare page; the page module is still imported by
			// entry.ts so it can seed wrapperState.Page (#518).
			pagePath := filepath.Join(route.Dir, pageName)
			moduleExports, err := extractModuleExportsFromSvelte(pagePath)
			if err != nil {
				return nil, err
			}
			hasSnapshot := slices.Contains(moduleExports, "snapshot")
			routeCount++

			// Path math runs unconditionally so the manifest's ClientKey
			// stays wired even under `sveltego prerender` (NoClient=true).
			// Without this, prerender would clobber manifest.gen.go and
			// drop ClientKey from every route, leaving the frozen HTML
			// without its <script type="module"> client bundle tag and
			// breaking hydration on SSG pages.
			csrfLowering := route.SSRFallback && routeCSRFEnabled(route.Pattern, routeOptions)
			ck, relPageFromRouter, chainKey, cerr := emitClientEntry(opts.ProjectRoot, outDir, routesDir, route, pageName, hasSnapshot, csrfLowering, !opts.NoClient)
			if cerr != nil {
				return nil, cerr
			}
			clientRouteKeys = append(clientRouteKeys, ck)
			// Vite's manifest keys facade entries by the source-relative
			// path of the input file, not the input alias. Match the
			// key Vite actually emits so route-tag injection finds the
			// chunk at runtime.
			clientKeysByPkg[route.PackagePath] = filepath.ToSlash(filepath.Join(outDir, "client", filepath.FromSlash(ck), "entry.ts"))
			// pageLoaders feed the SPA router the page module path so a
			// cross-route same-chain swap can pull in the destination
			// page module without touching the wrapper (#518).
			clientRouterMap[route.Pattern] = relPageFromRouter
			clientChainKeys[route.Pattern] = chainKey
			if chainKey != "" {
				chainLayouts[chainKey] = route.LayoutChain
			}
			if hasSnapshot {
				clientSnapshotRoutes[route.Pattern] = true
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
			if serverFile != "" {
				if err := emitLayoutMirrorAndWire(opts.ProjectRoot, outDir, modulePath, pkgPath, pkgName, serverFile); err != nil {
					return nil, err
				}
			}
			emittedLayouts[layoutDir] = struct{}{}
		}
	}

	// RFC #379 phase 2: emit `_page.svelte.d.ts` / `_layout.svelte.d.ts`
	// next to each route's `.svelte` so pure-Svelte templates pick up
	// the Go-side data shape via Svelte LSP. Runs alongside (does not
	// replace) the Mustache-Go pipeline.
	tgDiags, tgErr := runTypegen(scan.Routes)
	if tgErr != nil {
		return nil, tgErr
	}
	warnings = append(warnings, tgDiags...)

	localsDiags, err := collectLocalsPrerenderWarnings(scan, routeOptions)
	if err != nil {
		return nil, fmt.Errorf("codegen: locals/prerender scan: %w", err)
	}
	warnings = append(warnings, localsDiags...)

	// RFC #379 phase 3: routes opting into pure Svelte templates AND
	// Prerender: true must SSG via Node `svelte/server` at build time.
	// Surface a clear error when Node is missing so users do not chase
	// silent runtime SPA fallbacks.
	if needsNodeForSvelteSSG(scan.Routes, routeOptions) {
		if _, nerr := svelterender.EnsureNode(); nerr != nil {
			return nil, fmt.Errorf("codegen: %w", nerr)
		}
	}

	// RFC #379 / ADR 0009 phase 6 + phase 8: drive the SSR Option-B
	// pipeline. Pure-Svelte routes that aren't prerendered and aren't
	// annotated with `<!-- sveltego:ssr-fallback -->` get a
	// Render(payload, data) Go file emitted under
	// .gen/usersrc/<encoded-pkg>/ via the svelte_js2go transpiler.
	// Annotated routes are returned as Fallback entries so the manifest
	// can wire the runtime sidecar handler instead. ADR 0009 sub-decision
	// 2: any non-annotated transpile or lowering failure is a hard
	// build error.
	ssrPlan, err := runSSRTranspile(ctx, opts.ProjectRoot, outDir, modulePath, logger, scan, routeOptions)
	if err != nil {
		return nil, err
	}

	hasServiceWorker := serviceWorkerEntry(opts.ProjectRoot) != ""
	manifestBytes, err := GenerateManifest(scan, ManifestOptions{
		PackageName:       "gen",
		ModulePath:        modulePath,
		GenRoot:           outDir,
		RouteOptions:      routeOptions,
		ClientKeys:        clientKeysByPkg,
		HasServiceWorker:  hasServiceWorker,
		SSRRenderRoutes:   ssrPlan.Transpiled,
		SSRFallbackRoutes: ssrPlan.Fallback,
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

	// Auto-register user-defined param matchers so the runtime sees
	// them without requiring a manual `cmd/app/main.go` wire-up.
	// Emitted unconditionally so callers can reference gen.Matchers()
	// even when src/params/ is empty (the call returns the built-in
	// defaults in that case).
	if err := emitMatchers(opts.ProjectRoot, outDir, modulePath, "gen", scan.Matchers); err != nil {
		return nil, err
	}

	if err := emitEmbedStub(opts.ProjectRoot, outAbs); err != nil {
		return nil, err
	}

	var viteConfigPath string
	swEntry := ""
	if !opts.NoClient && hasServiceWorker {
		swEntry = "src/service-worker.ts"
	}
	if !opts.NoClient && len(clientRouteKeys) > 0 {
		// Emit one shared wrapper per chainKey BEFORE the router so
		// the loader paths in router.ts are valid the moment vite
		// resolves the import map (#518). Each wrapper takes its full
		// layout chain at codegen time; the page slot remains dynamic
		// through the wrapper-state rune.
		chainWrapperPaths := make(map[string]string, len(chainLayouts))
		for ck, chain := range chainLayouts {
			relWrapperFromRouter, werr := emitChainWrapper(opts.ProjectRoot, outDir, ck, chain)
			if werr != nil {
				return nil, werr
			}
			chainWrapperPaths[ck] = relWrapperFromRouter
		}
		if err := emitClientRouter(opts.ProjectRoot, outDir, clientRouterMap, clientSnapshotRoutes, clientChainKeys, chainWrapperPaths); err != nil {
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

// emitClientEntry writes .gen/client/<routeKey>/entry.ts for one page
// route. When the route walks a layout chain it points at the
// chainKey-shared wrapper module under .gen/client/__chain/<chainKey>/
// (emitted separately by emitChainWrapper) instead of mounting the
// page directly (#518 — one wrapper per chain, page swapped via the
// wrapper-state rune on cross-route same-chain SPA nav).
//
// routeKey is e.g. "routes/_page" or "routes/post/[id]/_page". Returns
// the routeKey, the path of the route's page module relative to the
// SPA router directory (so vite.GenerateRouter can build pageLoaders),
// and the layout-chain key. Routes without a chain mount the page
// directly; chained routes mount the shared wrapper and seed
// wrapperState.Page from the page module's default export.
//
// When csrfLowering is true the route is ssr-fallback annotated AND
// has CSRF enabled (#540): emit a Svelte-source rewrite under
// `.gen/csrf-fallback/<routeKey>/_page.svelte` that splices the hidden
// `_csrf_token` input into every POST form so Svelte 5 hydrate sees
// the input in the vDOM and does not strip the SSR-rendered DOM node.
// The sidecar continues reading the user's original `_page.svelte`;
// the post-hoc `csrfinject.Rewrite` keeps the SSR HTML correct.
func emitClientEntry(projectRoot, outDir, routesDir string, route routescan.ScannedRoute, pageName string, hasSnapshot, csrfLowering, writeFiles bool) (string, string, string, error) {
	routesParent := filepath.Dir(routesDir)
	relDir, err := filepath.Rel(routesParent, route.Dir)
	if err != nil {
		return "", "", "", fmt.Errorf("codegen: client entry rel path: %w", err)
	}
	routeKey := filepath.ToSlash(filepath.Join(relDir, strings.TrimSuffix(pageName, ".svelte")))

	entryRelFromRoot := filepath.ToSlash(filepath.Join(outDir, "client", routeKey, "entry.ts"))
	depth := len(strings.Split(filepath.ToSlash(filepath.Dir(entryRelFromRoot)), "/"))
	routeDirFromRoot, err := filepath.Rel(projectRoot, route.Dir)
	if err != nil {
		return "", "", "", fmt.Errorf("codegen: client entry svelte rel path: %w", err)
	}
	svelteSrc := filepath.ToSlash(routeDirFromRoot) + "/" + pageName

	if csrfLowering && writeFiles {
		lowered, lerr := emitCSRFFallbackLowering(projectRoot, outDir, routeKey, route.Dir, pageName)
		if lerr != nil {
			return "", "", "", lerr
		}
		if lowered != "" {
			svelteSrc = lowered
		}
	}

	// Path to the .svelte source from the SPA router directory
	// (.gen/client/__router/) — depth from routes parent to project root
	// is the same as for entry.ts, so router.ts at depth-3 ascends
	// correspondingly.
	routerDirFromRoot := filepath.ToSlash(filepath.Join(outDir, "client", "__router"))
	routerDepth := len(strings.Split(routerDirFromRoot, "/"))
	relSvelteFromRouter := vite.RelativeSveltePath(svelteSrc, routerDepth)

	chainKey := vite.LayoutChainKey(route.LayoutChain)

	if !writeFiles {
		return routeKey, relSvelteFromRouter, chainKey, nil
	}

	entryDir := filepath.Join(projectRoot, outDir, "client", filepath.FromSlash(routeKey))
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		return "", "", "", fmt.Errorf("codegen: mkdir client entry %s: %w", entryDir, err)
	}

	relSvelte := vite.RelativeSveltePath(svelteSrc, depth)
	// The per-route entry sits at .gen/client/<routeKey>/entry.ts; the
	// shared router module lives at .gen/client/__router/router.ts. The
	// relative path from the entry's directory walks up to .gen/client/
	// and into __router/router.
	relRouter := strings.Repeat("../", depth-2) + "__router/router"

	relWrapper := ""
	if chainKey != "" {
		// chainKey-shared wrapper sits under .gen/client/__chain/<chainKey>/.
		// From the per-route entry's directory, walk up to .gen/client/
		// then descend into __chain/<chainKey>/.
		relWrapper = strings.Repeat("../", depth-2) + "__chain/" + chainKey + "/wrapper.svelte"
	}

	src := vite.GenerateClientEntry(vite.ClientEntryOptions{
		RelSveltePath:  relSvelte,
		RelRouterPath:  relRouter,
		RelWrapperPath: relWrapper,
		LayoutChainKey: chainKey,
		HasSnapshot:    hasSnapshot,
	})
	entryAbs := filepath.Join(entryDir, "entry.ts")
	if err := os.WriteFile(entryAbs, []byte(src), genFileMode); err != nil {
		return "", "", "", fmt.Errorf("codegen: write client entry %s: %w", entryAbs, err)
	}

	return routeKey, relSvelteFromRouter, chainKey, nil
}

// emitCSRFFallbackLowering writes a CSRF-spliced copy of the user's
// `_page.svelte` to `.gen/csrf-fallback/<routeKey>/_page.svelte` for an
// ssr-fallback route that opted into CSRF (#540). Returns the
// project-relative path of the lowered file (suitable for the client
// entry's `import Page from`) when the splicer mutated the source, or
// the empty string when the source contains no POST forms (in which
// case the caller keeps the original path). The sidecar continues
// reading the user's original `_page.svelte`; only the client compile
// is redirected so Svelte 5 hydrate sees the hidden input in vDOM.
func emitCSRFFallbackLowering(projectRoot, outDir, routeKey, routeDir, pageName string) (string, error) {
	srcAbs := filepath.Join(routeDir, pageName)
	contents, err := os.ReadFile(srcAbs) //nolint:gosec // path comes from the trusted route scan
	if err != nil {
		return "", fmt.Errorf("codegen: read csrf-fallback source %s: %w", srcAbs, err)
	}
	result, err := csrffallback.Lower(csrffallback.LowerOptions{
		Source:        srcAbs,
		SourceContent: contents,
	})
	if err != nil {
		return "", fmt.Errorf("codegen: lower csrf-fallback %s: %w", srcAbs, err)
	}
	if !result.Mutated {
		return "", nil
	}
	loweredDir := filepath.Join(projectRoot, outDir, "csrf-fallback", filepath.FromSlash(routeKey))
	if err := os.MkdirAll(loweredDir, 0o755); err != nil {
		return "", fmt.Errorf("codegen: mkdir csrf-fallback %s: %w", loweredDir, err)
	}
	loweredAbs := filepath.Join(loweredDir, pageName)
	if err := os.WriteFile(loweredAbs, result.Content, genFileMode); err != nil {
		return "", fmt.Errorf("codegen: write csrf-fallback %s: %w", loweredAbs, err)
	}
	rel, err := filepath.Rel(projectRoot, loweredAbs)
	if err != nil {
		return "", fmt.Errorf("codegen: csrf-fallback rel path: %w", err)
	}
	return filepath.ToSlash(rel), nil
}

// emitChainWrapper writes a single chainKey-shared wrapper.svelte
// under .gen/client/__chain/<chainKey>/wrapper.svelte. Every route in
// the same layout chain mounts this wrapper; the page slot is
// dynamic, fed from the wrapper-state rune. Layout import paths are
// resolved relative to the wrapper file's directory.
func emitChainWrapper(projectRoot, outDir, chainKey string, layoutChain []string) (string, error) {
	if chainKey == "" {
		return "", nil
	}
	wrapperDir := filepath.Join(projectRoot, outDir, "client", "__chain", chainKey)
	if err := os.MkdirAll(wrapperDir, 0o755); err != nil {
		return "", fmt.Errorf("codegen: mkdir chain wrapper %s: %w", wrapperDir, err)
	}
	// Wrapper sits at .gen/client/__chain/<chainKey>/wrapper.svelte; depth
	// from project root is `len(outDir/client/__chain/<chainKey>)` segments.
	wrapperRelFromRoot := filepath.ToSlash(filepath.Join(outDir, "client", "__chain", chainKey))
	depth := len(strings.Split(wrapperRelFromRoot, "/"))
	layoutImports := make([]string, 0, len(layoutChain))
	for _, layoutDir := range layoutChain {
		layoutSrc, err := resolveLayoutSource(layoutDir)
		if err != nil {
			return "", err
		}
		layoutRelFromRoot, err := filepath.Rel(projectRoot, layoutSrc)
		if err != nil {
			return "", fmt.Errorf("codegen: layout rel path: %w", err)
		}
		layoutImports = append(layoutImports, vite.RelativeSveltePath(filepath.ToSlash(layoutRelFromRoot), depth))
	}
	// wrapper-store.svelte.ts lives in __router/. Walk up to .gen/client/
	// then into __router/.
	relStoreFromWrapper := strings.Repeat("../", depth-2) + "__router/wrapper-store.svelte"
	src := vite.GenerateChainWrapper(vite.ChainWrapperOptions{
		LayoutImports: layoutImports,
		StoreImport:   relStoreFromWrapper,
	})
	wrapperAbs := filepath.Join(wrapperDir, "wrapper.svelte")
	if err := os.WriteFile(wrapperAbs, []byte(src), genFileMode); err != nil {
		return "", fmt.Errorf("codegen: write chain wrapper %s: %w", wrapperAbs, err)
	}

	// The router imports the wrapper module by its path relative to
	// .gen/client/__router/. Both directories sit one level under
	// .gen/client/, so the relative path is `../__chain/<chainKey>/wrapper.svelte`.
	relWrapperFromRouter := "../__chain/" + chainKey + "/wrapper.svelte"
	return relWrapperFromRouter, nil
}

// emitClientRouter writes .gen/client/__router/router.ts — the shared
// SPA router module imported by every per-route entry.ts. routeMap keys
// are canonical route patterns (matching server-side router.Route.Pattern)
// and values are paths to each route's _page.svelte module relative to
// the router.ts file. snapshotRoutes flags which patterns export a
// Snapshot so the generated router wires capture/restore hooks.
// chainKeys maps each pattern to its layout-chain identifier; the
// router uses it to skip unmount/mount on cross-route same-chain navs
// (#518). chainWrappers maps each non-empty chainKey to the path of
// the chainKey-shared wrapper.svelte module relative to router.ts.
//
// Also emits wrapper-store.svelte.ts unconditionally so the router
// import resolves, even when no route currently uses a wrapper.
func emitClientRouter(projectRoot, outDir string, routeMap map[string]string, snapshotRoutes map[string]bool, chainKeys map[string]string, chainWrappers map[string]string) error {
	dir := filepath.Join(projectRoot, outDir, "client", "__router")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("codegen: mkdir client router %s: %w", dir, err)
	}
	src := vite.GenerateRouter(vite.RouterOptions{
		Routes:         routeMap,
		SnapshotRoutes: snapshotRoutes,
		ChainKeys:      chainKeys,
		ChainWrappers:  chainWrappers,
	})
	target := filepath.Join(dir, "router.ts")
	if err := os.WriteFile(target, []byte(src), genFileMode); err != nil {
		return fmt.Errorf("codegen: write client router %s: %w", target, err)
	}
	navTarget := filepath.Join(dir, "navigation.ts")
	if err := os.WriteFile(navTarget, []byte(vite.GenerateNavigationModule()), genFileMode); err != nil {
		return fmt.Errorf("codegen: write client navigation %s: %w", navTarget, err)
	}
	// $app/forms public surface (#545). User code imports `enhance` from
	// `$app/forms`; Vite aliases that to this file. Per-route entry.ts
	// also imports the same module by relative path, so each per-route
	// bundle ships one shared copy instead of a duplicated `enhance.ts`.
	formsTarget := filepath.Join(dir, "forms.ts")
	if err := os.WriteFile(formsTarget, []byte(vite.GenerateFormsModule()), genFileMode); err != nil {
		return fmt.Errorf("codegen: write client forms %s: %w", formsTarget, err)
	}
	// Emit with the `.svelte.ts` extension so vite-plugin-svelte runs
	// the file through the Svelte compiler — `state.svelte.ts` contains
	// `$state(...)` rune calls that a plain `.ts` would ship raw to the
	// browser (#471).
	stateTarget := filepath.Join(dir, "state.svelte.ts")
	if err := os.WriteFile(stateTarget, []byte(vite.GenerateStateModule()), genFileMode); err != nil {
		return fmt.Errorf("codegen: write client state %s: %w", stateTarget, err)
	}
	// wrapper-store backs the per-route wrapper.svelte's reactive prop
	// surface (#508). Always emit so the router import resolves, even
	// when no route currently uses a wrapper — the module is tiny and
	// keeps the import contract uniform.
	wrapperStoreTarget := filepath.Join(dir, "wrapper-store.svelte.ts")
	if err := os.WriteFile(wrapperStoreTarget, []byte(vite.GenerateWrapperStoreModule()), genFileMode); err != nil {
		return fmt.Errorf("codegen: write wrapper store %s: %w", wrapperStoreTarget, err)
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
	case strings.Contains(msg, "orphan _page.server.go"):
		return true
	case strings.Contains(msg, "unknown matcher"):
		return true
	case strings.Contains(msg, "may not have both _page.svelte and _server.go"):
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
		UserPath:    filepath.Join(route.Dir, "_page.server.go"),
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
		hasLoad:        usf.HasLoad,
	})
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
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

// resolveLayoutSource returns the path of the _layout.svelte (or its
// reset variant) inside layoutDir. The plain filename takes precedence;
// otherwise the first matching `_layout@*.svelte` entry wins. The
// scanner already guarantees the directory contains exactly one
// layout source, so the search is unambiguous.
func resolveLayoutSource(layoutDir string) (string, error) {
	plain := filepath.Join(layoutDir, "_layout.svelte")
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
		if !ok || base != "_layout" {
			continue
		}
		return filepath.Join(layoutDir, e.Name()), nil
	}
	return "", fmt.Errorf("codegen: %s contains no _layout.svelte", layoutDir)
}

// needsNodeForSvelteSSG reports whether at least one route combines
// the pure-Svelte template pipeline (the default) with Prerender,
// requiring the Node `svelte/server` sidecar at build time.
func needsNodeForSvelteSSG(routes []routescan.ScannedRoute, routeOptions map[string]kit.PageOptions) bool {
	for _, r := range routes {
		if !r.HasPage {
			continue
		}
		opts, ok := routeOptions[r.Pattern]
		if !ok {
			continue
		}
		if opts.Templates == kit.TemplatesSvelte && (opts.Prerender || opts.PrerenderAuto) {
			return true
		}
	}
	return false
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
