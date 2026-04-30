package router

import (
	"net/http"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/render"
)

// PageHandler renders a +page.svelte for the given context and load data.
type PageHandler func(w *render.Writer, ctx *kit.RenderCtx, data any) error

// PageHeadHandler renders a page's <svelte:head> contributions into a
// side buffer. The pipeline gathers the layout chain's head outputs and
// the page's head into one buffer that injects between <head> and
// </head>. nil entries denote pages without any head content.
type PageHeadHandler func(w *render.Writer, ctx *kit.RenderCtx, data any) error

// LayoutHandler renders a +layout.svelte. It composes outer layouts around
// inner content by writing its template up to <slot />, invoking children,
// then writing the rest. children is non-nil; layout templates dispatch
// the slot lowering through it.
type LayoutHandler func(w *render.Writer, ctx *kit.RenderCtx, data any, children func(*render.Writer) error) error

// LayoutHeadHandler renders a layout's <svelte:head> contributions into
// a side buffer. Runs in lockstep with LayoutChain; nil entries denote
// layouts without any head content.
type LayoutHeadHandler func(w *render.Writer, ctx *kit.RenderCtx, data any) error

// ErrorHandler renders a +error.svelte. It receives the SafeError produced
// by HandleError and writes the error template into w. The pipeline wraps
// the call in any outer layouts that survive the boundary.
type ErrorHandler func(w *render.Writer, ctx *kit.RenderCtx, safe kit.SafeError) error

// ServerHandlers maps HTTP methods to handlers emitted from +server.go.
type ServerHandlers map[string]http.HandlerFunc

// LoadHandler runs the user-written Load() from +page.server.go and
// returns the data threaded into the page render.
type LoadHandler func(ctx *kit.LoadCtx) (any, error)

// LayoutLoadHandler runs the user-written Load() from +layout.server.go.
// One handler per layout in the chain; nil entries denote layouts without
// a sibling layout.server.go and are skipped by the pipeline.
type LayoutLoadHandler func(ctx *kit.LoadCtx) (any, error)

// ActionsHandler returns the typed Actions value declared in
// +page.server.go. The router keeps it as `any` to remain type-erased;
// the dispatcher casts back to the concrete type.
type ActionsHandler func() any

// Route is one entry in the route table built from the codegen-emitted
// manifest. The router never invokes the handler refs; that is the
// dispatcher's job.
type Route struct {
	// ID is an 8-char hex tag derived from an FNV-1a 32-bit hash of Pattern,
	// populated by NewTree. It is a tag for logging and manifest correlation,
	// NOT a collision-free identity. Use Pattern as the unique key.
	ID string
	// Pattern is the SvelteKit-style canonical path, e.g. "/post/[id]/[...rest]".
	Pattern string
	// Segments is the parsed form of Pattern.
	Segments []Segment
	// Page is non-nil when the route owns a +page.svelte.
	Page PageHandler
	// Server holds method handlers when the route owns a +server.go.
	Server ServerHandlers
	// Load is non-nil when the route owns a +page.server.go with Load().
	Load LoadHandler
	// Actions is non-nil when +page.server.go declares Actions().
	Actions ActionsHandler
	// LayoutChain holds the layout handlers wrapping Page, ordered
	// outer -> inner. The server pipeline composes them so the outermost
	// layout owns the document chrome and the page renders innermost.
	LayoutChain []LayoutHandler
	// LayoutLoaders runs in lockstep with LayoutChain. Index i holds the
	// loader for layout chain[i] or nil when that layout has no
	// +layout.server.go. The pipeline invokes them outer -> inner before
	// the page Load and pushes each result onto the LoadCtx parent stack.
	LayoutLoaders []LayoutLoadHandler
	// Head is non-nil when the page contributes <svelte:head> content.
	// The pipeline calls it after the layout chain's head handlers and
	// before injecting the merged buffer into the document <head>.
	Head PageHeadHandler
	// LayoutHeads runs in lockstep with LayoutChain. Index i holds the
	// head handler for layout chain[i] or nil when that layout has no
	// <svelte:head> content.
	LayoutHeads []LayoutHeadHandler
	// Options carries the route's effective page options after the
	// codegen-time cascade resolves layout overrides into a single
	// PageOptions value. The pipeline reads SSR/CSR/TrailingSlash
	// directly from this field; no per-request layout walk is needed.
	Options kit.PageOptions
	// Error renders the +error.svelte covering this route. nil when no
	// boundary applies; the pipeline falls back to a minimal hardcoded
	// HTML response in that case.
	Error ErrorHandler
	// ErrorLayoutDepth is the count of LayoutChain entries that wrap
	// Error when the boundary fires. Entries past this prefix are
	// inside the broken subtree and are skipped.
	ErrorLayoutDepth int
}
