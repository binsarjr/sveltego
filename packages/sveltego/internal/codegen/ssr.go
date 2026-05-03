package codegen

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	sveltejs2go "github.com/binsarjr/sveltego/packages/sveltego/internal/codegen/svelte_js2go"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen/svelterender"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen/typegen"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/routescan"
)

// ssrPlan is the per-route work item for the Phase 6 SSR pipeline. It
// pairs the routescan record with the typegen Shape used to drive
// property-access lowering.
//
// Two flavors share the type:
//
//   - Live SSR routes (the original Phase 6 contract): non-prerendered,
//     own a sibling _page.server.go with non-empty PageData. shape is
//     the user's typegen.Shape.
//   - Prerender routes (#467): Templates=svelte AND Prerender=true.
//     When a sibling _page.server.go exists with PageData fields the
//     shape is the user's; otherwise synthetic is true and the route
//     goes through the transpile pipeline against an empty PageData
//     alias emitted into usersrc/<route>/. The Lowerer's strict mode
//     still hard-errors on `data.X` access because a Prerender route
//     without a Load() has no data to bind — operators must either add
//     a _page.server.go or stop accessing data in the template.
type ssrPlan struct {
	route     routescan.ScannedRoute
	shape     typegen.Shape
	synthetic bool
}

// SSRPlanResult is the outcome of the Phase 6/8 SSR planner. Transpiled
// holds routes that received a build-time JS→Go Render emit and the
// encoded subpath where it lives (consumed by manifest emission).
// Fallback holds routes that explicitly opted out via the
// `<!-- sveltego:ssr-fallback -->` comment; those route through the
// long-running Node sidecar at request time (Phase 8, #430).
type SSRPlanResult struct {
	Transpiled map[string]string
	Fallback   []SSRFallbackRoute
}

// SSRFallbackRoute names one route the runtime must dispatch to the
// long-running sidecar. Pattern is the canonical request path; Source
// is the project-relative path to the route's `_page.svelte` so the
// sidecar can compile and render at request time.
type SSRFallbackRoute struct {
	Pattern string
	Source  string
}

// layoutPlan is the per-layout work item for #456. dir is the absolute
// layout directory (matches a routescan LayoutChain entry); pkgPath is
// the encoded gen package path (`.gen/routes/...`); pkgName is the
// leaf-encoded package name; serverFile points at `_layout.server.go`
// when present; shape carries the typegen Shape used by the Lowerer
// (synthetic empty LayoutData when no server file exists).
type layoutPlan struct {
	dir        string
	pkgPath    string
	pkgName    string
	serverFile string
	shape      typegen.Shape
}

// errorPlan is the per-error-boundary work item for #412. dir is the
// absolute directory containing `_error.svelte`; pkgPath is the encoded
// gen package path (`.gen/routes/...` matching ErrorBoundaryPackagePath);
// pkgName is the leaf-encoded package name. shape is always the
// synthetic ErrorData alias (`type ErrorData = kit.SafeError`) — error
// templates do not declare a server file; their data shape is fixed by
// the framework, not user code. Strict-mode lowering against this shape
// rewrites `data.code` → `data.Code`, `data.message` → `data.Message`,
// `data.id` → `data.ID`.
type errorPlan struct {
	dir     string
	pkgPath string
	pkgName string
	shape   typegen.Shape
}

// runSSRTranspile drives the Phase 6 (#428) + Phase 8 (#430) SSR codegen
// pipeline:
//
//   - Pure-Svelte routes that are not prerendered, declare SSR=true, and
//     own a sibling `_page.server.go` with a non-empty PageData are
//     transpiled to Go via `internal/codegen/svelte_js2go/`. The Render
//     function lands at `.gen/usersrc/<encoded-pkg>/page_render.gen.go`.
//   - Routes that declare `<!-- sveltego:ssr-fallback -->` skip the
//     transpiler and are returned as Fallback entries; the runtime
//     proxies them to a long-running Node sidecar with a per-route
//     cache (Phase 8).
//   - Any other transpiler or lowerer failure is a hard build error
//     (ADR 0009 sub-decision 2). Operators must either fix the source
//     or annotate the route to opt into the sidecar fallback.
func runSSRTranspile(ctx context.Context, projectRoot, outDir, modulePath string, logger *slog.Logger, scan *routescan.ScanResult, routeOptions map[string]kit.PageOptions) (SSRPlanResult, error) {
	transpilePlan, fallback := planSSR(scan, routeOptions)
	layoutPlans := planSSRLayouts(scan, routeOptions)
	errorPlans := planSSRErrors(scan, routeOptions)
	result := SSRPlanResult{Fallback: fallback}
	if len(transpilePlan) == 0 && len(fallback) == 0 && len(layoutPlans) == 0 && len(errorPlans) == 0 {
		return result, nil
	}
	if len(transpilePlan) == 0 && len(layoutPlans) == 0 && len(errorPlans) == 0 {
		logger.Info("ssr fallback only: no routes transpiled to Go",
			logKeyFallbackCount, len(fallback))
		return result, nil
	}
	if _, err := svelterender.EnsureNode(); err != nil {
		return result, fmt.Errorf("codegen: ssr requires node 18+ on $PATH (or annotate routes with <!-- sveltego:ssr-fallback --> if they intentionally bypass the transpiler): %w", err)
	}

	// Image variant pipeline (issue #492): scan every .svelte source for
	// `<Image src=…>` literals once, run the build-time resize pass, and
	// share the resulting variant map across every Transpile call below.
	// Empty map for projects with no <Image> elements — the lowering
	// pre-pass is a no-op when the map is empty.
	imageVariants, err := buildImageVariants(projectRoot, projectImageWidths(routeOptions))
	if err != nil {
		return result, err
	}

	jobs := make([]svelterender.SSRJob, 0, len(transpilePlan)+len(layoutPlans)+len(errorPlans))
	for _, p := range transpilePlan {
		rel, err := filepath.Rel(projectRoot, filepath.Join(p.route.Dir, "_page.svelte"))
		if err != nil {
			return result, fmt.Errorf("codegen: ssr rel path: %w", err)
		}
		jobs = append(jobs, svelterender.SSRJob{
			Route:  p.route.Pattern,
			Source: filepath.ToSlash(rel),
		})
	}
	for _, lp := range layoutPlans {
		layoutSrc, err := resolveLayoutSource(lp.dir)
		if err != nil {
			return result, fmt.Errorf("codegen: ssr layout source %s: %w", lp.dir, err)
		}
		rel, err := filepath.Rel(projectRoot, layoutSrc)
		if err != nil {
			return result, fmt.Errorf("codegen: ssr layout rel %s: %w", layoutSrc, err)
		}
		jobs = append(jobs, svelterender.SSRJob{
			Route:  layoutJobKey(lp.pkgPath),
			Source: filepath.ToSlash(rel),
		})
	}
	for _, ep := range errorPlans {
		rel, err := filepath.Rel(projectRoot, filepath.Join(ep.dir, "_error.svelte"))
		if err != nil {
			return result, fmt.Errorf("codegen: ssr error rel path: %w", err)
		}
		jobs = append(jobs, svelterender.SSRJob{
			Route:  errorJobKey(ep.pkgPath),
			Source: filepath.ToSlash(rel),
		})
	}

	astOutDir := filepath.Join(projectRoot, outDir, "sveltejs2go")
	results, err := svelterender.BuildSSRAST(ctx, svelterender.SSROptions{
		Root:   projectRoot,
		OutDir: astOutDir,
		Jobs:   jobs,
	})
	if err != nil {
		return result, fmt.Errorf("codegen: ssr ast: %w", err)
	}
	resultsByRoute := make(map[string]string, len(results))
	for _, r := range results {
		resultsByRoute[r.Route] = r.Output
	}

	companionDropped := make(map[string]struct{})
	emitted := make(map[string]string, len(transpilePlan))
	for _, p := range transpilePlan {
		astPath, ok := resultsByRoute[p.route.Pattern]
		if !ok {
			return result, fmt.Errorf("codegen: ssr ast missing for route %s", p.route.Pattern)
		}
		envelope, err := os.ReadFile(astPath) //nolint:gosec // path is sidecar-emitted under .gen
		if err != nil {
			return result, fmt.Errorf("codegen: read ssr ast %s: %w", astPath, err)
		}

		typedParam := p.shape.RootType
		lowerer := sveltejs2go.NewLowerer(&p.shape, sveltejs2go.LowererOptions{
			Route:  p.route.Pattern,
			Strict: true,
		})

		encodedSub := strings.TrimPrefix(p.route.PackagePath, ".gen/")
		pkgDir := filepath.Join(projectRoot, outDir, "usersrc", filepath.FromSlash(encodedSub))
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			return result, fmt.Errorf("codegen: mkdir %s: %w", pkgDir, err)
		}

		out, err := sveltejs2go.Transpile(envelope, sveltejs2go.Options{
			PackageName:        p.route.PackageName,
			FuncName:           "Render",
			Rewriter:           lowerer,
			TypedDataParam:     typedParam,
			EmitPageStateParam: true,
			CSRFAutoInject:     routeCSRFEnabled(p.route.Pattern, routeOptions),
			ImageVariants:      imageVariants,
		})
		if err != nil {
			return result, fmt.Errorf("codegen: ssr transpile %s: %w (annotate the route with <!-- sveltego:ssr-fallback --> to opt into the sidecar fallback)", p.route.Pattern, err)
		}
		if lerr := lowerer.Err(); lerr != nil {
			return result, fmt.Errorf("codegen: ssr lowering %s: %w", p.route.Pattern, lerr)
		}

		target := filepath.Join(pkgDir, "page_render.gen.go")
		if err := os.WriteFile(target, out, genFileMode); err != nil {
			return result, fmt.Errorf("codegen: write %s: %w", target, err)
		}

		if _, done := companionDropped[pkgDir]; !done {
			companion := sveltejs2go.CompanionFile(p.route.PackageName)
			compPath := filepath.Join(pkgDir, "ssr_companion.gen.go")
			if err := os.WriteFile(compPath, companion, genFileMode); err != nil {
				return result, fmt.Errorf("codegen: write %s: %w", compPath, err)
			}
			companionDropped[pkgDir] = struct{}{}
		}

		if p.synthetic {
			if err := emitSyntheticPageData(pkgDir, p.route.PackageName); err != nil {
				return result, err
			}
		}

		wireDir := filepath.Join(projectRoot, outDir, filepath.FromSlash(encodedSub))
		if err := emitSSRWire(outDir, modulePath, mirrorRoute{
			encodedSubpath: encodedSub,
			packageName:    p.route.PackageName,
			wireDir:        wireDir,
			hasSSRRender:   true,
		}); err != nil {
			return result, err
		}

		emitted[p.route.Pattern] = encodedSub
	}
	result.Transpiled = emitted

	if len(layoutPlans) == 0 {
		return result, nil
	}
	for _, lp := range layoutPlans {
		jobKey := layoutJobKey(lp.pkgPath)
		astPath, ok := resultsByRoute[jobKey]
		if !ok {
			return result, fmt.Errorf("codegen: ssr ast missing for layout %s", lp.pkgPath)
		}
		envelope, err := os.ReadFile(astPath) //nolint:gosec // path is sidecar-emitted under .gen
		if err != nil {
			return result, fmt.Errorf("codegen: read ssr ast %s: %w", astPath, err)
		}

		shape := lp.shape
		lowerer := sveltejs2go.NewLowerer(&shape, sveltejs2go.LowererOptions{
			Route:  lp.pkgPath,
			Strict: true,
		})

		encodedSub := strings.TrimPrefix(lp.pkgPath, ".gen/")
		pkgDir := filepath.Join(projectRoot, outDir, "layoutsrc", filepath.FromSlash(encodedSub))
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			return result, fmt.Errorf("codegen: mkdir %s: %w", pkgDir, err)
		}

		out, err := sveltejs2go.Transpile(envelope, sveltejs2go.Options{
			PackageName:        lp.pkgName,
			FuncName:           "Render",
			Rewriter:           lowerer,
			TypedDataParam:     shape.RootType,
			EmitChildrenParam:  true,
			EmitPageStateParam: true,
			// Layouts span multiple routes whose CSRF flags may
			// differ; enable inject unconditionally. pageState.CSRFToken
			// is empty when CSRF is disabled for the bound page so the
			// hidden input renders with an empty value (harmless because
			// the server skips validation on opted-out routes).
			CSRFAutoInject: true,
			ImageVariants:  imageVariants,
		})
		if err != nil {
			return result, fmt.Errorf("codegen: ssr layout transpile %s: %w", lp.pkgPath, err)
		}
		if lerr := lowerer.Err(); lerr != nil {
			return result, fmt.Errorf("codegen: ssr layout lowering %s: %w", lp.pkgPath, lerr)
		}

		target := filepath.Join(pkgDir, "layout_render.gen.go")
		if err := os.WriteFile(target, out, genFileMode); err != nil {
			return result, fmt.Errorf("codegen: write %s: %w", target, err)
		}

		if _, done := companionDropped[pkgDir]; !done {
			companion := sveltejs2go.CompanionFile(lp.pkgName)
			compPath := filepath.Join(pkgDir, "ssr_companion.gen.go")
			if err := os.WriteFile(compPath, companion, genFileMode); err != nil {
				return result, fmt.Errorf("codegen: write %s: %w", compPath, err)
			}
			companionDropped[pkgDir] = struct{}{}
		}

		if lp.serverFile == "" {
			if err := emitSyntheticLayoutData(pkgDir, lp.pkgName); err != nil {
				return result, err
			}
		}

		wireDir := filepath.Join(projectRoot, outDir, filepath.FromSlash(encodedSub))
		if err := emitSSRLayoutWire(outDir, modulePath, mirrorRoute{
			encodedSubpath: encodedSub,
			packageName:    lp.pkgName,
			wireDir:        wireDir,
		}); err != nil {
			return result, err
		}
	}

	if len(errorPlans) == 0 {
		return result, nil
	}
	for _, ep := range errorPlans {
		jobKey := errorJobKey(ep.pkgPath)
		astPath, ok := resultsByRoute[jobKey]
		if !ok {
			return result, fmt.Errorf("codegen: ssr ast missing for error %s", ep.pkgPath)
		}
		envelope, err := os.ReadFile(astPath) //nolint:gosec // path is sidecar-emitted under .gen
		if err != nil {
			return result, fmt.Errorf("codegen: read ssr ast %s: %w", astPath, err)
		}

		shape := ep.shape
		lowerer := sveltejs2go.NewLowerer(&shape, sveltejs2go.LowererOptions{
			Route:  ep.pkgPath,
			Strict: true,
		})

		encodedSub := strings.TrimPrefix(ep.pkgPath, ".gen/")
		pkgDir := filepath.Join(projectRoot, outDir, "errorsrc", filepath.FromSlash(encodedSub))
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			return result, fmt.Errorf("codegen: mkdir %s: %w", pkgDir, err)
		}

		out, err := sveltejs2go.Transpile(envelope, sveltejs2go.Options{
			PackageName:        ep.pkgName,
			FuncName:           "Render",
			Rewriter:           lowerer,
			TypedDataParam:     shape.RootType,
			EmitPageStateParam: true,
			ImageVariants:      imageVariants,
		})
		if err != nil {
			return result, fmt.Errorf("codegen: ssr error transpile %s: %w", ep.pkgPath, err)
		}
		if lerr := lowerer.Err(); lerr != nil {
			return result, fmt.Errorf("codegen: ssr error lowering %s: %w", ep.pkgPath, lerr)
		}

		target := filepath.Join(pkgDir, "error_render.gen.go")
		if err := os.WriteFile(target, out, genFileMode); err != nil {
			return result, fmt.Errorf("codegen: write %s: %w", target, err)
		}

		if _, done := companionDropped[pkgDir]; !done {
			companion := sveltejs2go.CompanionFile(ep.pkgName)
			compPath := filepath.Join(pkgDir, "ssr_companion.gen.go")
			if err := os.WriteFile(compPath, companion, genFileMode); err != nil {
				return result, fmt.Errorf("codegen: write %s: %w", compPath, err)
			}
			companionDropped[pkgDir] = struct{}{}
		}

		if err := emitSyntheticErrorData(pkgDir, ep.pkgName); err != nil {
			return result, err
		}

		wireDir := filepath.Join(projectRoot, outDir, filepath.FromSlash(encodedSub))
		if err := emitSSRErrorWire(outDir, modulePath, mirrorRoute{
			encodedSubpath: encodedSub,
			packageName:    ep.pkgName,
			wireDir:        wireDir,
		}); err != nil {
			return result, err
		}
	}
	return result, nil
}

// layoutJobKey derives the BuildSSRAST job key for a layout package
// path. The key feeds the sidecar's `routeSlug` helper which strips
// leading slashes and `__`-joins segments to form an output directory
// name. Page jobs use the route Pattern (e.g. `/longlist`) so layouts
// pick the disjoint `__layout__` namespace to avoid colliding with a
// hypothetical `/layout` page route.
func layoutJobKey(layoutPkgPath string) string {
	return "/__layout__/" + strings.TrimPrefix(layoutPkgPath, ".gen/")
}

// errorJobKey derives the BuildSSRAST job key for an error-boundary
// package path. Mirrors layoutJobKey but uses the disjoint `__error__`
// namespace so a hypothetical `/error` page route never collides.
func errorJobKey(errorPkgPath string) string {
	return "/__error__/" + strings.TrimPrefix(errorPkgPath, ".gen/")
}

// emitSyntheticLayoutData drops a tiny `layout_synthetic.gen.go` into
// pkgDir declaring `type LayoutData = struct{}`. The wire-emitted
// `RenderLayoutSSR` references `usersrc.LayoutData` unconditionally;
// layouts without a `_layout.server.go` mirror have no other source for
// the type, so the synthetic alias keeps the typed-data ABI uniform
// without forcing every layout to author a server file.
func emitSyntheticLayoutData(pkgDir, pkgName string) error {
	src := "// Code generated by sveltego. DO NOT EDIT.\n\npackage " + pkgName + "\n\ntype LayoutData = struct{}\n"
	target := filepath.Join(pkgDir, "layout_synthetic.gen.go")
	if err := os.WriteFile(target, []byte(src), genFileMode); err != nil {
		return fmt.Errorf("codegen: write %s: %w", target, err)
	}
	return nil
}

// emitSyntheticPageData drops `page_synthetic.gen.go` into pkgDir
// declaring `type PageData = struct{}`. Used for Prerender routes
// (#467) that ship without a sibling `_page.server.go`: the transpiled
// page_render.gen.go references `PageData` unconditionally because the
// emitter binds the typed parameter at codegen time, so the synthetic
// alias keeps the route compiling without forcing every static page to
// author a server file.
func emitSyntheticPageData(pkgDir, pkgName string) error {
	src := "// Code generated by sveltego. DO NOT EDIT.\n\npackage " + pkgName + "\n\ntype PageData = struct{}\n"
	target := filepath.Join(pkgDir, "page_synthetic.gen.go")
	if err := os.WriteFile(target, []byte(src), genFileMode); err != nil {
		return fmt.Errorf("codegen: write %s: %w", target, err)
	}
	return nil
}

// emitSyntheticErrorData drops `error_synthetic.gen.go` into pkgDir
// declaring `type ErrorData = kit.SafeError`. The transpiled
// error_render.gen.go is emitted with `TypedDataParam: "ErrorData"` so
// the lowered `data.<field>` chain resolves against this alias rather
// than referencing kit directly (which would require importing kit
// into a file the emitter does not own). The alias keeps the typed-data
// ABI uniform with the page/layout SSR paths.
func emitSyntheticErrorData(pkgDir, pkgName string) error {
	src := "// Code generated by sveltego. DO NOT EDIT.\n\npackage " + pkgName + "\n\nimport \"github.com/binsarjr/sveltego/packages/sveltego/exports/kit\"\n\ntype ErrorData = kit.SafeError\n"
	target := filepath.Join(pkgDir, "error_synthetic.gen.go")
	if err := os.WriteFile(target, []byte(src), genFileMode); err != nil {
		return fmt.Errorf("codegen: write %s: %w", target, err)
	}
	return nil
}

// planSSR partitions routescan.Routes into the build-time transpile
// plan and the request-time sidecar fallback plan.
//
// Transpile plan: pure-Svelte page routes that satisfy one of two
// shape-source contracts:
//
//   - Live SSR routes (Phase 6 #428): NOT prerendered, NOT SSR-disabled,
//     NOT annotated with `<!-- sveltego:ssr-fallback -->`, AND own a
//     sibling `_page.server.go` with at least one PageData field. The
//     PageServer constraint is the typed-data-receipt invariant from
//     Phase 5 (#427); without it the typegen Shape is empty and the
//     Lowerer cannot map the JS member chain onto Go fields.
//   - Prerender routes (#467): Templates=svelte AND Prerender=true (or
//     PrerenderAuto=true). The route goes through the same Render emit
//     so `Server.Prerender` finds a non-nil Page closure on the manifest
//     and can write each <route>/index.html. When a sibling
//     `_page.server.go` declares a non-empty PageData it drives the
//     Lowerer; otherwise the plan is marked synthetic so a tiny empty
//     `PageData = struct{}` alias is emitted alongside the transpiled
//     output. SSRFallback annotation still wins over the prerender
//     path so the escape hatch survives.
//
// Fallback plan: any pure-Svelte page route that declares the
// `<!-- sveltego:ssr-fallback -->` annotation. The annotation overrides
// the transpile path even when the route would otherwise qualify; this
// is the explicit escape hatch for shapes the transpiler cannot lower.
func planSSR(scan *routescan.ScanResult, routeOptions map[string]kit.PageOptions) ([]ssrPlan, []SSRFallbackRoute) {
	plans := make([]ssrPlan, 0, len(scan.Routes))
	fallback := make([]SSRFallbackRoute, 0)
	for _, r := range scan.Routes {
		if !r.HasPage {
			continue
		}
		opts, ok := routeOptions[r.Pattern]
		if !ok {
			continue
		}
		if opts.Templates != kit.TemplatesSvelte {
			continue
		}
		prerender := opts.Prerender || opts.PrerenderAuto
		if !prerender && !opts.SSR {
			continue
		}

		if r.SSRFallback {
			fallback = append(fallback, SSRFallbackRoute{
				Pattern: r.Pattern,
				Source:  filepath.ToSlash(filepath.Join(r.Dir, "_page.svelte")),
			})
			continue
		}

		var (
			shape     typegen.Shape
			synthetic bool
		)
		if r.HasPageServer {
			s, _, err := typegen.BuildShape(filepath.Join(r.Dir, "_page.server.go"), typegen.KindPage)
			if err == nil && len(s.Types) > 0 && len(s.Types[s.RootType].Fields) > 0 {
				shape = s
			}
		}
		if shape.RootType == "" {
			if !prerender {
				continue
			}
			shape = typegen.Shape{
				RootType: "PageData",
				Types: map[string]typegen.ShapeType{
					"PageData": {Name: "PageData", Fields: nil},
				},
			}
			synthetic = true
		}

		plans = append(plans, ssrPlan{route: r, shape: shape, synthetic: synthetic})
	}
	return plans, fallback
}

// planSSRLayouts collects every unique layout package referenced by a
// pure-Svelte SSR-eligible route (#456, #478). Layouts shared between
// sibling routes are deduplicated by package path.
//
// Eligibility matches planSSR's predicate MINUS the SSRFallback gate:
// the page may opt out of build-time transpile via
// `<!-- sveltego:ssr-fallback -->`, but its layout chain still renders
// Go-side in both Phase 6 (transpile) and Phase 8 (sidecar fallback)
// paths. Decoupling layout eligibility from page transpile lets blog
// — where every page is fallback-annotated — still SSR-transpile its
// root layout instead of falling back to Mustache-Go (#478).
//
// A layout with `_layout.server.go` drives shape inference via typegen
// (KindLayout); a layout without a server file synthesises an empty
// shape so the Lowerer leaves bare expressions alone but still hard-
// errors on `data.X` access (matching the page contract).
func planSSRLayouts(scan *routescan.ScanResult, routeOptions map[string]kit.PageOptions) []layoutPlan {
	if scan == nil {
		return nil
	}
	seen := make(map[string]struct{})
	plans := make([]layoutPlan, 0)
	for _, r := range scan.Routes {
		if !routeEligibleForSSRChain(r, routeOptions) {
			continue
		}
		for i, layoutDir := range r.LayoutChain {
			if i >= len(r.LayoutPackagePaths) {
				continue
			}
			pkgPath := r.LayoutPackagePaths[i]
			if _, done := seen[pkgPath]; done {
				continue
			}
			seen[pkgPath] = struct{}{}

			serverFile := ""
			if i < len(r.LayoutServerFiles) {
				serverFile = r.LayoutServerFiles[i]
			}
			pkgName := layoutPackageName(pkgPath)
			lp := layoutPlan{
				dir:        layoutDir,
				pkgPath:    pkgPath,
				pkgName:    pkgName,
				serverFile: serverFile,
			}
			if serverFile != "" {
				shape, _, err := typegen.BuildShape(serverFile, typegen.KindLayout)
				if err == nil {
					lp.shape = shape
				}
			}
			if lp.shape.RootType == "" {
				lp.shape = typegen.Shape{
					RootType: "LayoutData",
					Types: map[string]typegen.ShapeType{
						"LayoutData": {Name: "LayoutData", Fields: nil},
					},
				}
			}
			plans = append(plans, lp)
		}
	}
	return plans
}

// planSSRErrors collects every unique error-boundary package
// referenced by a pure-Svelte SSR-eligible route (#412, #478).
// Boundaries shared between sibling routes are deduplicated by package
// path.
//
// Eligibility matches planSSR's predicate MINUS the SSRFallback gate
// (see planSSRLayouts for the rationale). Error boundaries always
// render Go-side regardless of how the page itself reached the runtime,
// so a fallback-annotated page must still surface its boundary through
// the SSR transpile path.
//
// All error templates receive the same synthetic ErrorData shape — an
// alias to kit.SafeError — so the Lowerer rewrites `data.code` →
// `data.Code`, `data.message` → `data.Message`, `data.id` → `data.ID`.
// The shape's field names mirror typegen's lowerFirst-of-Go-field
// convention so user templates stay JS-camelCase as expected.
func planSSRErrors(scan *routescan.ScanResult, routeOptions map[string]kit.PageOptions) []errorPlan {
	if scan == nil {
		return nil
	}
	shape := errorDataShape()
	seen := make(map[string]struct{})
	plans := make([]errorPlan, 0)
	for _, r := range scan.Routes {
		if !routeEligibleForSSRChain(r, routeOptions) {
			continue
		}
		if r.ErrorBoundaryPackagePath == "" || r.ErrorBoundaryDir == "" {
			continue
		}
		if _, done := seen[r.ErrorBoundaryPackagePath]; done {
			continue
		}
		seen[r.ErrorBoundaryPackagePath] = struct{}{}
		plans = append(plans, errorPlan{
			dir:     r.ErrorBoundaryDir,
			pkgPath: r.ErrorBoundaryPackagePath,
			pkgName: layoutPackageName(r.ErrorBoundaryPackagePath),
			shape:   shape,
		})
	}
	return plans
}

// routeCSRFEnabled reports whether the per-route options enable CSRF
// for pattern. Missing entries fall back to the framework default
// (kit.DefaultPageOptions().CSRF). Used by the SSR transpile driver to
// decide whether to run the CSRF auto-inject pre-pass over a page
// route's AST (issue #493).
func routeCSRFEnabled(pattern string, routeOptions map[string]kit.PageOptions) bool {
	opts, ok := routeOptions[pattern]
	if !ok {
		return kit.DefaultPageOptions().CSRF
	}
	return opts.CSRF
}

// routeEligibleForSSRChain reports whether a route's layout chain and
// error boundary should travel the SSR Option B transpile path. The
// predicate mirrors planSSR's Templates+SSR/Prerender check but
// deliberately ignores `r.SSRFallback`: fallback-annotated pages still
// render their layouts and errors Go-side at request time, so the
// Phase 6 transpile path must cover their chain-mates too (#478).
//
// Returns false for non-page routes (REST endpoints), Mustache-template
// routes, and routes that opt out of SSR entirely.
func routeEligibleForSSRChain(r routescan.ScannedRoute, routeOptions map[string]kit.PageOptions) bool {
	if !r.HasPage {
		return false
	}
	opts, ok := routeOptions[r.Pattern]
	if !ok {
		return false
	}
	if opts.Templates != kit.TemplatesSvelte {
		return false
	}
	return opts.SSR || opts.Prerender || opts.PrerenderAuto
}

// projectImageWidths returns the effective ImageWidths for the
// project's image variant pipeline (issue #492). Per
// [kit.PageOptions.ImageWidths], the field is project-global rather
// than per-route — variants share a single static/_app/immutable/
// pool. Pick the first non-empty ImageWidths from any route's
// effective options; an empty result lets the pipeline fall back to
// [images.DefaultWidths].
func projectImageWidths(routeOptions map[string]kit.PageOptions) []int {
	for _, opts := range routeOptions {
		if len(opts.ImageWidths) > 0 {
			out := make([]int, len(opts.ImageWidths))
			copy(out, opts.ImageWidths)
			return out
		}
	}
	return nil
}

// errorDataShape returns the synthetic typegen Shape used by the
// Lowerer when it walks `_error.svelte` member chains. The shape
// describes ErrorData (alias to kit.SafeError) so JS-camelCase access
// `data.code` / `data.message` / `data.id` lowers onto the Go-side
// fields `Code` / `Message` / `ID`. Field.Name is the JSON-tag-style
// identifier the lowerer matches; GoName is the Go field identifier
// the rewriter substitutes.
func errorDataShape() typegen.Shape {
	return typegen.Shape{
		RootType: "ErrorData",
		Types: map[string]typegen.ShapeType{
			"ErrorData": {
				Name: "ErrorData",
				Fields: []typegen.Field{
					{Name: "code", GoName: "Code", GoType: "int", TSType: "number"},
					{Name: "message", GoName: "Message", GoType: "string", TSType: "string"},
					{Name: "id", GoName: "ID", GoType: "string", TSType: "string"},
				},
			},
		},
	}
}
