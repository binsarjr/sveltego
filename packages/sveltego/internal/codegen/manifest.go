package codegen

import (
	"errors"
	"fmt"
	"go/format"
	"sort"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/routescan"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// entry pairs one ScannedRoute with the import alias the manifest will
// use for the route's gen package. Hoisted to package scope so the
// adapter emitter can range over the same slice.
type entry struct {
	route routescan.ScannedRoute
	alias string
}

// layoutImport pairs a layout package path with the import alias used
// for it inside the generated manifest. hasServer reports whether the
// layout dir owns a layout.server.go; the manifest emits a load adapter
// only for those. hasHead reports whether the layout template emits a
// Head method; the manifest emits a head adapter and a LayoutHeads slot
// for it. Hoisted to package scope so the adapter emitter can consume
// the same slice.
type layoutImport struct {
	pkgPath   string
	alias     string
	hasServer bool
	hasHead   bool
	// hasSSR is set for layouts in SSRRenderLayouts: the manifest emits
	// the payload-bridge form of render__layout__<alias> that dispatches
	// to <alias>.RenderLayoutSSR (children-callback ABI from #453).
	hasSSR bool
}

// errorImport pairs an error-page package path with the import alias
// used for it inside the generated manifest. The manifest emits one
// renderError__<alias> adapter per unique error package.
//
// hasSSR is set for boundaries in SSRRenderErrors: the manifest emits
// the payload-bridge form of renderError__<alias> that dispatches to
// `<alias>.RenderErrorSSR` (the wire shipped per #412) instead of the
// legacy `ErrorPage{}.Render` adapter.
type errorImport struct {
	pkgPath string
	alias   string
	hasSSR  bool
}

// ManifestOptions configures [GenerateManifest].
type ManifestOptions struct {
	// PackageName is the package clause for the generated file. Defaults to
	// "gen" when empty.
	PackageName string
	// ModulePath is the user app's module path; per-route imports are
	// composed as ModulePath + "/" + GenRoot + "/" + per-route subpath.
	ModulePath string
	// GenRoot is the gen-output root relative to the user module root, e.g.
	// ".gen". Trailing slashes are stripped.
	GenRoot string
	// RouteOptions, when non-nil, supplies effective page options per
	// route keyed by ScannedRoute.Pattern. Missing keys fall back to
	// kit.DefaultPageOptions(); a nil map disables emission entirely
	// (the manifest defaults at runtime). Build resolves the cascade
	// ahead of time so the manifest emitter does no I/O.
	RouteOptions map[string]kit.PageOptions
	// PageHeads tracks every page package that emits a Head method.
	// Keys are ScannedRoute.PackagePath; manifest emits a head__<alias>
	// adapter and a Head field on the route entry only for keys present
	// here.
	PageHeads map[string]bool
	// LayoutHeads tracks every layout package that emits a Head method.
	// Keys are layout PackagePath; the manifest emits a head__layout__
	// <alias> adapter and a LayoutHeads slot for keys present here.
	LayoutHeads map[string]bool
	// ClientKeys maps ScannedRoute.PackagePath to the Vite client entry
	// key (e.g. "routes/_page"). When present the manifest emits a
	// ClientKey field on each matching router.Route.
	ClientKeys map[string]string
	// HasServiceWorker, when true, makes the manifest declare a
	// `const HasServiceWorker = true` so the server runtime can gate the
	// auto-registration <script> on file presence (#89). Defaults to
	// false — the constant is still emitted so server code can read it
	// unconditionally without a build-tag dance.
	HasServiceWorker bool
	// SSRRenderRoutes is the set of Svelte-mode route patterns that
	// received a Phase 6 (#428) Render(payload, data) emit under
	// .gen/usersrc/<encoded-pkg>/. Keys are route Pattern; values are
	// the encoded package subpath (without the ".gen/" prefix) so the
	// emitter can route the Page adapter through it. Routes absent
	// from this map fall back to the legacy SPA shell.
	SSRRenderRoutes map[string]string
	// SSRFallbackRoutes is the ordered list of Svelte-mode routes that
	// declared `<!-- sveltego:ssr-fallback -->` in their _page.svelte
	// (Phase 8 / #430). The manifest emits one Page handler per entry
	// that proxies the request to the long-running Node sidecar via the
	// runtime/svelte/fallback registry.
	SSRFallbackRoutes []SSRFallbackRoute
	// SSRRenderLayouts is the set of layout package paths (".gen/..."
	// prefix preserved) whose `_layout.svelte` received a children-
	// callback ABI emit under `.gen/layoutsrc/<encoded>/` plus a
	// `wire_layout_render.gen.go` per route dir (#456). For these
	// layouts the manifest emits the payload-bridge form of
	// `render__layout__<alias>` — calling the typed
	// `RenderLayoutSSR(payload, data, inner)` from the wire — instead of
	// the legacy `Layout{}.Render(*render.Writer, ..., children)` shape.
	SSRRenderLayouts map[string]struct{}
	// SSRRenderErrors is the set of error-boundary package paths
	// (".gen/..." prefix preserved) whose `_error.svelte` received an
	// SSR transpile emit under `.gen/errorsrc/<encoded>/` plus a sibling
	// `wire_error_render.gen.go` (#412). For these boundaries the
	// manifest emits the payload-bridge form of `renderError__<alias>`
	// that dispatches `RenderErrorSSR(payload, safe)` instead of the
	// legacy `ErrorPage{}.Render` Mustache-Go adapter, so SSR error
	// rendering travels the same Option B transpile path as page bodies.
	SSRRenderErrors map[string]struct{}
}

// GenerateManifest emits a deterministic, gofmt-clean Go source file
// declaring `func Routes() []router.Route` from a [routescan.ScanResult].
//
// Routes with neither a _page.svelte nor a _server.go are skipped (e.g.
// orphan _page.server.go directories — the scanner already emits a
// diagnostic for these). Page routes emit Page: <pkg>.Page{}.Render and
// optionally Load / Actions when a _page.server.go is present alongside.
// API-only routes emit Server: <pkg>.Handlers; the generated package's
// _server.go is expected to declare `var Handlers map[string]http.HandlerFunc`.
//
// Symbol existence is not verified — Load / Actions / Handlers are emitted
// unconditionally on file presence and `go build` surfaces the missing
// declaration. The trade-off keeps the emitter free of a Go AST pass over
// user code at codegen time.
func GenerateManifest(scan *routescan.ScanResult, opts ManifestOptions) ([]byte, error) {
	if scan == nil {
		return nil, errors.New("codegen: nil scan result")
	}
	if opts.ModulePath == "" {
		return nil, errors.New("codegen: empty module path")
	}
	pkg := opts.PackageName
	if pkg == "" {
		pkg = "gen"
	}
	genRoot := strings.TrimRight(strings.TrimSpace(opts.GenRoot), "/")
	if genRoot == "" {
		genRoot = ".gen"
	}

	// Filter routes the manifest can register, preserving scanner order.
	var entries []entry
	aliasByPkg := make(map[string]string)
	aliasSet := make(map[string]struct{})
	uniqueAlias := func(pkg string) string {
		if a, ok := aliasByPkg[pkg]; ok {
			return a
		}
		base := packageAlias(pkg)
		alias := base
		for i := 2; ; i++ {
			if _, dup := aliasSet[alias]; !dup {
				break
			}
			alias = fmt.Sprintf("%s_%d", base, i)
		}
		aliasSet[alias] = struct{}{}
		aliasByPkg[pkg] = alias
		return alias
	}
	for _, r := range scan.Routes {
		if !r.HasPage && !r.HasServer {
			continue
		}
		alias := uniqueAlias(r.PackagePath)
		entries = append(entries, entry{route: r, alias: alias})
	}

	// Reserve aliases for layout-only packages (layout dirs touched by a
	// route whose own dir does not contribute an entry above). Layouts
	// with a sibling layout.server.go also get a hasServer flag so the
	// emitter knows whether to wire LayoutLoaders.
	var layoutImports []layoutImport
	seenLayoutPkg := make(map[string]struct{})
	layoutHasServer := make(map[string]bool)
	for _, r := range scan.Routes {
		for i, p := range r.LayoutPackagePaths {
			if i < len(r.LayoutServerFiles) && r.LayoutServerFiles[i] != "" {
				layoutHasServer[p] = true
			}
			if _, ok := seenLayoutPkg[p]; ok {
				continue
			}
			seenLayoutPkg[p] = struct{}{}
			alias := uniqueAlias(p)
			layoutImports = append(layoutImports, layoutImport{pkgPath: p, alias: alias})
		}
	}
	for i := range layoutImports {
		layoutImports[i].hasServer = layoutHasServer[layoutImports[i].pkgPath]
		layoutImports[i].hasHead = opts.LayoutHeads[layoutImports[i].pkgPath]
		if opts.SSRRenderLayouts != nil {
			_, ok := opts.SSRRenderLayouts[layoutImports[i].pkgPath]
			layoutImports[i].hasSSR = ok
		}
	}
	sort.Slice(layoutImports, func(i, j int) bool {
		return layoutImports[i].pkgPath < layoutImports[j].pkgPath
	})

	// Reserve aliases for error-page packages. A boundary may live in a
	// directory that itself contributes neither a page nor a layout
	// (orphan _error.svelte), so the lookup walks every route's
	// ErrorBoundaryPackagePath rather than relying on entries / layouts.
	var errorImports []errorImport
	seenErrorPkg := make(map[string]struct{})
	for _, r := range scan.Routes {
		if r.ErrorBoundaryPackagePath == "" {
			continue
		}
		p := r.ErrorBoundaryPackagePath
		if _, ok := seenErrorPkg[p]; ok {
			continue
		}
		seenErrorPkg[p] = struct{}{}
		alias := uniqueAlias(p)
		errorImports = append(errorImports, errorImport{pkgPath: p, alias: alias})
	}
	for i := range errorImports {
		if opts.SSRRenderErrors != nil {
			_, ok := opts.SSRRenderErrors[errorImports[i].pkgPath]
			errorImports[i].hasSSR = ok
		}
	}
	sort.Slice(errorImports, func(i, j int) bool {
		return errorImports[i].pkgPath < errorImports[j].pkgPath
	})

	var b Builder
	b.Line("// Code generated by sveltego. DO NOT EDIT.")
	b.Linef("package %s", pkg)
	b.Line("")

	// Surface scanner diagnostics in the generated source so users see them
	// when they open .gen/manifest.gen.go.
	for _, d := range scan.Diagnostics {
		b.Linef("// SCANNER DIAGNOSTIC: %s", d.String())
	}
	if len(scan.Diagnostics) > 0 {
		b.Line("")
	}

	// Adapter emission needs `fmt` for the type-mismatch error message
	// only when at least one Mustache-Go Page or Layout adapter is
	// present. Svelte-mode pages with a Phase 6 SSR Render emit also
	// emit an adapter (bridging RenderSSR → PageHandler) so they DO
	// contribute to the hasPage accounting.
	fallbackByRoute := make(map[string]string, len(opts.SSRFallbackRoutes))
	for _, fb := range opts.SSRFallbackRoutes {
		fallbackByRoute[fb.Pattern] = fb.Source
	}
	var (
		hasPage     bool
		hasSvelte   bool
		hasFallback = len(fallbackByRoute) > 0
		ssrRoutes   = opts.SSRRenderRoutes
	)
	for _, e := range entries {
		if !e.route.HasPage {
			continue
		}
		svelteMode := isSvelteRoute(e.route.Pattern, opts.RouteOptions)
		_, hasSSR := ssrRoutes[e.route.Pattern]
		_, isFallback := fallbackByRoute[e.route.Pattern]
		switch {
		case svelteMode && hasSSR:
			hasPage = true
			hasSvelte = true
		case svelteMode && isFallback:
			hasPage = true
		case !svelteMode:
			hasPage = true
		}
	}
	hasLayout := len(layoutImports) > 0
	hasError := len(errorImports) > 0
	if !hasSvelte {
		for _, li := range layoutImports {
			if li.hasSSR {
				hasSvelte = true
				break
			}
		}
	}
	if !hasSvelte {
		for _, ei := range errorImports {
			if ei.hasSSR {
				hasSvelte = true
				break
			}
		}
	}
	// usesFmt reports whether any emitted adapter actually references
	// fmt.Errorf. Only the legacy Mustache-Go page+layout adapters
	// (typed-data type-assert error path) use it; the SSR payload-bridge
	// adapters introduced in Phase 6 (#428) and #456 do not.
	usesFmt := false
	for _, e := range entries {
		if !e.route.HasPage {
			continue
		}
		if !isSvelteRoute(e.route.Pattern, opts.RouteOptions) {
			usesFmt = true
			break
		}
	}
	if !usesFmt {
		for _, li := range layoutImports {
			if !li.hasSSR {
				usesFmt = true
				break
			}
		}
	}

	// Build a deduplicated set of (alias, packagePath) pairs across page
	// entries, layout-only imports, and error-only imports.
	type importPair struct {
		alias   string
		pkgPath string
	}
	imports := make([]importPair, 0, len(entries)+len(layoutImports)+len(errorImports))
	seenAlias := make(map[string]struct{})
	for _, e := range entries {
		if _, ok := seenAlias[e.alias]; ok {
			continue
		}
		// RFC #379 phase 3: Svelte-mode page routes without a sibling
		// _page.server.go contribute no Go symbols (no Page render, no
		// Load wire). Skipping the import keeps the manifest compiling
		// when the route directory holds only `_page.svelte`. Exception
		// (#467): routes that received an SSR Render emit get a
		// wire_render.gen.go in their gen package, so the alias resolves
		// even without a user-authored _page.server.go.
		_, hasSSREmit := ssrRoutes[e.route.Pattern]
		if isSvelteRoute(e.route.Pattern, opts.RouteOptions) && !e.route.HasPageServer && !e.route.HasServer && !hasSSREmit {
			continue
		}
		seenAlias[e.alias] = struct{}{}
		imports = append(imports, importPair{alias: e.alias, pkgPath: e.route.PackagePath})
	}
	for _, li := range layoutImports {
		if _, ok := seenAlias[li.alias]; ok {
			continue
		}
		seenAlias[li.alias] = struct{}{}
		imports = append(imports, importPair{alias: li.alias, pkgPath: li.pkgPath})
	}
	for _, ei := range errorImports {
		if _, ok := seenAlias[ei.alias]; ok {
			continue
		}
		seenAlias[ei.alias] = struct{}{}
		imports = append(imports, importPair{alias: ei.alias, pkgPath: ei.pkgPath})
	}
	sort.Slice(imports, func(i, j int) bool { return imports[i].alias < imports[j].alias })

	hasNonDefaultOptions := false
	if opts.RouteOptions != nil {
		def := kit.DefaultPageOptions()
		for _, e := range entries {
			if v, ok := opts.RouteOptions[e.route.Pattern]; ok && !v.Equal(def) {
				hasNonDefaultOptions = true
				break
			}
		}
	}

	b.Line("import (")
	b.Indent()
	switch {
	case hasPage || hasLayout:
		if usesFmt {
			b.Line(`"fmt"`)
			b.Line("")
		}
		b.Line(`"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"`)
		b.Line(`"github.com/binsarjr/sveltego/packages/sveltego/render"`)
	case hasError:
		b.Line(`"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"`)
		b.Line(`"github.com/binsarjr/sveltego/packages/sveltego/render"`)
	case hasNonDefaultOptions:
		b.Line(`"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"`)
	}
	if hasSvelte {
		b.Line(`server "github.com/binsarjr/sveltego/packages/sveltego/runtime/svelte/server"`)
	}
	if hasFallback {
		b.Line(`fallback "github.com/binsarjr/sveltego/packages/sveltego/runtime/svelte/fallback"`)
	}
	b.Line(`"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"`)
	if len(imports) > 0 {
		b.Line("")
		for _, ip := range imports {
			importPath := opts.ModulePath + "/" + genRoot + "/" + strings.TrimPrefix(ip.pkgPath, ".gen/")
			b.Linef("%s %q", ip.alias, importPath)
		}
	}
	b.Dedent()
	b.Line(")")
	b.Line("")

	// HasServiceWorker is read by the runtime to decide whether to inject
	// the auto-registration <script> for /service-worker.js. Always
	// emitted so server code can reference it without a build-tag dance.
	b.Linef("// HasServiceWorker reports whether the project declares src/service-worker.ts.")
	b.Linef("// The runtime reads this constant to decide whether to emit the auto-registration")
	b.Linef("// <script> tag (#89).")
	b.Linef("const HasServiceWorker = %t", opts.HasServiceWorker)
	b.Line("")

	// Emit one RouteID<Slug> constant per linkable route so user code can
	// reference routes without magic strings.
	if err := emitRouteIDConsts(&b, entries); err != nil {
		return nil, err
	}

	// Per-route Render adapters: widen Page{}.Render's typed PageData
	// parameter to the `any`-shaped router.PageHandler. Type assertion
	// happens inside the adapter so a dispatcher mismatch fails fast
	// with a descriptive error instead of a panic.
	emitRenderAdapters(&b, entries, opts.RouteOptions, ssrRoutes)
	emitFallbackAdapters(&b, entries, opts.RouteOptions, fallbackByRoute)
	emitFallbackInit(&b, opts.SSRFallbackRoutes)
	emitHeadAdapters(&b, entries, opts.PageHeads, opts.RouteOptions)
	emitLayoutAdapters(&b, layoutImports)
	emitLayoutHeadAdapters(&b, layoutImports)
	emitErrorAdapters(&b, errorImports)
	layoutByPathForChain := make(map[string]layoutImport, len(layoutImports))
	for _, li := range layoutImports {
		layoutByPathForChain[li.pkgPath] = li
	}
	errorAliasByPathForChain := make(map[string]string, len(errorImports))
	for _, ei := range errorImports {
		errorAliasByPathForChain[ei.pkgPath] = ei.alias
	}
	emitRouteRenderChains(&b, entries, layoutByPathForChain, errorAliasByPathForChain)

	b.Line("// Routes returns the route table consumed by router.NewTree.")
	b.Line("func Routes() []router.Route {")
	b.Indent()
	if len(entries) == 0 {
		b.Line("return nil")
	} else {
		b.Line("return []router.Route{")
		b.Indent()
		layoutByPath := make(map[string]layoutImport, len(layoutImports))
		for _, li := range layoutImports {
			layoutByPath[li.pkgPath] = li
		}
		errorAliasByPath := make(map[string]string, len(errorImports))
		for _, ei := range errorImports {
			errorAliasByPath[ei.pkgPath] = ei.alias
		}
		for _, e := range entries {
			emitRouteEntry(&b, e.route, e.alias, layoutByPath, errorAliasByPath, opts.RouteOptions, opts.PageHeads, opts.ClientKeys, ssrRoutes, fallbackByRoute)
		}
		b.Dedent()
		b.Line("}")
	}
	b.Dedent()
	b.Line("}")

	if err := b.Err(); err != nil {
		return nil, err
	}

	out, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("codegen: format manifest source: %w", err)
	}
	return out, nil
}

// packageAlias derives the per-route import alias from a ScannedRoute
// PackagePath. The leading ".gen/" prefix is stripped, the remainder is
// split on "/" and joined with "_", and the result is prefixed with
// "page_". Example: ".gen/routes/posts/_slug_" -> "page_routes_posts__slug_".
func packageAlias(packagePath string) string {
	rel := strings.TrimPrefix(packagePath, ".gen/")
	if rel == "" || rel == ".gen" {
		return "page_routes"
	}
	parts := strings.Split(rel, "/")
	return "page_" + strings.Join(parts, "_")
}

// emitRouteIDConsts writes one `RouteID<Slug>` string constant per
// linkable route (HasPage or HasServer), sorted by pattern. The constant
// value is the SvelteKit-style canonical pattern ("/posts/[slug]") so
// user code can pass it directly to kit.Link without a magic string.
// Collision detection returns an error when two routes produce the same
// Go identifier; the build gate catches this at codegen time rather than
// silently shadowing a constant.
func emitRouteIDConsts(b *Builder, entries []entry) error {
	if len(entries) == 0 {
		return nil
	}
	// Detect identifier collisions before emitting anything.
	seen := make(map[string]string, len(entries))
	for _, e := range entries {
		ident := "RouteID" + routeIdent(e.route.Segments)
		if prev, ok := seen[ident]; ok {
			return fmt.Errorf(
				"codegen: RouteID collision: %q and %q both map to %s",
				prev, e.route.Pattern, ident,
			)
		}
		seen[ident] = e.route.Pattern
	}

	b.Line("// Route ID constants for type-safe linking. Pass to kit.Link instead of a")
	b.Line("// raw string so route renames surface as compile errors.")
	b.Line("const (")
	b.Indent()
	for _, e := range entries {
		ident := "RouteID" + routeIdent(e.route.Segments)
		b.Linef("%s = %s", ident, quoteGo(e.route.Pattern))
	}
	b.Dedent()
	b.Line(")")
	b.Line("")
	return nil
}

// emitRenderAdapters writes one `render__<alias>` function per Page
// route that wraps Page{}.Render into router.PageHandler shape. The
// adapter zero-values PageData on a nil input so an empty Load() return
// renders cleanly. Svelte-mode routes with a Phase 6 (#428) SSR Render
// emit go through ssr.RenderSSR(payload, data) and the result is
// written into the page writer. Svelte-mode routes WITHOUT an SSR
// emit are skipped — the runtime falls back to the SPA shell path.
func emitRenderAdapters(b *Builder, entries []entry, routeOptions map[string]kit.PageOptions, ssrRoutes map[string]string) {
	hasAny := false
	for _, e := range entries {
		switch {
		case !e.route.HasPage:
			continue
		case isSvelteRoute(e.route.Pattern, routeOptions):
			if _, ok := ssrRoutes[e.route.Pattern]; ok {
				hasAny = true
			}
		default:
			hasAny = true
		}
		if hasAny {
			break
		}
	}
	if !hasAny {
		return
	}
	for _, e := range entries {
		if !e.route.HasPage {
			continue
		}
		svelteMode := isSvelteRoute(e.route.Pattern, routeOptions)
		_, hasSSR := ssrRoutes[e.route.Pattern]
		if svelteMode && !hasSSR {
			continue
		}
		if svelteMode {
			emitSvelteRenderAdapter(b, e)
			continue
		}
		b.Linef("// render__%s adapts %s.Page{}.Render to router.PageHandler.", e.alias, e.alias)
		b.Linef("func render__%s(w *render.Writer, ctx *kit.RenderCtx, data any) error {", e.alias)
		b.Indent()
		b.Linef("var typed %s.PageData", e.alias)
		b.Line("if data != nil {")
		b.Indent()
		b.Linef("v, ok := data.(%s.PageData)", e.alias)
		b.Line("if !ok {")
		b.Indent()
		b.Linef(`return fmt.Errorf("sveltego: route %%q: PageData type mismatch (got %%T, want %s.PageData)", %s, data)`, e.alias, quoteGo(e.route.Pattern))
		b.Dedent()
		b.Line("}")
		b.Line("typed = v")
		b.Dedent()
		b.Line("}")
		b.Linef("return %s.Page{}.Render(w, ctx, typed)", e.alias)
		b.Dedent()
		b.Line("}")
		b.Line("")
	}
}

// emitSvelteRenderAdapter writes the Phase 6 (#428) bridge that calls
// the wire-emitted RenderSSR (which dispatches to the typed
// usersrc.Render(payload, PageData, pageState)), then copies the
// payload buffer into the route's page writer. The PageState is built
// per-request from kit.RenderCtx so templates that read $app/state
// runes (`page.url`, `page.params`, …) see the live snapshot
// (issue #466).
func emitSvelteRenderAdapter(b *Builder, e entry) {
	b.Linef("// render__%s bridges %s.RenderSSR (Svelte SSR Option B) into router.PageHandler.", e.alias, e.alias)
	b.Linef("func render__%s(w *render.Writer, ctx *kit.RenderCtx, data any) error {", e.alias)
	b.Indent()
	b.Line("var payload server.Payload")
	emitPageStateBuild(b, e.route.Pattern, "data")
	b.Linef("if err := %s.RenderSSR(&payload, data, pageState); err != nil {", e.alias)
	b.Indent()
	b.Line("return err")
	b.Dedent()
	b.Line("}")
	b.Line("if head := payload.HeadHTML(); head != \"\" {")
	b.Indent()
	b.Line("// Per ADR 0009 the head buffer is appended to the route's")
	b.Line("// page writer; the server pipeline picks it up via the same")
	b.Line("// shellHead injection point used for Mustache-Go pages.")
	b.Line("w.WriteString(head)")
	b.Dedent()
	b.Line("}")
	b.Line("w.WriteString(payload.Body())")
	b.Line("return nil")
	b.Dedent()
	b.Line("}")
	b.Line("")
}

// emitPageStateBuild writes the canonical block that constructs a
// server.PageState from the in-flight kit.RenderCtx for the page-render
// adapters to forward into RenderSSR / RenderLayoutSSR / RenderErrorSSR
// (issue #466). pattern is the route's canonical Pattern (e.g.
// `/post/[id]`) — Svelte exposes it as `page.route.id`. dataIdent is
// the local-binding name the caller's scope holds the typed PageData
// under — `data` for the page bridge, `pageData` for the chain
// composer — and lands on `page.data` reads.
//
// Server-side, navigating and updated stay at their idle defaults;
// they're client-driven signals.
func emitPageStateBuild(b *Builder, pattern, dataIdent string) {
	b.Line("pageState := server.PageState{")
	b.Indent()
	b.Line("URL:    ctx.URL,")
	b.Line("Params: ctx.Params,")
	b.Linef("Route:  server.PageRoute{ID: %s},", quoteGo(pattern))
	b.Line("Status: 200,")
	b.Linef("Data:   %s,", dataIdent)
	b.Dedent()
	b.Line("}")
}

// emitFallbackAdapters writes one `renderFallback__<routeIdent>` Page
// handler per route annotated with `<!-- sveltego:ssr-fallback -->`.
// The handler dispatches the (route, data) pair through the
// runtime/svelte/fallback registry which talks to the long-running
// Node sidecar over HTTP and caches by `(route, hash(load_result))`.
// Per ADR 0009 sub-decision 2 these handlers exist only for routes the
// build-time transpiler intentionally skipped — non-annotated lowering
// failures are hard build errors.
func emitFallbackAdapters(b *Builder, entries []entry, routeOptions map[string]kit.PageOptions, fallbackByRoute map[string]string) {
	if len(fallbackByRoute) == 0 {
		return
	}
	for _, e := range entries {
		if !e.route.HasPage {
			continue
		}
		if !isSvelteRoute(e.route.Pattern, routeOptions) {
			continue
		}
		if _, ok := fallbackByRoute[e.route.Pattern]; !ok {
			continue
		}
		ident := routeIdent(e.route.Segments)
		b.Linef("// renderFallback__%s dispatches %s through the long-running sidecar (Phase 8 / #430).", ident, quoteGo(e.route.Pattern))
		b.Linef("func renderFallback__%s(w *render.Writer, ctx *kit.RenderCtx, data any) error {", ident)
		b.Indent()
		b.Line("rctx := ctx.Request.Context()")
		b.Linef("resp, err := fallback.Default().Render(rctx, %s, data)", quoteGo(e.route.Pattern))
		b.Line("if err != nil {")
		b.Indent()
		b.Line("return err")
		b.Dedent()
		b.Line("}")
		b.Line("if resp.Head != \"\" {")
		b.Indent()
		b.Line("w.WriteString(resp.Head)")
		b.Dedent()
		b.Line("}")
		b.Line("w.WriteString(resp.Body)")
		b.Line("return nil")
		b.Dedent()
		b.Line("}")
		b.Line("")
	}
}

// emitFallbackInit writes a package-level init() that registers each
// fallback route with runtime/svelte/fallback.Default() so a single
// Configure call from the server boot wires them all to the live
// Client. The init() is omitted when no routes opt in.
func emitFallbackInit(b *Builder, fallback []SSRFallbackRoute) {
	if len(fallback) == 0 {
		return
	}
	b.Line("func init() {")
	b.Indent()
	b.Line("r := fallback.Default()")
	for _, fb := range fallback {
		b.Linef("r.Register(%s, %s)", quoteGo(fb.Pattern), quoteGo(fb.Source))
	}
	b.Dedent()
	b.Line("}")
	b.Line("")
}

// emitLayoutAdapters writes one `render__layout__<alias>` function per
// unique layout package. The default form widens Layout{}.Render's typed
// LayoutData to the `any`-shaped router.LayoutHandler (legacy mustache
// path). Layouts in SSRRenderLayouts swap to the payload-bridge form
// from #456: dispatch to the wire-emitted RenderLayoutSSR through a
// fresh server.Payload, bridge the writer-shape `children` callback
// into the payload, and copy payload.Body() into the outer writer at
// the end. Layout packages with a sibling layout.server.go additionally
// receive a load adapter `loadLayout__<alias>` that wraps the wire-
// emitted LayoutLoad to satisfy router.LayoutLoadHandler.
func emitLayoutAdapters(b *Builder, imports []layoutImport) {
	for _, li := range imports {
		if li.hasSSR {
			emitSSRLayoutAdapter(b, li)
		} else {
			b.Linef("// render__layout__%s adapts %s.Layout{}.Render to router.LayoutHandler.", li.alias, li.alias)
			b.Linef("// The trailing pageState parameter is the $app/state surface (#466);")
			b.Linef("// legacy Mustache-Go layouts ignore it but accept the trailing arg so")
			b.Linef("// the per-route renderChain__<routeIdent> emit can pass pageState")
			b.Linef("// uniformly across SSR-bridged and Mustache-Go layouts.")
			b.Linef("func render__layout__%s(w *render.Writer, ctx *kit.RenderCtx, data any, children func(*render.Writer) error, pageState server.PageState) error {", li.alias)
			b.Indent()
			b.Line("_ = pageState")
			b.Linef("var typed %s.LayoutData", li.alias)
			b.Line("if data != nil {")
			b.Indent()
			b.Linef("v, ok := data.(%s.LayoutData)", li.alias)
			b.Line("if !ok {")
			b.Indent()
			b.Linef(`return fmt.Errorf("sveltego: layout %%q: LayoutData type mismatch (got %%T, want %s.LayoutData)", %s, data)`, li.alias, quoteGo(li.pkgPath))
			b.Dedent()
			b.Line("}")
			b.Line("typed = v")
			b.Dedent()
			b.Line("}")
			b.Linef("return %s.Layout{}.Render(w, ctx, typed, children)", li.alias)
			b.Dedent()
			b.Line("}")
			b.Line("")
		}

		if li.hasServer {
			b.Linef("// loadLayout__%s adapts %s.LayoutLoad to router.LayoutLoadHandler.", li.alias, li.alias)
			b.Linef("func loadLayout__%s(ctx *kit.LoadCtx) (any, error) { return %s.LayoutLoad(ctx) }", li.alias, li.alias)
			b.Line("")
		}
	}
}

// emitSSRLayoutAdapter writes the children-callback payload-bridge form
// of render__layout__<alias> introduced by #456. The emitted adapter
// allocates a fresh server.Payload, dispatches to the wire-emitted
// RenderLayoutSSR (which calls the typed Render(payload, data, inner)
// from the layoutsrc package), bridges the writer-shape `children`
// callback into the payload via a temporary render.Writer, and copies
// the rendered head + body into the outer writer. pageState is built
// per-route by the renderChain__<routeIdent> caller and forwarded
// through every layer so layout templates that read $app/state runes
// see the same snapshot the page handler does (issue #466).
func emitSSRLayoutAdapter(b *Builder, li layoutImport) {
	b.Linef("// render__layout__%s bridges %s.RenderLayoutSSR (Svelte SSR Option B,", li.alias, li.alias)
	b.Linef("// children-callback ABI from #453) into router.LayoutHandler.")
	b.Linef("func render__layout__%s(w *render.Writer, ctx *kit.RenderCtx, data any, children func(*render.Writer) error, pageState server.PageState) error {", li.alias)
	b.Indent()
	b.Line("_ = ctx")
	b.Line("var payload server.Payload")
	b.Line("var childErr error")
	b.Line("inner := func(p *server.Payload) {")
	b.Indent()
	b.Line("if childErr != nil {")
	b.Indent()
	b.Line("return")
	b.Dedent()
	b.Line("}")
	b.Line("buf := render.Acquire()")
	b.Line("defer render.Release(buf)")
	b.Line("if err := children(buf); err != nil {")
	b.Indent()
	b.Line("childErr = err")
	b.Line("return")
	b.Dedent()
	b.Line("}")
	b.Line("p.Push(string(buf.Bytes()))")
	b.Dedent()
	b.Line("}")
	b.Linef("if err := %s.RenderLayoutSSR(&payload, data, inner, pageState); err != nil {", li.alias)
	b.Indent()
	b.Line("return err")
	b.Dedent()
	b.Line("}")
	b.Line("if childErr != nil {")
	b.Indent()
	b.Line("return childErr")
	b.Dedent()
	b.Line("}")
	b.Line("if head := payload.HeadHTML(); head != \"\" {")
	b.Indent()
	b.Line("w.WriteString(head)")
	b.Dedent()
	b.Line("}")
	b.Line("w.WriteString(payload.Body())")
	b.Line("return nil")
	b.Dedent()
	b.Line("}")
	b.Line("")
}

// isSvelteRoute reports whether the route's resolved page options
// select the pure-Svelte template pipeline (RFC #379 phase 3). Used by
// adapter emission to skip Go-side render/head wrapping for Svelte
// routes — those are served as shell + JSON payload at runtime and
// rendered client-side by Svelte.
func isSvelteRoute(pattern string, routeOptions map[string]kit.PageOptions) bool {
	if routeOptions == nil {
		return false
	}
	return routeOptions[pattern].Templates == kit.TemplatesSvelte
}

// emitHeadAdapters writes one `head__<alias>` function per Page
// route whose template emits a Head method. Each adapter widens the
// typed PageData parameter to router.PageHeadHandler shape using the
// same type-assert-or-error pattern as render__<alias>. Svelte-mode
// routes are skipped: Head is part of the .svelte body that Vite owns.
func emitHeadAdapters(b *Builder, entries []entry, pageHeads map[string]bool, routeOptions map[string]kit.PageOptions) {
	if len(pageHeads) == 0 {
		return
	}
	for _, e := range entries {
		if !e.route.HasPage || !pageHeads[e.route.PackagePath] || isSvelteRoute(e.route.Pattern, routeOptions) {
			continue
		}
		b.Linef("// head__%s adapts %s.Page{}.Head to router.PageHeadHandler.", e.alias, e.alias)
		b.Linef("func head__%s(w *render.Writer, ctx *kit.RenderCtx, data any) error {", e.alias)
		b.Indent()
		b.Linef("var typed %s.PageData", e.alias)
		b.Line("if data != nil {")
		b.Indent()
		b.Linef("v, ok := data.(%s.PageData)", e.alias)
		b.Line("if !ok {")
		b.Indent()
		b.Linef(`return fmt.Errorf("sveltego: route %%q: PageData type mismatch (got %%T, want %s.PageData)", %s, data)`, e.alias, quoteGo(e.route.Pattern))
		b.Dedent()
		b.Line("}")
		b.Line("typed = v")
		b.Dedent()
		b.Line("}")
		b.Linef("return %s.Page{}.Head(w, ctx, typed)", e.alias)
		b.Dedent()
		b.Line("}")
		b.Line("")
	}
}

// emitLayoutHeadAdapters writes one `head__layout__<alias>` function per
// layout package whose template emits a Head method. Mirrors the layout
// render adapter but binds Layout{}.Head and the LayoutData type.
func emitLayoutHeadAdapters(b *Builder, imports []layoutImport) {
	for _, li := range imports {
		if !li.hasHead {
			continue
		}
		b.Linef("// head__layout__%s adapts %s.Layout{}.Head to router.LayoutHeadHandler.", li.alias, li.alias)
		b.Linef("func head__layout__%s(w *render.Writer, ctx *kit.RenderCtx, data any) error {", li.alias)
		b.Indent()
		b.Linef("var typed %s.LayoutData", li.alias)
		b.Line("if data != nil {")
		b.Indent()
		b.Linef("v, ok := data.(%s.LayoutData)", li.alias)
		b.Line("if !ok {")
		b.Indent()
		b.Linef(`return fmt.Errorf("sveltego: layout %%q: LayoutData type mismatch (got %%T, want %s.LayoutData)", %s, data)`, li.alias, quoteGo(li.pkgPath))
		b.Dedent()
		b.Line("}")
		b.Line("typed = v")
		b.Dedent()
		b.Line("}")
		b.Linef("return %s.Layout{}.Head(w, ctx, typed)", li.alias)
		b.Dedent()
		b.Line("}")
		b.Line("")
	}
}

// emitErrorAdapters writes one `renderError__<alias>` function per
// unique error-page package. The default form forwards (ctx, w, safe)
// to the legacy `<alias>.ErrorPage{}.Render` (Mustache-Go path).
// Boundaries in SSRRenderErrors swap to the payload-bridge form from
// #412: dispatch to the wire-emitted RenderErrorSSR through a fresh
// server.Payload and copy payload.Body() into the outer writer at the
// end. kit.SafeError is shared across the framework so the typed and
// erased forms match.
func emitErrorAdapters(b *Builder, imports []errorImport) {
	for _, ei := range imports {
		if ei.hasSSR {
			emitSSRErrorAdapter(b, ei)
			continue
		}
		b.Linef("// renderError__%s adapts %s.ErrorPage{}.Render to router.ErrorHandler.", ei.alias, ei.alias)
		b.Linef("func renderError__%s(w *render.Writer, ctx *kit.RenderCtx, safe kit.SafeError) error {", ei.alias)
		b.Indent()
		b.Linef("return %s.ErrorPage{}.Render(w, ctx, safe)", ei.alias)
		b.Dedent()
		b.Line("}")
		b.Line("")
	}
}

// emitSSRErrorAdapter writes the payload-bridge form of
// renderError__<alias> introduced by #412. The emitted adapter
// allocates a fresh server.Payload, dispatches to the wire-emitted
// RenderErrorSSR (which calls the typed Render(payload, kit.SafeError,
// pageState) from the errorsrc package), and copies the rendered head
// + body into the outer writer. The shape mirrors emitSSRLayoutAdapter
// so the SSR pipeline composes consistently across page, layout, and
// error rendering paths. PageState is built inline from ctx and the
// SafeError so error templates that read $app/state runes see a
// snapshot tagged with the error's HTTP status (issue #466).
func emitSSRErrorAdapter(b *Builder, ei errorImport) {
	b.Linef("// renderError__%s bridges %s.RenderErrorSSR (Svelte SSR Option B,", ei.alias, ei.alias)
	b.Linef("// payload-bridge from #412) into router.ErrorHandler.")
	b.Linef("func renderError__%s(w *render.Writer, ctx *kit.RenderCtx, safe kit.SafeError) error {", ei.alias)
	b.Indent()
	b.Line("var payload server.Payload")
	b.Line("pageState := server.PageState{")
	b.Indent()
	b.Line("URL:    ctx.URL,")
	b.Line("Params: ctx.Params,")
	b.Line("Status: safe.HTTPStatus(),")
	b.Line("Error:  &server.PageError{Message: safe.Error(), Status: safe.HTTPStatus()},")
	b.Dedent()
	b.Line("}")
	b.Linef("%s.RenderErrorSSR(&payload, safe, pageState)", ei.alias)
	b.Line("if head := payload.HeadHTML(); head != \"\" {")
	b.Indent()
	b.Line("w.WriteString(head)")
	b.Dedent()
	b.Line("}")
	b.Line("w.WriteString(payload.Body())")
	b.Line("return nil")
	b.Dedent()
	b.Line("}")
	b.Line("")
}

// emitRouteRenderChains writes one renderChain__<routeIdent> function per
// route that has at least one layout, plus one renderErrorChain__<routeIdent>
// per route with an error boundary. Both fold the layout-composition loop
// (previously rebuilt as runtime closures in pipeline.go and errors.go)
// into a per-route emit so the runtime stores a single function pointer
// rather than a slice of LayoutHandler closures.
//
// renderChain composes outer→inner around the page handler:
//
//	render__layout__<l0>(w, ctx, layoutDatas[0], func(w) error {
//	  return render__layout__<l1>(w, ctx, layoutDatas[1], func(w) error {
//	    return page(w, ctx, pageData)
//	  })
//	})
//
// renderErrorChain wraps the route's error template with the surviving
// outer-N layouts (depth from ScannedRoute.ErrorBoundaryLayoutDepth)
// and passes nil layoutData to each — matching the previous behavior
// in renderErrorBoundary. The error template adapter (renderError__
// <errorAlias>) is reused for the innermost call.
func emitRouteRenderChains(b *Builder, entries []entry, layoutByPath map[string]layoutImport, errorAliasByPath map[string]string) {
	emitted := false
	for _, e := range entries {
		emitChain := e.route.HasPage && len(e.route.LayoutPackagePaths) > 0
		emitErrChain := e.route.ErrorBoundaryPackagePath != "" && errorAliasByPath[e.route.ErrorBoundaryPackagePath] != ""
		if !emitted && (emitChain || emitErrChain) {
			emitLayoutDataAtHelper(b)
			emitted = true
		}
		emitRouteRenderChain(b, e.route, layoutByPath)
		emitRouteRenderErrorChain(b, e.route, layoutByPath, errorAliasByPath)
	}
}

// emitLayoutDataAtHelper writes the helper read used by every
// renderChain__<routeIdent> emit. Hoisted so each route's chain expression
// stays a single `return render__layout__<...>` line rather than dragging
// in a per-call bounds check that bloats generated code.
func emitLayoutDataAtHelper(b *Builder) {
	b.Line("// layoutDataAt returns layoutDatas[i] when i is within bounds, otherwise nil.")
	b.Line("// Used by per-route renderChain__<routeIdent> emits to keep the chain expression")
	b.Line("// shape uniform regardless of how many entries the pipeline supplied.")
	b.Line("func layoutDataAt(layoutDatas []any, i int) any {")
	b.Indent()
	b.Line("if i < len(layoutDatas) {")
	b.Indent()
	b.Line("return layoutDatas[i]")
	b.Dedent()
	b.Line("}")
	b.Line("return nil")
	b.Dedent()
	b.Line("}")
	b.Line("")
}

// emitRouteRenderChain emits renderChain__<routeIdent> when the route has
// at least one layout. Routes without layouts get no emit; the runtime
// leaves Route.RenderChain == nil and the pipeline calls Page directly.
func emitRouteRenderChain(b *Builder, r routescan.ScannedRoute, layoutByPath map[string]layoutImport) {
	if !r.HasPage || len(r.LayoutPackagePaths) == 0 {
		return
	}
	ident := routeIdent(r.Segments)
	b.Linef("// renderChain__%s composes the route's layout chain (outer→inner)", ident)
	b.Linef("// around the page handler. Generated per-route so the pipeline does not")
	b.Linef("// rebuild a closure stack on every request.")
	b.Linef("func renderChain__%s(w *render.Writer, ctx *kit.RenderCtx, page router.PageHandler, pageData any, layoutDatas []any) error {", ident)
	b.Indent()
	emitPageStateBuild(b, r.Pattern, "pageData")
	emitChainBody(b, r.LayoutPackagePaths, layoutByPath, "page(w, ctx, pageData)")
	b.Dedent()
	b.Line("}")
	b.Line("")
}

// emitRouteRenderErrorChain emits renderErrorChain__<routeIdent> when the
// route has an error boundary. The function composes the surviving
// outer-layout prefix (depth = r.ErrorBoundaryLayoutDepth) around the
// error-template adapter. nil layoutData propagates to each layout —
// matching the legacy renderErrorBoundary behavior where error renders
// did not see layout data.
func emitRouteRenderErrorChain(b *Builder, r routescan.ScannedRoute, layoutByPath map[string]layoutImport, errorAliasByPath map[string]string) {
	if r.ErrorBoundaryPackagePath == "" {
		return
	}
	errAlias, ok := errorAliasByPath[r.ErrorBoundaryPackagePath]
	if !ok || errAlias == "" {
		return
	}
	ident := routeIdent(r.Segments)
	depth := r.ErrorBoundaryLayoutDepth
	if depth < 0 {
		depth = 0
	}
	if depth > len(r.LayoutPackagePaths) {
		depth = len(r.LayoutPackagePaths)
	}
	survivingLayouts := r.LayoutPackagePaths[:depth]

	b.Linef("// renderErrorChain__%s composes the surviving outer-layout prefix", ident)
	b.Linef("// around the route's _error.svelte handler. The boundary depth (%d)", depth)
	b.Linef("// is baked in; layoutData is intentionally nil per legacy behavior.")
	b.Linef("func renderErrorChain__%s(w *render.Writer, ctx *kit.RenderCtx, safe kit.SafeError, layoutDatas []any) error {", ident)
	b.Indent()
	b.Line("_ = layoutDatas")
	if len(survivingLayouts) > 0 {
		// pageState gets threaded into the surviving outer layouts so
		// their templates (nav bars, breadcrumbs) can read $app/state.
		// The error path overrides Status from safe.HTTPStatus() and
		// populates page.error so `{#if page.error}` branches activate.
		b.Linef("pageState := server.PageState{URL: ctx.URL, Params: ctx.Params, Route: server.PageRoute{ID: %s}, Status: safe.HTTPStatus(), Error: &server.PageError{Message: safe.Error(), Status: safe.HTTPStatus()}}", quoteGo(r.Pattern))
	}
	innerCall := fmt.Sprintf("renderError__%s(w, ctx, safe)", errAlias)
	emitErrorChainBody(b, survivingLayouts, layoutByPath, innerCall)
	b.Dedent()
	b.Line("}")
	b.Line("")
}

// emitChainBody writes the success-path layout composition. For zero
// layouts it just runs the inner expression. For N layouts it writes
// nested children-callback closures wrapping inner.
func emitChainBody(b *Builder, layoutPaths []string, layoutByPath map[string]layoutImport, innerCall string) {
	if len(layoutPaths) == 0 {
		b.Linef("return %s", innerCall)
		return
	}
	openLayoutCalls(b, layoutPaths, layoutByPath, innerCall, false)
}

// emitErrorChainBody writes the error-path layout composition. Each
// layout receives nil for its data parameter, matching the legacy
// renderErrorBoundary behavior.
func emitErrorChainBody(b *Builder, layoutPaths []string, layoutByPath map[string]layoutImport, innerCall string) {
	if len(layoutPaths) == 0 {
		b.Linef("return %s", innerCall)
		return
	}
	openLayoutCalls(b, layoutPaths, layoutByPath, innerCall, true)
}

// openLayoutCalls emits the outer→inner layout-composition Go code as a
// single `return render__layout__<l0>(...)` expression. nilData=true
// passes a nil layoutData to every wrapper (error path); nilData=false
// reads from layoutDatas[i] (success path). pageState is forwarded
// verbatim to every layout call so layout templates that read
// $app/state runes see the same snapshot the page handler does
// (issue #466).
func openLayoutCalls(b *Builder, layoutPaths []string, layoutByPath map[string]layoutImport, innerCall string, nilData bool) {
	// Build the nested expression depth-first. For three layouts l0,l1,l2:
	//   return render__layout__<l0>(w, ctx, d0, func(w) error {
	//     return render__layout__<l1>(w, ctx, d1, func(w) error {
	//       return render__layout__<l2>(w, ctx, d2, func(w) error {
	//         return <innerCall>
	//       }, pageState)
	//     }, pageState)
	//   }, pageState)
	for i, path := range layoutPaths {
		li, ok := layoutByPath[path]
		if !ok {
			// Layout package missing from import map — emit a defensive
			// passthrough so the build still compiles. This shouldn't
			// happen in practice (every layout in r.LayoutPackagePaths
			// appears in layoutImports), but bailing here keeps the
			// generated file syntactically valid.
			continue
		}
		dataExpr := "nil"
		if !nilData {
			dataExpr = fmt.Sprintf("layoutDataAt(layoutDatas, %d)", i)
		}
		b.Linef("return render__layout__%s(w, ctx, %s, func(w *render.Writer) error {", li.alias, dataExpr)
		b.Indent()
	}
	b.Linef("return %s", innerCall)
	for range layoutPaths {
		b.Dedent()
		b.Line("}, pageState)")
	}
}

func emitRouteEntry(b *Builder, r routescan.ScannedRoute, alias string, layoutByPath map[string]layoutImport, errorAliasByPath map[string]string, routeOptions map[string]kit.PageOptions, pageHeads map[string]bool, clientKeys map[string]string, ssrRoutes map[string]string, fallbackByRoute map[string]string) {
	b.Line("{")
	b.Indent()
	b.Linef("Pattern: %s,", quoteGo(r.Pattern))
	emitSegments(b, r.Segments)
	svelteMode := isSvelteRoute(r.Pattern, routeOptions)
	_, hasSSR := ssrRoutes[r.Pattern]
	_, isFallback := fallbackByRoute[r.Pattern]
	switch {
	case r.HasServer:
		b.Linef("Server: %s.Handlers,", alias)
	case r.HasPage:
		switch {
		case !svelteMode:
			b.Linef("Page: render__%s,", alias)
			if pageHeads[r.PackagePath] {
				b.Linef("Head: head__%s,", alias)
			}
		case hasSSR:
			// Phase 6 (#428): Svelte-mode route with a Render emit.
			// The bridge adapter wraps the typed RenderSSR into the
			// PageHandler shape so the existing renderPage path
			// produces a full HTML body.
			b.Linef("Page: render__%s,", alias)
		case isFallback:
			// Phase 8 (#430): annotated route routes through the
			// long-running sidecar at request time. The renderFallback
			// adapter dispatches via the runtime/svelte/fallback
			// registry. The route ident is used so two routes that
			// happen to share a package alias still get distinct
			// handler names.
			b.Linef("Page: renderFallback__%s,", routeIdent(r.Segments))
		}
		if ck := clientKeys[r.PackagePath]; ck != "" {
			b.Linef("ClientKey: %s,", quoteGo(ck))
		}
	}
	if r.HasPageServer {
		b.Linef("Load: %s.Load,", alias)
		b.Linef("Actions: %s.Actions,", alias)
	}
	if len(r.LayoutPackagePaths) > 0 {
		// RenderChain folds the layout-composition closure stack into a
		// single per-route function emitted by emitRouteRenderChain.
		// Server-only routes do not get a RenderChain even if they share
		// a layout dir — their Page handler is nil so the pipeline never
		// invokes RenderChain for them.
		if r.HasPage {
			b.Linef("RenderChain: renderChain__%s,", routeIdent(r.Segments))
		}

		anyServer := false
		anyHead := false
		for _, p := range r.LayoutPackagePaths {
			li, ok := layoutByPath[p]
			if !ok {
				continue
			}
			if li.hasServer {
				anyServer = true
			}
			if li.hasHead {
				anyHead = true
			}
		}
		if anyServer {
			b.Line("LayoutLoaders: []router.LayoutLoadHandler{")
			b.Indent()
			for _, p := range r.LayoutPackagePaths {
				li, ok := layoutByPath[p]
				if !ok {
					continue
				}
				if li.hasServer {
					b.Linef("loadLayout__%s,", li.alias)
				} else {
					b.Line("nil,")
				}
			}
			b.Dedent()
			b.Line("},")
		}
		if anyHead {
			b.Line("LayoutHeads: []router.LayoutHeadHandler{")
			b.Indent()
			for _, p := range r.LayoutPackagePaths {
				li, ok := layoutByPath[p]
				if !ok {
					continue
				}
				if li.hasHead {
					b.Linef("head__layout__%s,", li.alias)
				} else {
					b.Line("nil,")
				}
			}
			b.Dedent()
			b.Line("},")
		}
	}
	emitOptionsField(b, r.Pattern, routeOptions)
	if r.ErrorBoundaryPackagePath != "" {
		if ea, ok := errorAliasByPath[r.ErrorBoundaryPackagePath]; ok && ea != "" {
			// renderErrorChain__<routeIdent> wraps the surviving outer-layout
			// prefix around renderError__<errAlias> at compile time.
			b.Linef("RenderError: renderErrorChain__%s,", routeIdent(r.Segments))
		}
	}
	b.Dedent()
	b.Line("},")
}

// emitOptionsField writes one `Options: kit.PageOptions{...}` line per
// route when routeOptions is non-nil. Routes without an entry default
// to kit.DefaultPageOptions(). The Options literal is always emitted
// so the runtime sees the resolved cascade (Templates in particular,
// which gates the svelte vs legacy render path).
func emitOptionsField(b *Builder, pattern string, routeOptions map[string]kit.PageOptions) {
	if routeOptions == nil {
		return
	}
	opts, ok := routeOptions[pattern]
	if !ok {
		opts = kit.DefaultPageOptions()
	}
	b.Linef("Options: %s,", formatPageOptions(opts))
}

// formatPageOptions renders a kit.PageOptions value as a Go composite
// literal. Zero-valued fields are omitted so the output stays focused
// on the user-meaningful overrides.
func formatPageOptions(o kit.PageOptions) string {
	var parts []string
	if o.Prerender {
		parts = append(parts, "Prerender: true")
	}
	if o.PrerenderAuto {
		parts = append(parts, "PrerenderAuto: true")
	}
	if o.PrerenderProtected {
		parts = append(parts, "PrerenderProtected: true")
	}
	if !o.SSR {
		parts = append(parts, "SSR: false")
	} else {
		parts = append(parts, "SSR: true")
	}
	if !o.CSR {
		parts = append(parts, "CSR: false")
	} else {
		parts = append(parts, "CSR: true")
	}
	if o.SSROnly {
		parts = append(parts, "SSROnly: true")
	}
	if o.CSRF {
		parts = append(parts, "CSRF: true")
	} else {
		parts = append(parts, "CSRF: false")
	}
	if o.TrailingSlash != kit.TrailingSlashNever {
		parts = append(parts, "TrailingSlash: "+trailingSlashIdent(o.TrailingSlash))
	}
	if o.Templates != "" {
		parts = append(parts, "Templates: "+quoteGo(o.Templates))
	}
	return "kit.PageOptions{" + strings.Join(parts, ", ") + "}"
}

func trailingSlashIdent(ts kit.TrailingSlash) string {
	switch ts {
	case kit.TrailingSlashAlways:
		return "kit.TrailingSlashAlways"
	case kit.TrailingSlashIgnore:
		return "kit.TrailingSlashIgnore"
	case kit.TrailingSlashDefault:
		return "kit.TrailingSlashDefault"
	}
	return "kit.TrailingSlashNever"
}

func emitSegments(b *Builder, segs []router.Segment) {
	if len(segs) == 0 {
		b.Line("Segments: []router.Segment{},")
		return
	}
	b.Line("Segments: []router.Segment{")
	b.Indent()
	for _, s := range segs {
		b.Line(formatSegment(s))
	}
	b.Dedent()
	b.Line("},")
}

func formatSegment(s router.Segment) string {
	var sb strings.Builder
	sb.WriteString("{Kind: ")
	sb.WriteString(segmentKindIdent(s.Kind))
	if s.Value != "" {
		sb.WriteString(", Value: ")
		sb.WriteString(quoteGo(s.Value))
	}
	if s.Name != "" {
		sb.WriteString(", Name: ")
		sb.WriteString(quoteGo(s.Name))
	}
	if s.Matcher != "" {
		sb.WriteString(", Matcher: ")
		sb.WriteString(quoteGo(s.Matcher))
	}
	sb.WriteString("},")
	return sb.String()
}

func segmentKindIdent(k router.SegmentKind) string {
	switch k {
	case router.SegmentStatic:
		return "router.SegmentStatic"
	case router.SegmentParam:
		return "router.SegmentParam"
	case router.SegmentOptional:
		return "router.SegmentOptional"
	case router.SegmentRest:
		return "router.SegmentRest"
	}
	return "router.SegmentStatic"
}
