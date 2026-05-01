//go:build sveltego

package routes

import (
	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/playgrounds/dashboard/src/lib/store"
)

// Load returns the auth state so the index template can render a
// personalized welcome or a sign-in CTA. Locals["user"] is populated by
// the Handle hook in src/hooks.server.go.
// The Form field is required by codegen on every page that declares
// `var Actions = kit.ActionMap{...}` — codegen widens PageData with
// `Form any` so action results can re-render. The user Load returns
// the same struct type so the manifest adapter's type assertion
// matches; runtime injects the action's payload after Load runs.
func Load(ctx *kit.LoadCtx) (struct {
	LoggedIn bool
	Username string
	Form     any
}, error,
) {
	loggedIn := false
	username := ""
	if u, ok := ctx.Locals["user"].(*store.User); ok && u != nil {
		loggedIn = true
		username = u.Username
	}
	return struct {
		LoggedIn bool
		Username string
		Form     any
	}{
		LoggedIn: loggedIn,
		Username: username,
	}, nil
}

// Actions surfaces the logout action: clear the session cookie, drop
// the server-side mapping, redirect home.
var Actions = kit.ActionMap{
	"logout": func(ev *kit.RequestEvent) kit.ActionResult {
		if tok, ok := ev.Cookies.Get("session"); ok {
			store.Default.Revoke(tok)
		}
		ev.Cookies.Delete("session", kit.CookieOpts{Path: "/"})
		return kit.ActionRedirect(303, "/")
	},
}
