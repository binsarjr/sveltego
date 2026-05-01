package codegen

import (
	"fmt"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen/typegen"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/routescan"
)

// runTypegen walks every scanned route and layout and emits the
// sibling `.d.ts` declaration the pure-Svelte template imports.
//
// RFC #379 phase 2: the .d.ts files run in parallel to the existing
// Mustache-Go pipeline so the build keeps producing legacy artifacts
// while pure-Svelte authors gain IDE autocompletion. Phase 5 (#384)
// removes the Mustache-Go path; until then this pass writes
// `_page.svelte.d.ts` / `_layout.svelte.d.ts` next to each route's
// `.svelte` file with no influence on the rest of codegen.
//
// Diagnostics surface via the returned slice; the build driver merges
// them with routescan warnings so the user sees them on stderr.
func runTypegen(routes []routescan.ScannedRoute) ([]routescan.Diagnostic, error) {
	var diags []routescan.Diagnostic
	emittedLayouts := map[string]struct{}{}
	for _, route := range routes {
		if route.HasPageServer {
			_, tdiags, err := typegen.EmitForRoute(typegen.EmitOptions{
				RouteDir: route.Dir,
				Kind:     typegen.KindPage,
			})
			if err != nil {
				return nil, fmt.Errorf("codegen: typegen page %s: %w", route.Dir, err)
			}
			diags = append(diags, convertTypegenDiags(tdiags)...)
		}
		for i, layoutDir := range route.LayoutChain {
			if _, done := emittedLayouts[layoutDir]; done {
				continue
			}
			emittedLayouts[layoutDir] = struct{}{}
			if i >= len(route.LayoutServerFiles) || route.LayoutServerFiles[i] == "" {
				continue
			}
			_, tdiags, err := typegen.EmitForRoute(typegen.EmitOptions{
				RouteDir: layoutDir,
				Kind:     typegen.KindLayout,
			})
			if err != nil {
				return nil, fmt.Errorf("codegen: typegen layout %s: %w", layoutDir, err)
			}
			diags = append(diags, convertTypegenDiags(tdiags)...)
		}
	}
	return diags, nil
}

func convertTypegenDiags(in []typegen.Diagnostic) []routescan.Diagnostic {
	out := make([]routescan.Diagnostic, 0, len(in))
	for _, d := range in {
		out = append(out, routescan.Diagnostic{
			Path:    d.Path,
			Message: d.Message,
		})
	}
	return out
}
