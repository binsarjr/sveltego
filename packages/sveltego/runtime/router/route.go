package router

import (
	"net/http"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/render"
)

// PageHandler renders a +page.svelte for the given context and load data.
type PageHandler func(w *render.Writer, ctx *kit.RenderCtx, data any) error

// ServerHandlers maps HTTP methods to handlers emitted from +server.go.
type ServerHandlers map[string]http.HandlerFunc

// LoadHandler runs the user-written Load() from +page.server.go and
// returns the data threaded into the page render.
type LoadHandler func(ctx *kit.LoadCtx) (any, error)

// ActionsHandler returns the typed Actions value declared in
// +page.server.go. The router keeps it as `any` to remain type-erased;
// the dispatcher casts back to the concrete type.
type ActionsHandler func() any

// Route is one entry in the route table built from the codegen-emitted
// manifest. The router never invokes the handler refs; that is the
// dispatcher's job.
type Route struct {
	// ID is an 8-char FNV-1a hash of Pattern populated by NewTree.
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
	// LayoutChain is filled by the dispatcher in Phase 0h; nil for now.
	LayoutChain []*Route
}
