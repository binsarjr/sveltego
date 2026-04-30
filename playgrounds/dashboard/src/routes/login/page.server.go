//go:build sveltego

package login

import (
	"strings"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/playgrounds/dashboard/src/lib/store"
)

const (
	flashErrorCookie = "flash_error"
	flashUserCookie  = "flash_user"
)

// Load surfaces the last-typed username and the flash error message so
// a failed login round-trip can re-render without forcing the user to
// retype. The cookies are consumed-and-deleted so a refresh does not
// resurface the stale error.
func Load(ctx *kit.LoadCtx) (struct {
	LastUsername string
	LastError    string
	Form         any
}, error,
) {
	username := ""
	errMsg := ""
	if v, ok := ctx.Cookies.Get(flashErrorCookie); ok {
		errMsg = v
		ctx.Cookies.Delete(flashErrorCookie, kit.CookieOpts{Path: "/"})
	}
	if v, ok := ctx.Cookies.Get(flashUserCookie); ok {
		username = v
		ctx.Cookies.Delete(flashUserCookie, kit.CookieOpts{Path: "/"})
	}
	return struct {
		LastUsername string
		LastError    string
		Form         any
	}{
		LastUsername: username,
		LastError:    errMsg,
	}, nil
}

// Actions ships the default login action.
var Actions = kit.ActionMap{
	"default": func(ev *kit.RequestEvent) kit.ActionResult {
		var form struct {
			Username string `form:"username"`
			Password string `form:"password"`
		}
		if err := ev.BindForm(&form); err != nil {
			return failLogin(ev, form.Username, "invalid form data")
		}
		form.Username = strings.TrimSpace(form.Username)
		if form.Username == "" || form.Password == "" {
			return failLogin(ev, form.Username, "username and password required")
		}
		u, err := store.Default.Verify(form.Username, form.Password)
		if err != nil {
			return failLogin(ev, form.Username, "invalid credentials")
		}
		tok, err := store.Default.IssueSession(u.ID)
		if err != nil {
			return failLogin(ev, form.Username, "could not issue session")
		}
		ev.Cookies.Set("session", tok, kit.CookieOpts{Path: "/"})
		return kit.ActionRedirect(303, "/dashboard")
	},
}

func failLogin(ev *kit.RequestEvent, username, msg string) kit.ActionResult {
	ev.Cookies.Set(flashErrorCookie, msg, kit.CookieOpts{Path: "/"})
	if username != "" {
		ev.Cookies.Set(flashUserCookie, username, kit.CookieOpts{Path: "/"})
	}
	return kit.ActionRedirect(303, "/login")
}
