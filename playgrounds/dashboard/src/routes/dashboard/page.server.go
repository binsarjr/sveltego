//go:build sveltego

package dashboard

import (
	"strconv"
	"strings"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/playgrounds/dashboard/src/lib/store"
)

const dashFlashCookie = "dash_flash"

// Load returns the user's items + the polling chart's latest sample +
// any flash message queued by a redirected Action.
//
// PageData inference (ADR 0004 amendment) only walks anonymous struct
// literals, so the return statement uses an inline struct literal —
// not a named alias — and every nested type follows the same rule.
func Load(ctx *kit.LoadCtx) (struct {
	Username string
	Items    []struct {
		ID        string
		Title     string
		Note      string
		UpdatedAt string
	}
	FlashMsg       string
	MetricLatest   int
	MetricLatestTS string
	MetricBars     []struct {
		Label string
		Value int
		Width string
	}
}, error,
) {
	u, _ := ctx.Locals["user"].(*store.User)
	if u == nil {
		return struct {
			Username string
			Items    []struct {
				ID        string
				Title     string
				Note      string
				UpdatedAt string
			}
			FlashMsg       string
			MetricLatest   int
			MetricLatestTS string
			MetricBars     []struct {
				Label string
				Value int
				Width string
			}
		}{}, kit.Redirect(303, "/login")
	}

	flash := ""
	if v, ok := ctx.Cookies.Get(dashFlashCookie); ok {
		flash = v
		ctx.Cookies.Delete(dashFlashCookie, kit.CookieOpts{Path: "/"})
	}

	raw := store.Default.List(u.ID)
	items := make([]struct {
		ID        string
		Title     string
		Note      string
		UpdatedAt string
	}, len(raw))
	for i, it := range raw {
		items[i] = struct {
			ID        string
			Title     string
			Note      string
			UpdatedAt string
		}{
			ID:        it.ID,
			Title:     it.Title,
			Note:      it.Note,
			UpdatedAt: it.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		}
	}

	samples := store.Default.Metrics()
	bars := make([]struct {
		Label string
		Value int
		Width string
	}, len(samples))
	maxV := 1
	for _, s := range samples {
		if s.Value > maxV {
			maxV = s.Value
		}
	}
	for i, s := range samples {
		w := s.Value * 200 / maxV
		bars[i] = struct {
			Label string
			Value int
			Width string
		}{
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

	return struct {
		Username string
		Items    []struct {
			ID        string
			Title     string
			Note      string
			UpdatedAt string
		}
		FlashMsg       string
		MetricLatest   int
		MetricLatestTS string
		MetricBars     []struct {
			Label string
			Value int
			Width string
		}
	}{
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
