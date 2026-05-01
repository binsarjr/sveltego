//go:build sveltego

package routes

import (
	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/playgrounds/dashboard/src/lib/store"
)

const Templates = "svelte"

type PageData struct {
	LoggedIn bool   `json:"loggedIn"`
	Username string `json:"username"`
	Form     any    `json:"form"`
}

// Load returns the auth state so the index template can render a
// personalized welcome or a sign-in CTA. Locals["user"] is populated by
// the Handle hook in src/hooks.server.go.
func Load(ctx *kit.LoadCtx) (PageData, error) {
	loggedIn := false
	username := ""
	if u, ok := ctx.Locals["user"].(*store.User); ok && u != nil {
		loggedIn = true
		username = u.Username
	}
	return PageData{
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
