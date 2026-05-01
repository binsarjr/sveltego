//go:build sveltego

package _id_

import (
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/playgrounds/dashboard/src/lib/store"
)

const Templates = "svelte"

const detailFlashCookie = "item_flash"

type Item struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Note      string `json:"note"`
	UpdatedAt string `json:"updatedAt"`
}

type PageData struct {
	Username string `json:"username"`
	Item     Item   `json:"item"`
	FlashMsg string `json:"flashMsg"`
	Form     any    `json:"form"`
}

// Load fetches the item by [id], scoped to the signed-in user. Missing
// items 303 to /dashboard.
func Load(ctx *kit.LoadCtx) (PageData, error) {
	u, _ := ctx.Locals["user"].(*store.User)
	if u == nil {
		return PageData{}, kit.Redirect(303, "/login")
	}
	id := ctx.Params["id"]
	it := store.Default.Get(u.ID, id)
	if it == nil {
		return PageData{}, kit.Redirect(303, "/dashboard")
	}
	flash := ""
	if v, ok := ctx.Cookies.Get(detailFlashCookie); ok {
		flash = v
		ctx.Cookies.Delete(detailFlashCookie, kit.CookieOpts{Path: "/"})
	}
	return PageData{
		Username: u.Username,
		Item: Item{
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
