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
}

// errorImport pairs an error-page package path with the import alias
// used for it inside the generated manifest. The manifest emits one
// renderError__<alias> adapter per unique error package.
type errorImport struct {
	pkgPath string
	alias   string
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
	// only when at least one Page or Layout adapter is present.
	var hasPage bool
	for _, e := range entries {
		if e.route.HasPage {
			hasPage = true
			break
		}
	}
	hasLayout := len(layoutImports) > 0
	hasError := len(errorImports) > 0

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
		b.Line(`"fmt"`)
		b.Line("")
		b.Line(`"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"`)
		b.Line(`"github.com/binsarjr/sveltego/packages/sveltego/render"`)
	case hasError:
		b.Line(`"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"`)
		b.Line(`"github.com/binsarjr/sveltego/packages/sveltego/render"`)
	case hasNonDefaultOptions:
		b.Line(`"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"`)
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
	emitRenderAdapters(&b, entries)
	emitHeadAdapters(&b, entries, opts.PageHeads)
	emitLayoutAdapters(&b, layoutImports)
	emitLayoutHeadAdapters(&b, layoutImports)
	emitErrorAdapters(&b, errorImports)

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
			emitRouteEntry(&b, e.route, e.alias, layoutByPath, errorAliasByPath, opts.RouteOptions, opts.PageHeads, opts.ClientKeys)
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
// renders cleanly.
func emitRenderAdapters(b *Builder, entries []entry) {
	hasAny := false
	for _, e := range entries {
		if e.route.HasPage {
			hasAny = true
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

// emitLayoutAdapters writes one `render__layout__<alias>` function per
// unique layout package. Each adapter widens Layout{}.Render's typed
// LayoutData to the `any`-shaped router.LayoutHandler. Layout packages
// with a sibling layout.server.go additionally receive a load adapter
// `loadLayout__<alias>` that wraps the wire-emitted LayoutLoad to
// satisfy router.LayoutLoadHandler.
func emitLayoutAdapters(b *Builder, imports []layoutImport) {
	for _, li := range imports {
		b.Linef("// render__layout__%s adapts %s.Layout{}.Render to router.LayoutHandler.", li.alias, li.alias)
		b.Linef("func render__layout__%s(w *render.Writer, ctx *kit.RenderCtx, data any, children func(*render.Writer) error) error {", li.alias)
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
		b.Linef("return %s.Layout{}.Render(w, ctx, typed, children)", li.alias)
		b.Dedent()
		b.Line("}")
		b.Line("")

		if li.hasServer {
			b.Linef("// loadLayout__%s adapts %s.LayoutLoad to router.LayoutLoadHandler.", li.alias, li.alias)
			b.Linef("func loadLayout__%s(ctx *kit.LoadCtx) (any, error) { return %s.LayoutLoad(ctx) }", li.alias, li.alias)
			b.Line("")
		}
	}
}

// emitHeadAdapters writes one `head__<alias>` function per Page
// route whose template emits a Head method. Each adapter widens the
// typed PageData parameter to router.PageHeadHandler shape using the
// same type-assert-or-error pattern as render__<alias>.
func emitHeadAdapters(b *Builder, entries []entry, pageHeads map[string]bool) {
	if len(pageHeads) == 0 {
		return
	}
	for _, e := range entries {
		if !e.route.HasPage || !pageHeads[e.route.PackagePath] {
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
// unique error-page package. The adapter forwards (ctx, w, safe) to
// ErrorPage{}.Render with no widening; kit.SafeError is shared across
// the framework so the typed and erased forms match.
func emitErrorAdapters(b *Builder, imports []errorImport) {
	for _, ei := range imports {
		b.Linef("// renderError__%s adapts %s.ErrorPage{}.Render to router.ErrorHandler.", ei.alias, ei.alias)
		b.Linef("func renderError__%s(w *render.Writer, ctx *kit.RenderCtx, safe kit.SafeError) error {", ei.alias)
		b.Indent()
		b.Linef("return %s.ErrorPage{}.Render(w, ctx, safe)", ei.alias)
		b.Dedent()
		b.Line("}")
		b.Line("")
	}
}

func emitRouteEntry(b *Builder, r routescan.ScannedRoute, alias string, layoutByPath map[string]layoutImport, errorAliasByPath map[string]string, routeOptions map[string]kit.PageOptions, pageHeads map[string]bool, clientKeys map[string]string) {
	b.Line("{")
	b.Indent()
	b.Linef("Pattern: %s,", quoteGo(r.Pattern))
	emitSegments(b, r.Segments)
	switch {
	case r.HasServer:
		b.Linef("Server: %s.Handlers,", alias)
	case r.HasPage:
		b.Linef("Page: render__%s,", alias)
		if pageHeads[r.PackagePath] {
			b.Linef("Head: head__%s,", alias)
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
		b.Line("LayoutChain: []router.LayoutHandler{")
		b.Indent()
		for _, p := range r.LayoutPackagePaths {
			li, ok := layoutByPath[p]
			if !ok {
				continue
			}
			b.Linef("render__layout__%s,", li.alias)
		}
		b.Dedent()
		b.Line("},")

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
			b.Linef("Error: renderError__%s,", ea)
			if r.ErrorBoundaryLayoutDepth > 0 {
				b.Linef("ErrorLayoutDepth: %d,", r.ErrorBoundaryLayoutDepth)
			}
		}
	}
	b.Dedent()
	b.Line("},")
}

// emitOptionsField writes one `Options: kit.PageOptions{...}` line per
// route when routeOptions is non-nil. Routes without an entry default
// to kit.DefaultPageOptions(); the default value is suppressed so the
// generated source stays minimal for projects that declare no options.
func emitOptionsField(b *Builder, pattern string, routeOptions map[string]kit.PageOptions) {
	if routeOptions == nil {
		return
	}
	opts, ok := routeOptions[pattern]
	if !ok {
		opts = kit.DefaultPageOptions()
	}
	if opts.Equal(kit.DefaultPageOptions()) {
		return
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
