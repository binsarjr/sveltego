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
// property-access lowering. Plan items only exist for non-prerendered
// Svelte-mode routes that own a sibling _page.server.go with a
// non-empty PageData (the typed-data receipt strategy needs a struct
// to land on; an empty shape would map every member access to a hard
// error which is the wrong default before Phase 8's fallback lands).
type ssrPlan struct {
	route routescan.ScannedRoute
	shape typegen.Shape
}

// SSRPlanResult is the outcome of the Phase 6/8 SSR planner. Transpiled
// holds routes that received a build-time JS→Go Render emit and the
// encoded subpath where it lives (consumed by manifest emission).
// Fallback holds routes that explicitly opted out via the
// `<!-- sveltego:ssr-fallback -->` comment; those route through the
// long-running Node sidecar at request time (Phase 8, #430).
//
// Layouts holds layout package paths (with the ".gen/" prefix, matching
// LayoutPackagePaths) that received a wire_layout_render.gen.go emit so
// the manifest can swap their adapter from the legacy
// `Layout{}.Render(*render.Writer, ...)` form to the children-callback
// payload bridge wired via #453 (issue #456).
type SSRPlanResult struct {
	Transpiled map[string]string
	Fallback   []SSRFallbackRoute
	Layouts    map[string]struct{}
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
	layoutPlans := planSSRLayouts(scan, transpilePlan)
	result := SSRPlanResult{Fallback: fallback}
	if len(transpilePlan) == 0 && len(fallback) == 0 && len(layoutPlans) == 0 {
		return result, nil
	}
	if len(transpilePlan) == 0 && len(layoutPlans) == 0 {
		logger.Info("ssr fallback only: no routes transpiled to Go",
			logKeyFallbackCount, len(fallback))
		return result, nil
	}
	if _, err := svelterender.EnsureNode(); err != nil {
		return result, fmt.Errorf("codegen: ssr requires node 18+ on $PATH (or annotate routes with <!-- sveltego:ssr-fallback --> if they intentionally bypass the transpiler): %w", err)
	}

	jobs := make([]svelterender.SSRJob, 0, len(transpilePlan)+len(layoutPlans))
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
			PackageName:    p.route.PackageName,
			FuncName:       "Render",
			Rewriter:       lowerer,
			TypedDataParam: typedParam,
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
	layoutsByPkg := make(map[string]struct{}, len(layoutPlans))
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
			PackageName:       lp.pkgName,
			FuncName:          "Render",
			Rewriter:          lowerer,
			TypedDataParam:    shape.RootType,
			EmitChildrenParam: true,
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

		layoutsByPkg[lp.pkgPath] = struct{}{}
	}
	result.Layouts = layoutsByPkg
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

// planSSR partitions routescan.Routes into the build-time transpile
// plan and the request-time sidecar fallback plan.
//
// Transpile plan: pure-Svelte page routes that are NOT prerendered, NOT
// SSR-disabled, NOT annotated with `<!-- sveltego:ssr-fallback -->`,
// AND own a sibling `_page.server.go` with at least one PageData field.
// The PageServer constraint is the typed-data-receipt invariant from
// Phase 5 (#427); without it the typegen Shape is empty and the Lowerer
// cannot map the JS member chain onto Go fields.
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
		if opts.Prerender || opts.PrerenderAuto {
			continue
		}
		if !opts.SSR {
			continue
		}

		if r.SSRFallback {
			fallback = append(fallback, SSRFallbackRoute{
				Pattern: r.Pattern,
				Source:  filepath.ToSlash(filepath.Join(r.Dir, "_page.svelte")),
			})
			continue
		}

		if !r.HasPageServer {
			continue
		}

		shape, _, err := typegen.BuildShape(filepath.Join(r.Dir, "_page.server.go"), typegen.KindPage)
		if err != nil {
			continue
		}
		if len(shape.Types) == 0 || len(shape.Types[shape.RootType].Fields) == 0 {
			continue
		}

		plans = append(plans, ssrPlan{route: r, shape: shape})
	}
	return plans, fallback
}

// planSSRLayouts collects every unique layout package referenced by a
// route that qualifies for the Phase 6 SSR transpile (#456). Layouts
// shared between sibling routes are deduplicated by package path; only
// layouts reachable from a Svelte-SSR-eligible page route are included.
//
// A layout with `_layout.server.go` drives shape inference via typegen
// (KindLayout); a layout without a server file synthesises an empty
// shape so the Lowerer leaves bare expressions alone but still hard-
// errors on `data.X` access (matching the page contract).
func planSSRLayouts(scan *routescan.ScanResult, pagePlans []ssrPlan) []layoutPlan {
	if scan == nil {
		return nil
	}
	pageRoutes := make(map[string]struct{}, len(pagePlans))
	for _, p := range pagePlans {
		pageRoutes[p.route.Pattern] = struct{}{}
	}
	seen := make(map[string]struct{})
	plans := make([]layoutPlan, 0)
	for _, r := range scan.Routes {
		if _, ok := pageRoutes[r.Pattern]; !ok {
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
