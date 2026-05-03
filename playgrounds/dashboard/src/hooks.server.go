// Package src holds the dashboard's request-pipeline hooks. Handle
// reads the session cookie, populates ev.Locals["user"], and short-
// circuits unauthenticated requests targeted at protected routes.
package src

import (
	"net/http"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/playgrounds/dashboard/src/lib/store"
)

// sessionCookie is the cookie name carrying the opaque session token.
const sessionCookie = "session"

// Handle is the user-authored top-level Handle hook. It runs once per
// request, threads the authenticated user into Locals, and 303s
// unauthenticated requests aimed at protected routes back to /login.
func Handle(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
	if tok, ok := ev.Cookies.Get(sessionCookie); ok {
		if u := store.Default.Lookup(tok); u != nil {
			ev.Locals["user"] = u
			ev.Locals["session"] = tok
		}
	}

	if requiresAuth(ev) && ev.Locals["user"] == nil {
		res := kit.NewResponse(http.StatusSeeOther, nil)
		res.Headers.Set("Location", "/login")
		return res, nil
	}

	return resolve(ev)
}

// requiresAuth reports whether the URL points at a protected route. The
// public surface is /, /login, /api/metrics (the metrics endpoint is
// open so the chart panel can poll without leaking auth state). Every
// other path under /dashboard requires a logged-in user.
func requiresAuth(ev *kit.RequestEvent) bool {
	if ev == nil || ev.URL == nil {
		return false
	}
	p := ev.URL.Path
	switch {
	case p == "/" || p == "/login":
		return false
	case strings.HasPrefix(p, "/api/"):
		return false
	case strings.HasPrefix(p, "/dashboard"):
		return true
	default:
		return false
	}
}
