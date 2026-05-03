package routescan

import "github.com/binsarjr/sveltego/packages/sveltego/runtime/router"

// ScannedRoute is the scanner's view of a single route directory before
// the manifest emitter consumes it. It carries the parsed router segments
// and the per-special-file flags needed to decide what to emit.
//
// LayoutChain is ordered ancestor -> self: the first entry is the topmost
// directory under RoutesDir that owns a _layout.svelte; the last entry is
// the route's own directory when the route also owns a layout.
//
// LayoutPackagePaths runs in lockstep with LayoutChain and holds each
// layout dir's encoded gen-tree package path (e.g. ".gen/routes",
// ".gen/routes/_g_app"). It enables the manifest emitter to import the
// generated layout package without re-deriving the encoding.
//
// LayoutServerFiles also runs in lockstep with LayoutChain. Each entry is
// the absolute path to <layoutDir>/_layout.server.go when that file exists,
// otherwise the empty string. The codegen emitter uses these paths to
// mirror layout server sources and emit per-layout Load wires.
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
	LayoutServerFiles  []string
	// ResetTarget records the @<target> suffix parsed from a reset
	// filename like _page@(app).svelte. The empty string means a root
	// reset (skip every intermediate layout); a non-empty value names
	// the group whose _layout.svelte the chain truncates at, inclusive.
	// Meaningful only when HasReset is true.
	ResetTarget string
	// ErrorBoundaryDir is the absolute path of the nearest ancestor (or
	// self) directory that owns a _error.svelte. Empty when no error
	// boundary covers this route.
	ErrorBoundaryDir string
	// ErrorBoundaryPackagePath is the .gen package path for
	// ErrorBoundaryDir, mirroring how layout package paths are encoded.
	// Empty when ErrorBoundaryDir is empty.
	ErrorBoundaryPackagePath string
	// ErrorBoundaryLayoutDepth is the count of LayoutChain entries that
	// remain when rendering the boundary: layouts at-or-above the
	// boundary's directory wrap the error template; layouts strictly
	// below the boundary abort. Zero when no boundary applies.
	ErrorBoundaryLayoutDepth int
	// SSRFallback is true when the route's `_page.svelte` declares
	// `<!-- sveltego:ssr-fallback -->`. Annotated routes opt out of the
	// build-time JS→Go transpiler (ADR 0009) and route through the
	// long-running Node sidecar at request time (Phase 8, #430).
	SSRFallback bool
}

// DiscoveredMatcher names a Go file under src/params/<name>/ that
// exports `func Match(s string) bool`. Name is the matcher name (the
// segment after `=` in `[id=int]`); Path is the absolute filesystem
// path of `<name>.go`; PackageName is the file's `package` clause —
// equal to Name by convention so the codegen registry emit can call
// `<name>.Match` after importing the user package by alias.
type DiscoveredMatcher struct {
	Name        string
	Path        string
	PackageName string
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
