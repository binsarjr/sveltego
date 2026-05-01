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

// runSSRTranspile drives the Phase 6 (#428) SSR codegen pipeline:
// detect Svelte routes that need SSR, run the Node sidecar in SSR mode
// once for the entire batch, and emit one Render(payload, data) Go
// file per route under .gen/usersrc/<encoded-pkg>/page_render.gen.go.
//
// Returns the set of route patterns that successfully received an SSR
// Render emit. The manifest emitter consumes this set to decide which
// routes wire the typed Page render adapter (vs falling through to the
// legacy SPA shell).
//
// Node-missing is treated as a soft fallback: the function logs a
// warning via the build's logger and returns nil, so the pipeline
// continues to ship SPA-mode shells. This keeps `sveltego build`
// usable on hosts without Node while the deploy adapters and v0.6
// auth track land. ADR 0009 mandates a hard error for unrecognised
// emit shapes; that path runs inside Transpile/Lowerer once Node is
// available.
func runSSRTranspile(ctx context.Context, projectRoot, outDir, modulePath string, logger *slog.Logger, scan *routescan.ScanResult, routeOptions map[string]kit.PageOptions) (map[string]string, error) {
	plan := planSSR(scan, routeOptions)
	if len(plan) == 0 {
		return nil, nil
	}
	// Probe Node up front so a missing toolchain logs once per build,
	// not once per route.
	if _, err := svelterender.EnsureNode(); err != nil {
		logger.Warn("ssr codegen skipped: node binary not on $PATH; routes will fall back to SPA shell",
			logKeyDiagnostic, err.Error())
		return nil, nil
	}

	jobs := make([]svelterender.SSRJob, 0, len(plan))
	for _, p := range plan {
		rel, err := filepath.Rel(projectRoot, filepath.Join(p.route.Dir, "_page.svelte"))
		if err != nil {
			return nil, fmt.Errorf("codegen: ssr rel path: %w", err)
		}
		jobs = append(jobs, svelterender.SSRJob{
			Route:  p.route.Pattern,
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
		// Sidecar-deps-missing or sidecar-source-tree-missing are
		// build-host configuration errors; treat them as the same
		// soft-fallback as Node-missing so the build completes and
		// each route serves the SPA shell. Operators see the warning
		// and can run `npm install` in the sidecar tree.
		logger.Warn("ssr ast skipped; routes fall back to SPA shell",
			logKeyDiagnostic, err.Error())
		return nil, nil
	}
	resultsByRoute := make(map[string]string, len(results))
	for _, r := range results {
		resultsByRoute[r.Route] = r.Output
	}

	// Track which encoded package directories already received the
	// CompanionFile so we drop it once per package, not once per route
	// in that package.
	companionDropped := make(map[string]struct{})
	emitted := make(map[string]string, len(plan))
	for _, p := range plan {
		astPath, ok := resultsByRoute[p.route.Pattern]
		if !ok {
			return nil, fmt.Errorf("codegen: ssr ast missing for route %s", p.route.Pattern)
		}
		envelope, err := os.ReadFile(astPath) //nolint:gosec // path is sidecar-emitted under .gen
		if err != nil {
			return nil, fmt.Errorf("codegen: read ssr ast %s: %w", astPath, err)
		}

		typedParam := p.shape.RootType
		lowerer := sveltejs2go.NewLowerer(&p.shape, sveltejs2go.LowererOptions{
			Route:  p.route.Pattern,
			Strict: true,
		})

		encodedSub := strings.TrimPrefix(p.route.PackagePath, ".gen/")
		// SSR Render lives in the same usersrc package as the user's
		// PageData declaration. That package is the typegen shape's
		// declaration site, which keeps the typed parameter resolvable
		// without re-export.
		pkgDir := filepath.Join(projectRoot, outDir, "usersrc", filepath.FromSlash(encodedSub))
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			return nil, fmt.Errorf("codegen: mkdir %s: %w", pkgDir, err)
		}

		out, err := sveltejs2go.Transpile(envelope, sveltejs2go.Options{
			PackageName:    p.route.PackageName,
			FuncName:       "Render",
			Rewriter:       lowerer,
			TypedDataParam: typedParam,
		})
		if err != nil {
			// Phase 8 (#430) lands the explicit `// sveltego:ssr-fallback`
			// opt-out so routes the transpiler cannot lower can route
			// through the long-running sidecar at request time. Until
			// then, treat unknown shapes as a soft fallback: log the
			// failure and let the route serve the SPA shell. Hard-error
			// remains the goal once Phase 8 ships and the corpus is
			// validated against Phase 5's coverage.
			logger.Warn("ssr codegen skipped; route falls back to SPA shell",
				logKeyDiagnostic, fmt.Sprintf("route=%s err=%v", p.route.Pattern, err))
			continue
		}
		if lerr := lowerer.Err(); lerr != nil {
			logger.Warn("ssr lowering skipped; route falls back to SPA shell",
				logKeyDiagnostic, fmt.Sprintf("route=%s err=%v", p.route.Pattern, lerr))
			continue
		}

		target := filepath.Join(pkgDir, "page_render.gen.go")
		if err := os.WriteFile(target, out, genFileMode); err != nil {
			return nil, fmt.Errorf("codegen: write %s: %w", target, err)
		}

		if _, done := companionDropped[pkgDir]; !done {
			companion := sveltejs2go.CompanionFile(p.route.PackageName)
			compPath := filepath.Join(pkgDir, "ssr_companion.gen.go")
			if err := os.WriteFile(compPath, companion, genFileMode); err != nil {
				return nil, fmt.Errorf("codegen: write %s: %w", compPath, err)
			}
			companionDropped[pkgDir] = struct{}{}
		}

		// Drop a wire bridge in the gen-side route package that wraps
		// the typed Render into a data-erased adapter the manifest can
		// reference. This is the per-route SSR analogue of emitWire.
		wireDir := filepath.Join(projectRoot, outDir, filepath.FromSlash(encodedSub))
		if err := emitSSRWire(outDir, modulePath, mirrorRoute{
			encodedSubpath: encodedSub,
			packageName:    p.route.PackageName,
			wireDir:        wireDir,
			hasSSRRender:   true,
		}); err != nil {
			return nil, err
		}

		emitted[p.route.Pattern] = encodedSub
	}
	return emitted, nil
}

// planSSR filters routescan.Routes to the subset that needs Phase 6 SSR
// codegen: pure-Svelte page routes that are NOT prerendered, NOT
// SSR-disabled, AND own a sibling _page.server.go. The PageServer
// constraint is the typed-data-receipt invariant from Phase 5 (#427)
// — without it the typegen Shape is empty and the Lowerer cannot map
// the JS member chain onto Go fields. Routes without _page.server.go
// continue to render via the SPA shell until either the user adds a
// Load() or Phase 8 (#430) sidecar fallback lands.
//
// Routes opted into // sveltego:ssr-fallback (Phase 8) are excluded
// here once that comment is recognised; for now every qualifying
// Svelte page route is in scope.
func planSSR(scan *routescan.ScanResult, routeOptions map[string]kit.PageOptions) []ssrPlan {
	out := make([]ssrPlan, 0, len(scan.Routes))
	for _, r := range scan.Routes {
		if !r.HasPage || !r.HasPageServer {
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

		shape, _, err := typegen.BuildShape(filepath.Join(r.Dir, "_page.server.go"), typegen.KindPage)
		if err != nil {
			// Failure to read the typed shape is a recoverable case:
			// the route still renders via the SPA shell. Don't fail
			// the build for this — a missing PageData type just means
			// the user has not declared one and the route's data is
			// untyped.
			continue
		}
		if len(shape.Types) == 0 || len(shape.Types[shape.RootType].Fields) == 0 {
			// No PageData fields → typed receipt is `struct{}` which
			// the .svelte template typically doesn't reference. Skip
			// SSR for these until the user populates Load(); the SPA
			// shell still renders.
			continue
		}

		out = append(out, ssrPlan{route: r, shape: shape})
	}
	return out
}
