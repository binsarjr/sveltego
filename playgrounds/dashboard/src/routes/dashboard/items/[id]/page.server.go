//go:build sveltego

package _id_

import (
	"strings"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/playgrounds/dashboard/src/lib/store"
)

const detailFlashCookie = "item_flash"

// Load fetches the item by [id], scoped to the signed-in user. Missing
// items 303 to /dashboard. Inline struct literals only (ADR 0004
// amendment): PageData inference walks the return statement's literal
// type and skips named aliases.
func Load(ctx *kit.LoadCtx) (struct {
	Username string
	Item     struct {
		ID        string
		Title     string
		Note      string
		UpdatedAt string
	}
	FlashMsg string
}, error,
) {
	zero := struct {
		Username string
		Item     struct {
			ID        string
			Title     string
			Note      string
			UpdatedAt string
		}
		FlashMsg string
	}{}

	u, _ := ctx.Locals["user"].(*store.User)
	if u == nil {
		return zero, kit.Redirect(303, "/login")
	}
	id := ctx.Params["id"]
	it := store.Default.Get(u.ID, id)
	if it == nil {
		return zero, kit.Redirect(303, "/dashboard")
	}
	flash := ""
	if v, ok := ctx.Cookies.Get(detailFlashCookie); ok {
		flash = v
		ctx.Cookies.Delete(detailFlashCookie, kit.CookieOpts{Path: "/"})
	}
	return struct {
		Username string
		Item     struct {
			ID        string
			Title     string
			Note      string
			UpdatedAt string
		}
		FlashMsg string
	}{
		Username: u.Username,
		Item: struct {
			ID        string
			Title     string
			Note      string
			UpdatedAt string
		}{
			ID:        it.ID,
			Title:     it.Title,
			Note:      it.Note,
			UpdatedAt: it.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		},
		FlashMsg: flash,
	}, nil
}

// Actions wires update + delete on the detail page.
var Actions = kit.ActionMap{
	"update": func(ev *kit.RequestEvent) kit.ActionResult {
		u, _ := ev.Locals["user"].(*store.User)
		if u == nil {
			return kit.ActionRedirect(303, "/login")
		}
		id := ev.Params["id"]
		var form struct {
			Title string `form:"title"`
			Note  string `form:"note"`
		}
		if err := ev.BindForm(&form); err != nil {
			ev.Cookies.Set(detailFlashCookie, "invalid form data", kit.CookieOpts{Path: "/"})
			return kit.ActionRedirect(303, "/dashboard/items/"+id)
		}
		form.Title = strings.TrimSpace(form.Title)
		if form.Title == "" {
			ev.Cookies.Set(detailFlashCookie, "title required", kit.CookieOpts{Path: "/"})
			return kit.ActionRedirect(303, "/dashboard/items/"+id)
		}
		if _, err := store.Default.Update(u.ID, id, form.Title, strings.TrimSpace(form.Note)); err != nil {
			ev.Cookies.Set(detailFlashCookie, err.Error(), kit.CookieOpts{Path: "/"})
			return kit.ActionRedirect(303, "/dashboard/items/"+id)
		}
		return kit.ActionRedirect(303, "/dashboard/items/"+id)
	},
	"delete": func(ev *kit.RequestEvent) kit.ActionResult {
		u, _ := ev.Locals["user"].(*store.User)
		if u == nil {
			return kit.ActionRedirect(303, "/login")
		}
		id := ev.Params["id"]
		if err := store.Default.Delete(u.ID, id); err != nil {
			ev.Cookies.Set(detailFlashCookie, err.Error(), kit.CookieOpts{Path: "/"})
			return kit.ActionRedirect(303, "/dashboard/items/"+id)
		}
		return kit.ActionRedirect(303, "/dashboard")
	},
}
