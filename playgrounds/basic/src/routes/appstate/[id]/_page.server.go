//go:build sveltego

package _id_

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

const Templates = "svelte"

// PageData is the load-shape for the $app/state fixture. The .svelte
// template renders every field of `page` and `navigating` so a smoke
// test (or a human eyeballing the rendered HTML) can confirm the rune
// surface lights up end-to-end.
type PageData struct {
	Greeting string `json:"greeting"`
}

func Load(ctx *kit.LoadCtx) (PageData, error) {
	id := ctx.Params["id"]
	return PageData{
		Greeting: "appstate-" + id,
	}, nil
}
