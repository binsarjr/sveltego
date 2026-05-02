package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// RoutesManifestFilename is the file MaybePrerenderFromEnv writes
// alongside manifest.json so adapters spawning the user binary can
// enumerate the full route table — including dynamic routes that did
// not produce prerendered HTML. Consumed by adapter-static's subprocess
// runner.
//
// Experimental — see STABILITY.md.
const RoutesManifestFilename = "routes.json"

// RouteSummary projects one entry from the runtime route table into the
// minimal shape adapter tooling needs: pattern plus the prerender,
// SSR, dynamic-segment, and full PageOptions snapshot. Returned by
// Server.Routes.
//
// Experimental — see STABILITY.md.
type RouteSummary struct {
	// Pattern is the canonical SvelteKit-style path, e.g. "/post/[id]".
	Pattern string
	// Prerender mirrors the route's resolved kit.PageOptions.Prerender.
	Prerender bool
	// SSR mirrors the route's resolved kit.PageOptions.SSR.
	SSR bool
	// DynamicParams lists the parameter names for every non-static
	// segment in declaration order. Empty for fully static routes.
	DynamicParams []string
	// PageOptions is the full resolved options snapshot for the route.
	PageOptions kit.PageOptions
}

// Routes returns one RouteSummary per registered route. The slice is a
// fresh allocation; callers may sort or filter without affecting the
// server. Order matches the underlying route table (insertion order).
//
// Experimental — see STABILITY.md.
func (s *Server) Routes() []RouteSummary {
	routes := s.tree.Routes()
	out := make([]RouteSummary, len(routes))
	for i := range routes {
		out[i] = routeSummaryFor(&routes[i])
	}
	return out
}

// writeRoutesManifest writes one JSON file at outDir/RoutesManifestFilename
// holding s.Routes(). The file is the side-channel adapter-static reads
// to surface dynamic routes through the subprocess boundary.
func (s *Server) writeRoutesManifest(outDir string) error {
	if outDir == "" {
		return nil
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("server: mkdir routes manifest: %w", err)
	}
	body, err := json.MarshalIndent(s.Routes(), "", "  ")
	if err != nil {
		return fmt.Errorf("server: marshal routes manifest: %w", err)
	}
	body = append(body, '\n')
	target := filepath.Join(outDir, RoutesManifestFilename)
	if err := os.WriteFile(target, body, 0o644); err != nil { //nolint:gosec
		return fmt.Errorf("server: write routes manifest: %w", err)
	}
	return nil
}

func routeSummaryFor(r *router.Route) RouteSummary {
	var params []string
	for _, seg := range r.Segments {
		switch seg.Kind {
		case router.SegmentParam, router.SegmentOptional, router.SegmentRest:
			params = append(params, seg.Name)
		}
	}
	return RouteSummary{
		Pattern:       r.Pattern,
		Prerender:     r.Options.Prerender,
		SSR:           r.Options.SSR,
		DynamicParams: params,
		PageOptions:   r.Options,
	}
}
