//go:build sveltego

package dashboard

import (
	"strconv"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/playgrounds/dashboard/src/lib/store"
)

const Templates = "svelte"

const dashFlashCookie = "dash_flash"

type Item struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Note      string `json:"note"`
	UpdatedAt string `json:"updatedAt"`
}

type MetricBar struct {
	Label string `json:"label"`
	Value int    `json:"value"`
	Width string `json:"width"`
}

type PageData struct {
	Username       string      `json:"username"`
	Items          []Item      `json:"items"`
	FlashMsg       string      `json:"flashMsg"`
	MetricLatest   int         `json:"metricLatest"`
	MetricLatestTS string      `json:"metricLatestTs"`
	MetricBars     []MetricBar `json:"metricBars"`
	Form           any         `json:"form"`
}

// Load returns the user's items + the polling chart's latest sample +
// any flash message queued by a redirected Action.
func Load(ctx *kit.LoadCtx) (PageData, error) {
	u, _ := ctx.Locals["user"].(*store.User)
	if u == nil {
		return PageData{}, kit.Redirect(303, "/login")
	}

	flash := ""
	if v, ok := ctx.Cookies.Get(dashFlashCookie); ok {
		flash = v
		ctx.Cookies.Delete(dashFlashCookie, kit.CookieOpts{Path: "/"})
	}

	raw := store.Default.List(u.ID)
	items := make([]Item, len(raw))
	for i, it := range raw {
		items[i] = Item{
			ID:        it.ID,
			Title:     it.Title,
			Note:      it.Note,
			UpdatedAt: it.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		}
	}

	samples := store.Default.Metrics()
	bars := make([]MetricBar, len(samples))
	maxV := 1
	for _, s := range samples {
		if s.Value > maxV {
			maxV = s.Value
		}
	}
	for i, s := range samples {
		w := s.Value * 200 / maxV
		bars[i] = MetricBar{
			Label: s.TS.Format("15:04:05"),
			Value: s.Value,
			Width: "width:" + strconv.Itoa(w) + "px",
		}
	}
	latest := 0
	latestTS := ""
	if len(samples) > 0 {
		latest = samples[len(samples)-1].Value
		latestTS = samples[len(samples)-1].TS.Format("15:04:05")
	}

	return PageData{
		Username:       u.Username,
		Items:          items,
		FlashMsg:       flash,
		MetricLatest:   latest,
		MetricLatestTS: latestTS,
		MetricBars:     bars,
	}, nil
}

// Actions wires create + delete forms on the list page.
var Actions = kit.ActionMap{
	"create": func(ev *kit.RequestEvent) kit.ActionResult {
		u, _ := ev.Locals["user"].(*store.User)
		if u == nil {
			return kit.ActionRedirect(303, "/login")
		}
		var form struct {
			Title string `form:"title"`
			Note  string `form:"note"`
		}
		if err := ev.BindForm(&form); err != nil {
			ev.Cookies.Set(dashFlashCookie, "invalid form data", kit.CookieOpts{Path: "/"})
			return kit.ActionRedirect(303, "/dashboard")
		}
		form.Title = strings.TrimSpace(form.Title)
		if form.Title == "" {
			ev.Cookies.Set(dashFlashCookie, "title required", kit.CookieOpts{Path: "/"})
			return kit.ActionRedirect(303, "/dashboard")
		}
		store.Default.Create(u.ID, form.Title, strings.TrimSpace(form.Note))
		return kit.ActionRedirect(303, "/dashboard")
	},
	"delete": func(ev *kit.RequestEvent) kit.ActionResult {
		u, _ := ev.Locals["user"].(*store.User)
		if u == nil {
			return kit.ActionRedirect(303, "/login")
		}
		var form struct {
			ID string `form:"id"`
		}
		if err := ev.BindForm(&form); err != nil {
			ev.Cookies.Set(dashFlashCookie, "invalid form data", kit.CookieOpts{Path: "/"})
			return kit.ActionRedirect(303, "/dashboard")
		}
		if err := store.Default.Delete(u.ID, form.ID); err != nil {
			ev.Cookies.Set(dashFlashCookie, err.Error(), kit.CookieOpts{Path: "/"})
		}
		return kit.ActionRedirect(303, "/dashboard")
	},
}
