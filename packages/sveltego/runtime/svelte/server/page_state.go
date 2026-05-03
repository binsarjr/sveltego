package server

import "net/url"

// PageState carries the request-scoped state that Svelte's `$app/state`
// runes expose to render-time code. The transpiled Render function
// receives a PageState alongside the typed PageData; the per-route
// bridge constructs it from kit.RenderCtx before invoking Render so
// templates can read `page.url`, `page.params`, `page.route`, etc.
//
// Server-side, Navigating and Updated stay at their idle values
// (`*Navigation` is nil; Updated is false). Both signals belong to the
// client SPA router — server SSR sees the post-load resting state.
type PageState struct {
	URL    *url.URL
	Params map[string]string
	Route  PageRoute
	Status int
	Error  *PageError
	// Data widens the typed PageData so `page.data.<x>` chains land on
	// the same shape `data.<x>` chains do. The bridge passes the typed
	// load result through here verbatim.
	Data any
	// Form mirrors `page.form`. Always nil during the initial server
	// render — page.form is a client-side reflection of action results.
	Form any
	// State mirrors `page.state` (the user-visible portion of
	// history.state). Always empty server-side; the client hydrates
	// the real value via the SPA router.
	State map[string]any
	// Navigating mirrors `navigating.current`. Always nil on the
	// server; the client SPA router toggles it during in-progress
	// navigations.
	Navigating *Navigation
	// Updated mirrors `updated.current`. Always false on the server;
	// the client version-poller flips it when a new build ships.
	Updated bool
	// CSRFToken carries the per-request double-submit token issued by
	// the CSRF middleware. The per-route bridge populates it from
	// kit.RenderCtx.CSRFToken(). The CSRF auto-inject pre-pass
	// (issue #493) emits `pageState.CSRFToken` as the value of the
	// hidden `_csrf_token` input it splices after every POST form.
	// Empty when CSRF is disabled for the route.
	CSRFToken string
}

// PageRoute mirrors the `page.route` field. ID is the matched route's
// canonical Pattern (e.g. "/post/[id]") or empty when no route matched.
type PageRoute struct {
	ID string
}

// PageError mirrors the `page.error` field surfaced after an error
// boundary catch. Nil when no error is in flight; templates guard with
// `{#if page.error}` (lowered to a nil check).
type PageError struct {
	Message string
	Status  int
}

// Navigation mirrors the `navigating.current` field. Always nil
// server-side; defined here so the lowerer's static field map can
// resolve `navigating.current.<x>` chains without further plumbing.
type Navigation struct {
	From     *NavigationTarget
	To       *NavigationTarget
	Type     string
	Complete chan struct{}
}

// NavigationTarget mirrors the `from` / `to` shape inside Navigation.
// Server renders never see a non-nil Navigation, so this type exists
// only for chain-walk completeness.
type NavigationTarget struct {
	URL    *url.URL
	Params map[string]string
	Route  PageRoute
}
