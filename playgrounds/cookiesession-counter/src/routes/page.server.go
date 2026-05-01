//go:build sveltego

package routes

import (
	"github.com/binsarjr/sveltego/cookiesession"
	"github.com/binsarjr/sveltego/exports/kit"
)

// CounterSession holds the persistent counter value.
type CounterSession struct{ Count int }

type PageData struct{ Count int }

func Load(ctx *kit.LoadCtx) (PageData, error) {
	sess, ok := cookiesession.FromCtx[CounterSession](ctx)
	if !ok {
		return PageData{}, nil
	}
	return PageData{Count: sess.Data().Count}, nil
}

var Actions kit.ActionMap = kit.ActionMap{
	"increment": func(ev *kit.RequestEvent) kit.ActionResult {
		sess, ok := cookiesession.From[CounterSession](ev)
		if !ok {
			return kit.ActionFail(400, nil)
		}
		_ = sess.Update(func(s CounterSession) CounterSession {
			s.Count++
			return s
		})
		return kit.ActionRedirect(303, "/")
	},
	"reset": func(ev *kit.RequestEvent) kit.ActionResult {
		sess, ok := cookiesession.From[CounterSession](ev)
		if !ok {
			return kit.ActionFail(400, nil)
		}
		_ = sess.Set(CounterSession{Count: 0})
		return kit.ActionRedirect(303, "/")
	},
}
