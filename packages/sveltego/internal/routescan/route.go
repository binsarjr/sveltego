package routescan

import "github.com/binsarjr/sveltego/runtime/router"

// ScannedRoute is the scanner's view of a single route directory before
// the manifest emitter consumes it. It carries the parsed router segments
// and the per-special-file flags needed to decide what to emit.
//
// LayoutChain is ordered ancestor -> self: the first entry is the topmost
// directory under RoutesDir that owns a +layout.svelte; the last entry is
// the route's own directory when the route also owns a layout.
//
// LayoutPackagePaths runs in lockstep with LayoutChain and holds each
// layout dir's encoded gen-tree package path (e.g. ".gen/routes",
// ".gen/routes/_g_app"). It enables the manifest emitter to import the
// generated layout package without re-deriving the encoding.
type ScannedRoute struct {
	Pattern            string
	Segments           []router.Segment
	Dir                string
	PackageName        string
	PackagePath        string
	HasPage            bool
	HasLayout          bool
	HasError           bool
	HasReset           bool
	HasPageServer      bool
	HasLayoutServer    bool
	HasServer          bool
	LayoutChain        []string
	LayoutPackagePaths []string
}

// DiscoveredMatcher names a Go file under src/params/ that exports
// `func Match(s string) bool`. Path is the absolute filesystem path.
type DiscoveredMatcher struct {
	Name string
	Path string
}

// ScanInput configures one scan. RoutesDir is required and must be an
// absolute path; ParamsDir is optional — when empty, matcher discovery is
// skipped silently.
type ScanInput struct {
	RoutesDir string
	ParamsDir string
}

// ScanResult carries the aggregated output of Scan: every route directory
// that owns at least one special file, every discovered matcher, and any
// recoverable diagnostics. All slices are sorted deterministically.
type ScanResult struct {
	Routes      []ScannedRoute
	Matchers    []DiscoveredMatcher
	Diagnostics []Diagnostic
}
